package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/example/gitea-jenkins-webhook/internal/config"
	"github.com/example/gitea-jenkins-webhook/internal/processor"
	"github.com/example/gitea-jenkins-webhook/pkg/webhook"
)

const (
	headerEvent     = "X-Gitea-Event"
	headerSignature = "X-Gitea-Signature"
)

type Server struct {
	cfg       *config.Config
	processor *processor.Processor
	server    *http.Server
	log       *slog.Logger
}

func New(cfg *config.Config, proc *processor.Processor, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	mux := http.NewServeMux()
	s := &Server{
		cfg:       cfg,
		processor: proc,
		log:       logger,
	}
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /webhook", s.handleWebhook)

	s.server = &http.Server{
		Addr:              cfg.Server.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

func (s *Server) Run(ctx context.Context) error {
	s.processor.Start()
	defer s.processor.Stop()

	errCh := make(chan error, 1)
	go func() {
		s.log.Info("starting HTTP server", "addr", s.server.Addr)
		if err := s.server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		s.log.Info("shutting down HTTP server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("server shutdown: %w", err)
		}
		return nil
	case err := <-errCh:
		return err
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	event := r.Header.Get(headerEvent)
	if event != "pull_request" {
		s.log.Info("unsupported gitea event", "event", event)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.log.Error("read webhook body", "err", err)
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if s.cfg.Server.WebhookSecret != "" {
		if err := verifySignature(body, r.Header.Get(headerSignature), s.cfg.Server.WebhookSecret); err != nil {
			s.log.Warn("invalid webhook signature", "err", err)
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	}

	var prEvent webhook.PullRequestEvent
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&prEvent); err != nil {
		s.log.Error("decode webhook payload", "err", err)
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	prEvent.Timestamp = time.Now()

	if err := s.processor.Enqueue(prEvent); err != nil {
		s.log.Error("enqueue event", "err", err)
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func verifySignature(payload []byte, signature, secret string) error {
	if signature == "" {
		return fmt.Errorf("missing signature header")
	}
	signature = normalizeSignature(signature)
	expected := computeSignature(payload, secret)
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}

func computeSignature(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func normalizeSignature(sig string) string {
	s := strings.TrimSpace(sig)
	if strings.HasPrefix(s, "sha256=") {
		return strings.TrimPrefix(s, "sha256=")
	}
	return s
}

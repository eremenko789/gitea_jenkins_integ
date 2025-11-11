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
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /webhook", s.handleWebhook)

	s.server = &http.Server{
		Addr:              cfg.Server.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

func (s *Server) Run(ctx context.Context) error {
	s.log.Info("starting processor")
	s.processor.Start()
	defer func() {
		s.log.Info("stopping processor")
		s.processor.Stop()
	}()

	errCh := make(chan error, 1)
	go func() {
		s.log.Info("starting HTTP server", "addr", s.server.Addr)
		if err := s.server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			s.log.Error("HTTP server error", "err", err)
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		s.log.Info("shutting down HTTP server", "reason", ctx.Err())
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			s.log.Error("server shutdown error", "err", err)
			return fmt.Errorf("server shutdown: %w", err)
		}
		s.log.Info("HTTP server shut down successfully")
		return nil
	case err := <-errCh:
		return err
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("health check request",
		"method", r.Method,
		"remote_addr", r.RemoteAddr,
		"user_agent", r.UserAgent())
	s.log.Debug("health check request headers", "headers", r.Header)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
	s.log.Debug("health check response sent", "status", http.StatusOK)
}

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	s.log.Info("webhook request received",
		"method", r.Method,
		"remote_addr", r.RemoteAddr,
		"user_agent", r.UserAgent())
	s.log.Debug("webhook request headers", "headers", r.Header)

	event := r.Header.Get(headerEvent)
	s.log.Debug("webhook event type", "event", event)
	if event != "pull_request" {
		s.log.Info("unsupported gitea event", "event", event)
		w.WriteHeader(http.StatusNoContent)
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.log.Error("read webhook body", "err", err)
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	s.log.Debug("webhook request body", "body", string(body), "size_bytes", len(body))

	if s.cfg.Server.WebhookSecret != "" {
		signature := r.Header.Get(headerSignature)
		s.log.Debug("verifying webhook signature", "signature_header", signature)
		if err := verifySignature(body, signature, s.cfg.Server.WebhookSecret); err != nil {
			s.log.Warn("invalid webhook signature", "err", err)
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		s.log.Debug("webhook signature verified successfully")
	} else {
		s.log.Debug("webhook secret not configured, skipping signature verification")
	}

	var prEvent webhook.PullRequestEvent
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&prEvent); err != nil {
		s.log.Error("decode webhook payload", "err", err)
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	prEvent.Timestamp = time.Now()

	s.log.Info("webhook payload decoded",
		"action", prEvent.Action,
		"repo", prEvent.Repository.FullName,
		"pr_number", prEvent.PullRequest.Number,
		"sender", prEvent.Sender.Login)
	s.log.Debug("webhook event details",
		"event", prEvent,
		"timestamp", prEvent.Timestamp)

	if err := s.processor.Enqueue(prEvent); err != nil {
		s.log.Error("enqueue event", "err", err)
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}

	s.log.Info("webhook event enqueued successfully",
		"repo", prEvent.Repository.FullName,
		"pr_number", prEvent.PullRequest.Number)
	w.WriteHeader(http.StatusAccepted)
	s.log.Debug("webhook response sent", "status", http.StatusAccepted)
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

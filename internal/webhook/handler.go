package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"gitea_jenkins_integ/internal/config"
	"gitea_jenkins_integ/internal/processor"
)

const (
	headerEvent     = "X-Gitea-Event"
	headerSignature = "X-Gitea-Signature"
	headerDelivery  = "X-Gitea-Delivery"
)

var allowedActions = map[string]struct{}{
	"opened":   {},
	"reopened": {},
}

// Handler processes incoming Gitea pull request webhooks.
type Handler struct {
	cfg    *config.Config
	queue  TaskQueue
	logger *slog.Logger
	secret string
}

// TaskQueue wraps processor.Enqueue to facilitate testing.
type TaskQueue interface {
	Enqueue(task processor.Task) error
}

// New creates a webhook handler.
func New(cfg *config.Config, queue TaskQueue, logger *slog.Logger, secret string) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		cfg:    cfg,
		queue:  queue,
		logger: logger,
		secret: secret,
	}
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	eventType := strings.ToLower(strings.TrimSpace(r.Header.Get(headerEvent)))
	if eventType != "pull_request" {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusInternalServerError)
		return
	}

	if h.secret != "" {
		if !h.verifySignature(body, r.Header.Get(headerSignature)) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	}

	var payload PullRequestEvent
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	if _, ok := allowedActions[strings.ToLower(payload.Action)]; !ok {
		h.logger.Info("ignoring action", slog.String("action", payload.Action))
		w.WriteHeader(http.StatusAccepted)
		return
	}

	if payload.Repository.FullName == "" {
		http.Error(w, "missing repository.full_name", http.StatusBadRequest)
		return
	}

	repoCfg, ok := h.cfg.Repo(payload.Repository.FullName)
	if !ok {
		h.logger.Info("repository not configured",
			slog.String("repo", payload.Repository.FullName),
			slog.String("delivery_id", r.Header.Get(headerDelivery)),
		)
		w.WriteHeader(http.StatusAccepted)
		return
	}

	prNumber := payload.PullRequest.Number
	if prNumber == 0 {
		prNumber = payload.Number
	}
	if prNumber == 0 {
		http.Error(w, "missing pull request number", http.StatusBadRequest)
		return
	}

	task := processor.PullRequestInfo{
		Number:       prNumber,
		Title:        payload.PullRequest.Title,
		URL:          payload.PullRequest.HTMLURL,
		SourceBranch: payload.PullRequest.Head.Ref,
		TargetBranch: payload.PullRequest.Base.Ref,
	}

	now := time.Now()
	for i := range repoCfg.Patterns {
		pattern := &repoCfg.Patterns[i]
		err := h.queue.Enqueue(processor.Task{
			RepositoryFullName: payload.Repository.FullName,
			RepositoryConfig:   repoCfg,
			Pattern:            pattern,
			PullRequest:        task,
			ReceivedAt:         now,
		})
		if err != nil {
			if errors.Is(err, processor.ErrQueueFull) {
				http.Error(w, "processor overloaded", http.StatusServiceUnavailable)
				return
			}
			h.logger.Error("failed to enqueue task",
				slog.String("error", err.Error()),
				slog.String("repo", payload.Repository.FullName),
				slog.Int("pr", prNumber),
				slog.String("pattern", pattern.Name),
			)
		}
	}

	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) verifySignature(body []byte, header string) bool {
	secret := []byte(h.secret)
	signature := strings.TrimSpace(header)
	if signature == "" {
		return false
	}
	const prefix = "sha256="
	if strings.HasPrefix(signature, prefix) {
		signature = signature[len(prefix):]
	}
	got, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	expected := mac.Sum(nil)

	return hmac.Equal(expected, got)
}

// PullRequestEvent models the subset of Gitea webhook fields the service requires.
type PullRequestEvent struct {
	Action      string `json:"action"`
	Number      int    `json:"number"`
	PullRequest struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		HTMLURL string `json:"html_url"`
		Head    struct {
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	} `json:"pull_request"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

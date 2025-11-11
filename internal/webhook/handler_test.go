package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gitea_jenkins_integ/internal/config"
	"gitea_jenkins_integ/internal/processor"
)

type stubQueue struct {
	tasks []processor.Task
	err   error
}

func (s *stubQueue) Enqueue(task processor.Task) error {
	s.tasks = append(s.tasks, task)
	return s.err
}

func TestHandlerRejectsInvalidSignature(t *testing.T) {
	cfg := &config.Config{
		Repositories: []config.RepositoryConfig{
			{
				FullName: "org/repo",
				Patterns: []config.PatternConfig{{Name: "default", RegexTemplate: ".*"}},
			},
		},
	}
	queue := &stubQueue{}
	handler := New(cfg, queue, nil, "secret")

	payload := map[string]any{
		"action": "opened",
		"number": 1,
		"pull_request": map[string]any{
			"number": 1,
		},
		"repository": map[string]any{
			"full_name": "org/repo",
		},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Gitea-Event", "pull_request")
	req.Header.Set("X-Gitea-Signature", "sha256=deadbeef")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandlerEnqueuesTasks(t *testing.T) {
	cfg := &config.Config{
		Processing: config.ProcessingConfig{
			DefaultWaitTimeout:  config.Duration{Duration: time.Second},
			DefaultPollInterval: config.Duration{Duration: 10 * time.Millisecond},
		},
		Repositories: []config.RepositoryConfig{
			{
				FullName: "org/repo",
				Patterns: []config.PatternConfig{
					{Name: "default", RegexTemplate: "job-{{ .PullRequest }}"},
					{Name: "secondary", RegexTemplate: "other-{{ .PullRequest }}"},
				},
			},
		},
	}
	queue := &stubQueue{}
	handler := New(cfg, queue, nil, "secret")

	payload := map[string]any{
		"action": "opened",
		"number": 7,
		"pull_request": map[string]any{
			"number":   7,
			"title":    "Example",
			"html_url": "https://gitea/pulls/7",
			"head":     map[string]any{"ref": "feature"},
			"base":     map[string]any{"ref": "main"},
		},
		"repository": map[string]any{
			"full_name": "org/repo",
		},
	}
	body, _ := json.Marshal(payload)

	signature := computeSignature("secret", body)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Gitea-Event", "pull_request")
	req.Header.Set("X-Gitea-Signature", "sha256="+signature)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
	if len(queue.tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(queue.tasks))
	}
	if queue.tasks[0].PullRequest.Number != 7 {
		t.Fatalf("unexpected PR number %d", queue.tasks[0].PullRequest.Number)
	}
}

func TestHandlerQueueFull(t *testing.T) {
	cfg := &config.Config{
		Repositories: []config.RepositoryConfig{
			{
				FullName: "org/repo",
				Patterns: []config.PatternConfig{{Name: "default", RegexTemplate: ".*"}},
			},
		},
	}
	queue := &stubQueue{err: processor.ErrQueueFull}
	handler := New(cfg, queue, nil, "")

	payload := map[string]any{
		"action": "opened",
		"number": 1,
		"pull_request": map[string]any{
			"number": 1,
		},
		"repository": map[string]any{
			"full_name": "org/repo",
		},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Gitea-Event", "pull_request")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func computeSignature(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

package processor

import (
	"context"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"gitea_jenkins_integ/internal/config"
	"gitea_jenkins_integ/internal/jenkins"
)

type fakeJenkins struct {
	mu        sync.Mutex
	jobs      []jenkins.Job
	callCount int
	failUntil int
	err       error
}

func (f *fakeJenkins) FindJob(ctx context.Context, rx *regexp.Regexp) (*jenkins.Job, bool, error) {
	if err := f.err; err != nil {
		return nil, false, err
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.callCount++
	if f.failUntil > 0 && f.callCount <= f.failUntil {
		return nil, false, nil
	}
	for _, job := range f.jobs {
		if rx.MatchString(job.Name) {
			return &job, true, nil
		}
	}
	return nil, false, nil
}

type fakeGitea struct {
	mu       sync.Mutex
	comments []string
	ch       chan string
}

func newFakeGitea() *fakeGitea {
	return &fakeGitea{ch: make(chan string, 1)}
}

func (f *fakeGitea) CreateComment(ctx context.Context, _ string, _ int, body string) error {
	f.mu.Lock()
	f.comments = append(f.comments, body)
	f.mu.Unlock()
	select {
	case f.ch <- body:
	default:
	}
	return nil
}

func TestProcessorSuccess(t *testing.T) {
	cfg := &config.Config{
		Processing: config.ProcessingConfig{
			WorkerCount:         1,
			QueueSize:           2,
			DefaultWaitTimeout:  config.Duration{Duration: 500 * time.Millisecond},
			DefaultPollInterval: config.Duration{Duration: 10 * time.Millisecond},
		},
	}
	repo := &config.RepositoryConfig{
		FullName: "org/repo",
		Patterns: []config.PatternConfig{
			{Name: "default", RegexTemplate: "^build-pr-{{ .PullRequest }}$"},
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	gitea := newFakeGitea()
	jenk := &fakeJenkins{
		jobs: []jenkins.Job{
			{Name: "build-pr-42", URL: "https://jenkins/job/build-pr-42"},
		},
	}

	proc := New(cfg, jenk, gitea, logger)
	defer proc.Shutdown(context.Background())

	task := Task{
		RepositoryFullName: repo.FullName,
		RepositoryConfig:   repo,
		Pattern:            &repo.Patterns[0],
		PullRequest: PullRequestInfo{
			Number: 42,
			Title:  "Test PR",
		},
		ReceivedAt: time.Now(),
	}

	if err := proc.Enqueue(task); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	select {
	case comment := <-gitea.ch:
		if !strings.Contains(comment, "https://jenkins/job/build-pr-42") {
			t.Fatalf("unexpected comment: %q", comment)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for comment")
	}
}

func TestProcessorFailure(t *testing.T) {
	cfg := &config.Config{
		Processing: config.ProcessingConfig{
			WorkerCount:         1,
			QueueSize:           2,
			DefaultWaitTimeout:  config.Duration{Duration: 100 * time.Millisecond},
			DefaultPollInterval: config.Duration{Duration: 20 * time.Millisecond},
		},
	}
	repo := &config.RepositoryConfig{
		FullName: "org/repo",
		Patterns: []config.PatternConfig{
			{Name: "default", RegexTemplate: "^build-pr-{{ .PullRequest }}$"},
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	gitea := newFakeGitea()
	jenk := &fakeJenkins{}

	proc := New(cfg, jenk, gitea, logger)
	defer proc.Shutdown(context.Background())

	task := Task{
		RepositoryFullName: repo.FullName,
		RepositoryConfig:   repo,
		Pattern:            &repo.Patterns[0],
		PullRequest: PullRequestInfo{
			Number: 99,
			Title:  "Missing job",
		},
		ReceivedAt: time.Now(),
	}

	if err := proc.Enqueue(task); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	select {
	case comment := <-gitea.ch:
		if !strings.Contains(comment, "not found") {
			t.Fatalf("expected failure comment, got %q", comment)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for failure comment")
	}
}

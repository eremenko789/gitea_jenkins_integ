package processor_test

import (
	"context"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/example/gitea-jenkins-webhook/internal/config"
	"github.com/example/gitea-jenkins-webhook/internal/jenkins"
	"github.com/example/gitea-jenkins-webhook/internal/processor"
	"github.com/example/gitea-jenkins-webhook/pkg/webhook"
)

type stubJenkins struct {
	job *jenkins.Job
	err error
}

func (s stubJenkins) WaitForJob(ctx context.Context, _ *regexp.Regexp, _ string, timeout, interval time.Duration) (*jenkins.Job, error) {
	return s.job, s.err
}

type stubGitea struct {
	t        *testing.T
	mu       sync.Mutex
	comments []string
	wg       sync.WaitGroup
}

func newStubGitea(t *testing.T) *stubGitea {
	return &stubGitea{t: t}
}

func (s *stubGitea) PostComment(ctx context.Context, repoFullName string, issueIndex int64, body string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.comments = append(s.comments, body)
	s.wg.Done()
	return nil
}

func TestProcessor_PostsSuccessComment(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			WorkerPoolSize: 1,
			QueueSize:      10,
		},
		Jenkins: config.JenkinsConfig{
			BaseURL:      "https://jenkins.example.com",
			PollInterval: time.Millisecond,
			Timeout:      time.Second,
		},
		Gitea: config.GiteaConfig{
			BaseURL: "https://gitea.example.com",
			Token:   "token",
		},
		Repositories: []config.RepositoryRule{
			{
				Name:       "org/repo",
				JobPattern: `^job-{{ .Number }}$`,
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	job := &jenkins.Job{Name: "job-42", URL: "https://jenkins/job-42"}
	jClient := stubJenkins{job: job}
	gClient := newStubGitea(t)
	gClient.wg.Add(1)

	proc := processor.New(cfg, jClient, gClient, nil)
	proc.Start()
	defer proc.Stop()

	event := webhook.PullRequestEvent{
		Action: "opened",
		PullRequest: webhook.PullRequest{
			Number: 42,
			Title:  "test",
		},
		Repository: webhook.Repository{
			FullName: "org/repo",
		},
	}

	if err := proc.Enqueue(event); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	waitWithTimeout(t, &gClient.wg, 2*time.Second)

	gClient.mu.Lock()
	defer gClient.mu.Unlock()
	if len(gClient.comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(gClient.comments))
	}
	if got := gClient.comments[0]; got != "✅ Jenkins job job-42 detected: https://jenkins/job-42" {
		t.Fatalf("unexpected comment: %s", got)
	}
}

func TestProcessor_PostsFailureCommentWhenNoJobFound(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			WorkerPoolSize: 1,
			QueueSize:      10,
		},
		Jenkins: config.JenkinsConfig{
			BaseURL:      "https://jenkins.example.com",
			PollInterval: time.Millisecond,
			Timeout:      time.Second,
		},
		Gitea: config.GiteaConfig{
			BaseURL: "https://gitea.example.com",
			Token:   "token",
		},
		Repositories: []config.RepositoryRule{
			{
				Name:                   "org/repo",
				JobPattern:             `^job-{{ .Number }}$`,
				FailureCommentTemplate: "failure for {{ .Number }}",
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	jClient := stubJenkins{job: nil, err: context.DeadlineExceeded}
	gClient := newStubGitea(t)
	gClient.wg.Add(1)

	proc := processor.New(cfg, jClient, gClient, nil)
	proc.Start()
	defer proc.Stop()

	event := webhook.PullRequestEvent{
		Action: "opened",
		PullRequest: webhook.PullRequest{
			Number: 7,
			Title:  "test",
		},
		Repository: webhook.Repository{
			FullName: "org/repo",
		},
	}

	if err := proc.Enqueue(event); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	waitWithTimeout(t, &gClient.wg, 2*time.Second)

	gClient.mu.Lock()
	defer gClient.mu.Unlock()
	if len(gClient.comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(gClient.comments))
	}
	if got := gClient.comments[0]; got != "failure for 7" {
		t.Fatalf("unexpected comment: %s", got)
	}
}

func waitWithTimeout(t *testing.T, wg *sync.WaitGroup, timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("timeout waiting for waitgroup")
	}
}

package processor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/example/gitea-jenkins-webhook/internal/config"
	"github.com/example/gitea-jenkins-webhook/internal/jenkins"
	"github.com/example/gitea-jenkins-webhook/pkg/webhook"
)

type JenkinsClient interface {
	WaitForJob(ctx context.Context, pattern *regexp.Regexp, timeout, interval time.Duration) (*jenkins.Job, error)
}

type GiteaClient interface {
	PostComment(ctx context.Context, repoFullName string, issueIndex int64, body string) error
}

type Processor struct {
	cfg     *config.Config
	log     *slog.Logger
	jc      JenkinsClient
	gc      GiteaClient
	queue   chan webhook.PullRequestEvent
	wg      sync.WaitGroup
	started bool
	mu      sync.Mutex
}

func New(cfg *config.Config, jc JenkinsClient, gc GiteaClient, logger *slog.Logger) *Processor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Processor{
		cfg:   cfg,
		log:   logger,
		jc:    jc,
		gc:    gc,
		queue: make(chan webhook.PullRequestEvent, cfg.Server.QueueSize),
	}
}

func (p *Processor) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return
	}

	for i := 0; i < p.cfg.Server.WorkerPoolSize; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
	p.started = true
}

func (p *Processor) Stop() {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return
	}
	close(p.queue)
	p.mu.Unlock()
	p.wg.Wait()
}

func (p *Processor) Enqueue(evt webhook.PullRequestEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.started {
		return errors.New("processor not started")
	}
	select {
	case p.queue <- evt:
		return nil
	default:
		return fmt.Errorf("processor queue is full")
	}
}

func (p *Processor) worker(id int) {
	defer p.wg.Done()
	for evt := range p.queue {
		p.processEvent(context.Background(), evt)
	}
}

func (p *Processor) processEvent(ctx context.Context, evt webhook.PullRequestEvent) {
	if evt.Repository.FullName == "" {
		p.log.Warn("event missing repository", "event", evt)
		return
	}

	rule, ok := p.cfg.GetRepositoryRule(evt.Repository.FullName)
	if !ok {
		p.log.Info("repository not configured, skipping", "repo", evt.Repository.FullName)
		return
	}

	if evt.Action != "opened" && evt.Action != "reopened" {
		p.log.Info("ignoring pull request action", "action", evt.Action)
		return
	}

	ctx = context.WithValue(ctx, "repository", evt.Repository.FullName)
	p.log.Info("processing pull request", "repo", evt.Repository.FullName, "pr", evt.PullRequest.Number)

	data := map[string]any{
		"Number":  evt.PullRequest.Number,
		"Title":   evt.PullRequest.Title,
		"Repo":    evt.Repository.FullName,
		"Sender":  evt.Sender.Login,
		"Timeout": rule.Timeout,
	}

	var (
		jobFound *jenkins.Job
		pattern  string
		err      error
	)

	for _, patternTemplate := range rule.JobPatterns {
		pattern, err = executeTemplate("pattern", patternTemplate, data)
		if err != nil {
			p.log.Error("failed to execute pattern template", "err", err, "pattern", patternTemplate)
			continue
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			p.log.Error("invalid regex pattern", "pattern", pattern, "err", err)
			continue
		}

		p.log.Info("waiting for jenkins job", "pattern", pattern)
		jobFound, err = p.jc.WaitForJob(ctx, re, rule.Timeout, rule.PollInterval)
		if err == nil && jobFound != nil {
			p.log.Info("jenkins job detected", "job", jobFound.Name, "url", jobFound.URL)
			break
		}
		if errors.Is(err, context.DeadlineExceeded) || jobFound == nil {
			p.log.Warn("jenkins job not found within timeout", "pattern", pattern)
			continue
		}
		if err != nil {
			p.log.Error("error waiting for jenkins job", "pattern", pattern, "err", err)
		}
	}

	var commentTemplate string
	if jobFound != nil {
		commentTemplate = rule.SuccessCommentTemplate
		data["JobName"] = jobFound.Name
		data["JobURL"] = jobFound.URL
	} else {
		commentTemplate = rule.FailureCommentTemplate
	}

	body, err := executeTemplate("comment", commentTemplate, data)
	if err != nil {
		p.log.Error("failed to execute comment template", "err", err)
		return
	}

	if err := p.gc.PostComment(ctx, evt.Repository.FullName, evt.PullRequest.Number, body); err != nil {
		p.log.Error("failed to post comment to gitea", "err", err)
	} else {
		p.log.Info("comment posted to Gitea", "repo", evt.Repository.FullName, "pr", evt.PullRequest.Number)
	}
}

func executeTemplate(name, tpl string, data any) (string, error) {
	t, err := template.New(name).Parse(tpl)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

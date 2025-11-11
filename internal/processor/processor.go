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

	"gitea_jenkins_integ/internal/config"
	"gitea_jenkins_integ/internal/jenkins"
)

// JenkinsClient defines the subset of Jenkins functionality the processor depends on.
type JenkinsClient interface {
	FindJob(ctx context.Context, rx *regexp.Regexp) (*jenkins.Job, bool, error)
}

// GiteaClient defines the subset of Gitea functionality the processor depends on.
type GiteaClient interface {
	CreateComment(ctx context.Context, repoFullName string, prNumber int, body string) error
}

// Task includes all context required to evaluate Jenkins job patterns for a PR.
type Task struct {
	RepositoryFullName string
	RepositoryConfig   *config.RepositoryConfig
	Pattern            *config.PatternConfig
	PullRequest        PullRequestInfo
	ReceivedAt         time.Time
}

// PullRequestInfo captures the relevant fields from a Gitea webhook payload.
type PullRequestInfo struct {
	Number       int
	Title        string
	URL          string
	SourceBranch string
	TargetBranch string
}

// Processor executes tasks asynchronously using a worker pool.
type Processor struct {
	cfg     *config.Config
	jenkins JenkinsClient
	gitea   GiteaClient
	logger  *slog.Logger

	queue    chan Task
	wg       sync.WaitGroup
	stopOnce sync.Once
}

// ErrQueueFull indicates that no capacity is available to enqueue the task.
var ErrQueueFull = errors.New("processor queue is full")

// New creates a processor with workerCount workers.
func New(cfg *config.Config, j JenkinsClient, g GiteaClient, logger *slog.Logger) *Processor {
	if logger == nil {
		logger = slog.Default()
	}
	p := &Processor{
		cfg:     cfg,
		jenkins: j,
		gitea:   g,
		logger:  logger,
		queue:   make(chan Task, cfg.Processing.QueueSize),
	}

	for i := 0; i < cfg.Processing.WorkerCount; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}

	return p
}

// Enqueue submits a task for asynchronous processing.
func (p *Processor) Enqueue(task Task) error {
	select {
	case p.queue <- task:
		return nil
	default:
		return ErrQueueFull
	}
}

// Shutdown gracefully stops accepting new tasks and waits for in-flight tasks to complete.
func (p *Processor) Shutdown(ctx context.Context) {
	p.stopOnce.Do(func() {
		close(p.queue)
	})

	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		p.logger.Warn("shutdown timed out", slog.String("error", ctx.Err().Error()))
	}
}

func (p *Processor) worker(id int) {
	defer p.wg.Done()
	for task := range p.queue {
		logger := p.logger.With(
			slog.Int("worker_id", id),
			slog.Int("pr_number", task.PullRequest.Number),
			slog.String("repo", task.RepositoryFullName),
			slog.String("pattern", task.Pattern.Name),
		)
		if err := p.handleTask(task, logger); err != nil {
			logger.Error("task failed", slog.String("error", err.Error()))
		}
	}
}

func (p *Processor) handleTask(task Task, logger *slog.Logger) error {
	waitTimeout := p.cfg.WaitTimeoutForPattern(task.RepositoryConfig, task.Pattern)
	pollInterval := p.cfg.PollIntervalForPattern(task.RepositoryConfig, task.Pattern)
	if pollInterval <= 0 {
		pollInterval = time.Second
	}

	regexStr, err := executeTemplate(task.Pattern.RegexTemplate, p.templateData(task, nil))
	if err != nil {
		return fmt.Errorf("render regex template: %w", err)
	}

	regex, err := regexp.Compile(regexStr)
	if err != nil {
		return fmt.Errorf("compile regex %q: %w", regexStr, err)
	}

	logger.Info("started task",
		slog.String("regex", regex.String()),
		slog.Duration("wait_timeout", waitTimeout),
		slog.Duration("poll_interval", pollInterval),
	)

	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	attempt := 0
	for {
		attempt++
		job, found, err := p.jenkins.FindJob(ctx, regex)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				break
			}
			logger.Warn("failed to query Jenkins", slog.String("error", err.Error()))
		} else if found {
			logger.Info("jenkins job found",
				slog.String("job_name", job.Name),
				slog.String("job_url", job.URL),
				slog.Int("attempt", attempt),
			)
			if err := p.postSuccess(task, job, regex, waitTimeout, attempt, time.Since(task.ReceivedAt)); err != nil {
				return err
			}
			logger.Info("success comment posted",
				slog.String("job_url", job.URL),
				slog.Int("attempts", attempt),
			)
			return nil
		}

		select {
		case <-ctx.Done():
			if err := p.postFailure(task, regex, waitTimeout, attempt); err != nil {
				return err
			}
			logger.Info("failure comment posted",
				slog.Int("attempts", attempt),
				slog.String("regex", regex.String()),
			)
			return nil
		case <-ticker.C:
		}
	}

	if err := p.postFailure(task, regex, waitTimeout, attempt); err != nil {
		return err
	}
	logger.Info("failure comment posted",
		slog.Int("attempts", attempt),
		slog.String("regex", regex.String()),
	)
	return nil
}

func (p *Processor) postSuccess(task Task, job *jenkins.Job, regex *regexp.Regexp, waitTimeout time.Duration, attempts int, elapsed time.Duration) error {
	data := p.templateData(task, job)
	data.Regex = regex.String()
	data.Attempts = attempts
	data.WaitDuration = waitTimeout
	data.WaitDurationText = waitTimeout.String()
	data.Elapsed = elapsed
	data.ElapsedText = elapsed.String()

	commentTemplate := p.cfg.SuccessCommentTemplate(task.RepositoryConfig, task.Pattern)
	body, err := executeTemplate(commentTemplate, data)
	if err != nil {
		return fmt.Errorf("render success comment: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	return p.gitea.CreateComment(ctx, task.RepositoryFullName, task.PullRequest.Number, body)
}

func (p *Processor) postFailure(task Task, regex *regexp.Regexp, waitTimeout time.Duration, attempts int) error {
	data := p.templateData(task, nil)
	data.Regex = regex.String()
	data.Attempts = attempts
	data.WaitDuration = waitTimeout
	data.WaitDurationText = waitTimeout.String()
	data.Elapsed = waitTimeout
	data.ElapsedText = waitTimeout.String()

	commentTemplate := p.cfg.FailureCommentTemplate(task.RepositoryConfig, task.Pattern)
	body, err := executeTemplate(commentTemplate, data)
	if err != nil {
		return fmt.Errorf("render failure comment: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	return p.gitea.CreateComment(ctx, task.RepositoryFullName, task.PullRequest.Number, body)
}

func (p *Processor) templateData(task Task, job *jenkins.Job) *TemplateData {
	data := &TemplateData{
		Repository:       task.RepositoryFullName,
		PatternName:      task.Pattern.Name,
		PullRequest:      task.PullRequest.Number,
		PullRequestTitle: task.PullRequest.Title,
		PullRequestURL:   task.PullRequest.URL,
		SourceBranch:     task.PullRequest.SourceBranch,
		TargetBranch:     task.PullRequest.TargetBranch,
		ReceivedAt:       task.ReceivedAt,
	}
	if job != nil {
		data.JobName = job.Name
		data.JobURL = job.URL
	}
	return data
}

// TemplateData contains the values exposed to comment templates.
type TemplateData struct {
	Repository       string
	PatternName      string
	PullRequest      int
	PullRequestTitle string
	PullRequestURL   string
	SourceBranch     string
	TargetBranch     string
	JobName          string
	JobURL           string
	Regex            string
	Attempts         int
	WaitDuration     time.Duration
	WaitDurationText string
	Elapsed          time.Duration
	ElapsedText      string
	ReceivedAt       time.Time
}

func executeTemplate(tmpl string, data any) (string, error) {
	t, err := template.New("tmpl").Funcs(template.FuncMap{
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
	}).Parse(tmpl)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

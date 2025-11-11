package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	ListenAddr     string `yaml:"listen_addr"`
	WebhookSecret  string `yaml:"webhook_secret"`
	WorkerPoolSize int    `yaml:"worker_pool_size"`
	QueueSize      int    `yaml:"queue_size"`
}

type JenkinsConfig struct {
	BaseURL      string        `yaml:"base_url"`
	Username     string        `yaml:"username"`
	APIToken     string        `yaml:"api_token"`
	PollInterval time.Duration `yaml:"poll_interval"`
	Timeout      time.Duration `yaml:"timeout"`
	JobTree      string        `yaml:"job_tree"`
}

type GiteaConfig struct {
	BaseURL string `yaml:"base_url"`
	Token   string `yaml:"token"`
}

type RepositoryRule struct {
	Name                   string        `yaml:"name"`
	JobPatterns            []string      `yaml:"job_patterns"`
	PollInterval           time.Duration `yaml:"poll_interval"`
	Timeout                time.Duration `yaml:"timeout"`
	SuccessCommentTemplate string        `yaml:"success_comment_template"`
	FailureCommentTemplate string        `yaml:"failure_comment_template"`
}

type Config struct {
	Server       ServerConfig      `yaml:"server"`
	Jenkins      JenkinsConfig     `yaml:"jenkins"`
	Gitea        GiteaConfig       `yaml:"gitea"`
	Repositories []RepositoryRule  `yaml:"repositories"`
	RepoIndex    map[string]RepoID `yaml:"-"`
}

type RepoID struct {
	Rule RepositoryRule
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	cfg.buildIndex()
	return &cfg, nil
}

func (c *Config) Validate() error {
	if c.Server.ListenAddr == "" {
		c.Server.ListenAddr = ":8080"
	}
	if c.Server.WorkerPoolSize <= 0 {
		c.Server.WorkerPoolSize = 4
	}
	if c.Server.QueueSize <= 0 {
		c.Server.QueueSize = 100
	}

	if c.Jenkins.BaseURL == "" {
		return fmt.Errorf("jenkins.base_url must be provided")
	}
	if c.Jenkins.PollInterval <= 0 {
		c.Jenkins.PollInterval = 15 * time.Second
	}
	if c.Jenkins.Timeout <= 0 {
		c.Jenkins.Timeout = 5 * time.Minute
	}
	if c.Jenkins.JobTree == "" {
		c.Jenkins.JobTree = "jobs[name,url,fullName]"
	}

	if c.Gitea.BaseURL == "" {
		return fmt.Errorf("gitea.base_url must be provided")
	}
	if c.Gitea.Token == "" {
		return fmt.Errorf("gitea.token must be provided")
	}

	for idx := range c.Repositories {
		if c.Repositories[idx].Name == "" {
			return fmt.Errorf("repository rule at index %d missing name", idx)
		}
		if len(c.Repositories[idx].JobPatterns) == 0 {
			return fmt.Errorf("repository %s must define at least one job pattern", c.Repositories[idx].Name)
		}
		if c.Repositories[idx].PollInterval <= 0 {
			c.Repositories[idx].PollInterval = c.Jenkins.PollInterval
		}
		if c.Repositories[idx].Timeout <= 0 {
			c.Repositories[idx].Timeout = c.Jenkins.Timeout
		}
		if c.Repositories[idx].SuccessCommentTemplate == "" {
			c.Repositories[idx].SuccessCommentTemplate = "✅ Jenkins job {{ .JobName }} detected: {{ .JobURL }}"
		}
		if c.Repositories[idx].FailureCommentTemplate == "" {
			c.Repositories[idx].FailureCommentTemplate = "⚠️ Jenkins job not detected for PR {{ .Number }} within timeout ({{ .Timeout }})."
		}
	}

	return nil
}

func (c *Config) buildIndex() {
	c.RepoIndex = make(map[string]RepoID, len(c.Repositories))
	for _, repo := range c.Repositories {
		c.RepoIndex[repo.Name] = RepoID{Rule: repo}
	}
}

func (c *Config) GetRepositoryRule(fullName string) (RepositoryRule, bool) {
	if c.RepoIndex == nil {
		c.buildIndex()
	}
	repo, ok := c.RepoIndex[fullName]
	return repo.Rule, ok
}

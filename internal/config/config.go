package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration wraps time.Duration to support YAML unmarshalling from strings.
type Duration struct {
	time.Duration
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode && value.Value == "" {
		d.Duration = 0
		return nil
	}

	str := strings.TrimSpace(value.Value)
	if str == "" {
		d.Duration = 0
		return nil
	}

	parsed, err := time.ParseDuration(str)
	if err != nil {
		return fmt.Errorf("parse duration %q: %w", str, err)
	}

	d.Duration = parsed
	return nil
}

// Config represents the root of the service configuration.
type Config struct {
	Server       ServerConfig       `yaml:"server"`
	Processing   ProcessingConfig   `yaml:"processing"`
	Gitea        GiteaConfig        `yaml:"gitea"`
	Jenkins      JenkinsConfig      `yaml:"jenkins"`
	Repositories []RepositoryConfig `yaml:"repositories"`
	Logging      LoggingConfig      `yaml:"logging"`

	repoIndex map[string]*RepositoryConfig `yaml:"-"`
}

// ServerConfig controls HTTP server behaviour.
type ServerConfig struct {
	Address          string   `yaml:"address"`
	ReadTimeout      Duration `yaml:"read_timeout"`
	WriteTimeout     Duration `yaml:"write_timeout"`
	IdleTimeout      Duration `yaml:"idle_timeout"`
	WebhookSecretEnv string   `yaml:"webhook_secret_env"`
}

// ProcessingConfig controls background worker behaviour.
type ProcessingConfig struct {
	WorkerCount         int      `yaml:"worker_count"`
	QueueSize           int      `yaml:"queue_size"`
	DefaultWaitTimeout  Duration `yaml:"default_wait_timeout"`
	DefaultPollInterval Duration `yaml:"default_poll_interval"`
}

// LoggingConfig customises slog configuration.
type LoggingConfig struct {
	Level string `yaml:"level"`
}

// GiteaConfig contains settings for interacting with the Gitea API.
type GiteaConfig struct {
	BaseURL       string `yaml:"base_url"`
	Token         string `yaml:"token"`
	TokenEnv      string `yaml:"token_env"`
	SkipTLSVerify bool   `yaml:"skip_tls_verify"`
}

// JenkinsConfig contains Jenkins connection settings.
type JenkinsConfig struct {
	BaseURL       string `yaml:"base_url"`
	User          string `yaml:"user"`
	UserEnv       string `yaml:"user_env"`
	APIToken      string `yaml:"api_token"`
	APITokenEnv   string `yaml:"api_token_env"`
	SkipTLSVerify bool   `yaml:"skip_tls_verify"`
}

// RepositoryConfig configures PR handling per repository.
type RepositoryConfig struct {
	FullName               string          `yaml:"full_name"`
	Patterns               []PatternConfig `yaml:"patterns"`
	WaitTimeout            Duration        `yaml:"wait_timeout"`
	PollInterval           Duration        `yaml:"poll_interval"`
	SuccessCommentTemplate string          `yaml:"success_comment_template"`
	FailureCommentTemplate string          `yaml:"failure_comment_template"`
}

// PatternConfig defines a Jenkins job pattern for a repository.
type PatternConfig struct {
	Name                   string   `yaml:"name"`
	RegexTemplate          string   `yaml:"regex_template"`
	WaitTimeout            Duration `yaml:"wait_timeout"`
	PollInterval           Duration `yaml:"poll_interval"`
	SuccessCommentTemplate string   `yaml:"success_comment_template"`
	FailureCommentTemplate string   `yaml:"failure_comment_template"`
}

// Load reads configuration from the provided path.
func Load(path string) (*Config, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal yaml: %w", err)
	}

	cfg.applyDefaults()

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	cfg.indexRepositories()

	return &cfg, nil
}

// Repo looks up repository configuration by full name (owner/name).
func (c *Config) Repo(fullName string) (*RepositoryConfig, bool) {
	if c.repoIndex == nil {
		c.indexRepositories()
	}
	cfg, ok := c.repoIndex[strings.ToLower(fullName)]
	return cfg, ok
}

func (c *Config) indexRepositories() {
	c.repoIndex = make(map[string]*RepositoryConfig, len(c.Repositories))
	for i := range c.Repositories {
		repo := &c.Repositories[i]
		repo.FullName = strings.TrimSpace(repo.FullName)
		c.repoIndex[strings.ToLower(repo.FullName)] = repo
	}
}

func (c *Config) applyDefaults() {
	if c.Server.Address == "" {
		c.Server.Address = ":8080"
	}

	if c.Server.ReadTimeout.Duration == 0 {
		c.Server.ReadTimeout = Duration{Duration: 15 * time.Second}
	}
	if c.Server.WriteTimeout.Duration == 0 {
		c.Server.WriteTimeout = Duration{Duration: 15 * time.Second}
	}
	if c.Server.IdleTimeout.Duration == 0 {
		c.Server.IdleTimeout = Duration{Duration: 60 * time.Second}
	}

	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}

	if c.Processing.WorkerCount <= 0 {
		c.Processing.WorkerCount = 4
	}

	if c.Processing.QueueSize <= 0 {
		c.Processing.QueueSize = 128
	}

	if c.Processing.DefaultWaitTimeout.Duration == 0 {
		c.Processing.DefaultWaitTimeout = Duration{Duration: 5 * time.Minute}
	}

	if c.Processing.DefaultPollInterval.Duration == 0 {
		c.Processing.DefaultPollInterval = Duration{Duration: 15 * time.Second}
	}
}

func (c *Config) validate() error {
	if c.Gitea.BaseURL == "" {
		return errors.New("gitea.base_url is required")
	}
	if c.Jenkins.BaseURL == "" {
		return errors.New("jenkins.base_url is required")
	}
	if len(c.Repositories) == 0 {
		return errors.New("at least one repository config is required")
	}
	for i := range c.Repositories {
		if err := c.Repositories[i].validate(c); err != nil {
			return err
		}
	}
	return nil
}

func (r *RepositoryConfig) validate(_ *Config) error {
	if r.FullName == "" {
		return errors.New("repository.full_name is required")
	}
	if len(r.Patterns) == 0 {
		return fmt.Errorf("repository %s must define at least one pattern", r.FullName)
	}
	for i := range r.Patterns {
		if err := r.Patterns[i].validate(r); err != nil {
			return fmt.Errorf("repository %s pattern[%d]: %w", r.FullName, i, err)
		}
	}
	return nil
}

func (p *PatternConfig) validate(repo *RepositoryConfig) error {
	if p.RegexTemplate == "" {
		return errors.New("regex_template is required")
	}
	return nil
}

// ResolveToken returns the configured Gitea token using the precedence order.
func (c *GiteaConfig) ResolveToken() (string, error) {
	if c.Token != "" {
		return c.Token, nil
	}
	if c.TokenEnv == "" {
		return "", errors.New("gitea token or token_env must be provided")
	}
	val := strings.TrimSpace(os.Getenv(c.TokenEnv))
	if val == "" {
		return "", fmt.Errorf("gitea token environment variable %s is empty", c.TokenEnv)
	}
	return val, nil
}

// ResolveCredentials returns Jenkins user/token from config or environment.
func (c *JenkinsConfig) ResolveCredentials() (string, string, error) {
	user := c.User
	token := c.APIToken
	if user == "" && c.UserEnv != "" {
		user = strings.TrimSpace(os.Getenv(c.UserEnv))
	}
	if token == "" && c.APITokenEnv != "" {
		token = strings.TrimSpace(os.Getenv(c.APITokenEnv))
	}
	if user == "" {
		return "", "", errors.New("jenkins user or user_env must be provided")
	}
	if token == "" {
		return "", "", errors.New("jenkins api_token or api_token_env must be provided")
	}
	return user, token, nil
}

// WaitTimeoutForPattern returns the wait timeout for a pattern, falling back to repository and global defaults.
func (c *Config) WaitTimeoutForPattern(repo *RepositoryConfig, pattern *PatternConfig) time.Duration {
	if pattern.WaitTimeout.Duration > 0 {
		return pattern.WaitTimeout.Duration
	}
	if repo.WaitTimeout.Duration > 0 {
		return repo.WaitTimeout.Duration
	}
	return c.Processing.DefaultWaitTimeout.Duration
}

// PollIntervalForPattern returns the polling interval for a pattern.
func (c *Config) PollIntervalForPattern(repo *RepositoryConfig, pattern *PatternConfig) time.Duration {
	if pattern.PollInterval.Duration > 0 {
		return pattern.PollInterval.Duration
	}
	if repo.PollInterval.Duration > 0 {
		return repo.PollInterval.Duration
	}
	return c.Processing.DefaultPollInterval.Duration
}

func (c *Config) SuccessCommentTemplate(repo *RepositoryConfig, pattern *PatternConfig) string {
	if pattern.SuccessCommentTemplate != "" {
		return pattern.SuccessCommentTemplate
	}
	if repo.SuccessCommentTemplate != "" {
		return repo.SuccessCommentTemplate
	}
	return "Jenkins job detected: {{ .JobURL }}"
}

func (c *Config) FailureCommentTemplate(repo *RepositoryConfig, pattern *PatternConfig) string {
	if pattern.FailureCommentTemplate != "" {
		return pattern.FailureCommentTemplate
	}
	if repo.FailureCommentTemplate != "" {
		return repo.FailureCommentTemplate
	}
	return "Jenkins job was not found within {{ .WaitDurationText }}."
}

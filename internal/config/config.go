package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server       ServerConfig       `yaml:"server"`
	Gitea        GiteaConfig        `yaml:"gitea"`
	Jenkins      JenkinsConfig      `yaml:"jenkins"`
	Webhook      WebhookConfig      `yaml:"webhook"`
	Repositories []RepositoryConfig `yaml:"repositories"`
}

type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

type GiteaConfig struct {
	BaseURL string `yaml:"base_url"`
	Token   string `yaml:"token"`
}

type JenkinsConfig struct {
	BaseURL  string `yaml:"base_url"`
	Username string `yaml:"username"`
	Token    string `yaml:"token"`
}

type WebhookConfig struct {
	TimeoutSeconds int `yaml:"timeout_seconds"`
}

type RepositoryConfig struct {
	Owner          string `yaml:"owner"`
	Name           string `yaml:"name"`
	JobPattern     string `yaml:"job_pattern"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

// LoadConfig загружает конфигурацию из файла
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Валидация конфигурации
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &config, nil
}

// Validate проверяет корректность конфигурации
func (c *Config) Validate() error {
	if c.Server.Port <= 0 {
		return fmt.Errorf("server port must be positive")
	}

	if c.Gitea.BaseURL == "" {
		return fmt.Errorf("gitea base_url is required")
	}

	if c.Gitea.Token == "" {
		return fmt.Errorf("gitea token is required")
	}

	if c.Jenkins.BaseURL == "" {
		return fmt.Errorf("jenkins base_url is required")
	}

	if c.Jenkins.Username == "" {
		return fmt.Errorf("jenkins username is required")
	}

	if c.Jenkins.Token == "" {
		return fmt.Errorf("jenkins token is required")
	}

	if c.Webhook.TimeoutSeconds <= 0 {
		return fmt.Errorf("webhook timeout_seconds must be positive")
	}

	return nil
}

// GetRepositoryConfig возвращает конфигурацию для конкретного репозитория
func (c *Config) GetRepositoryConfig(owner, name string) *RepositoryConfig {
	for _, repo := range c.Repositories {
		if repo.Owner == owner && repo.Name == name {
			return &repo
		}
	}
	return nil
}

// GetTimeout возвращает таймаут для репозитория или дефолтный
func (c *Config) GetTimeout(repoConfig *RepositoryConfig) time.Duration {
	if repoConfig != nil && repoConfig.TimeoutSeconds > 0 {
		return time.Duration(repoConfig.TimeoutSeconds) * time.Second
	}
	return time.Duration(c.Webhook.TimeoutSeconds) * time.Second
}

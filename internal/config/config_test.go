package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	// Создаем временный конфиг файл
	configContent := `
server:
  port: 8080
  host: "0.0.0.0"

gitea:
  base_url: "http://gitea:3000"
  token: "test-token"

jenkins:
  base_url: "http://jenkins:8080"
  username: "admin"
  token: "jenkins-token"

webhook:
  timeout_seconds: 300

repositories:
  - owner: "test-org"
    name: "test-repo"
    job_pattern: "^test-repo-pr-\\d+$"
    timeout_seconds: 300
`

	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(configContent)
	require.NoError(t, err)
	tmpFile.Close()

	cfg, err := LoadConfig(tmpFile.Name())
	require.NoError(t, err)
	assert.NotNil(t, cfg)

	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "0.0.0.0", cfg.Server.Host)
	assert.Equal(t, "http://gitea:3000", cfg.Gitea.BaseURL)
	assert.Equal(t, "test-token", cfg.Gitea.Token)
	assert.Equal(t, "http://jenkins:8080", cfg.Jenkins.BaseURL)
	assert.Equal(t, 300, cfg.Webhook.TimeoutSeconds)
	assert.Len(t, cfg.Repositories, 1)
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				Server: ServerConfig{Port: 8080, Host: "0.0.0.0"},
				Gitea:  GiteaConfig{BaseURL: "http://gitea:3000", Token: "token"},
				Jenkins: JenkinsConfig{
					BaseURL:  "http://jenkins:8080",
					Username: "admin",
					Token:    "token",
				},
				Webhook: WebhookConfig{TimeoutSeconds: 300},
			},
			wantErr: false,
		},
		{
			name: "invalid port",
			config: Config{
				Server: ServerConfig{Port: -1},
				Gitea:  GiteaConfig{BaseURL: "http://gitea:3000", Token: "token"},
				Jenkins: JenkinsConfig{
					BaseURL:  "http://jenkins:8080",
					Username: "admin",
					Token:    "token",
				},
				Webhook: WebhookConfig{TimeoutSeconds: 300},
			},
			wantErr: true,
		},
		{
			name: "missing gitea base_url",
			config: Config{
				Server: ServerConfig{Port: 8080},
				Gitea:  GiteaConfig{Token: "token"},
				Jenkins: JenkinsConfig{
					BaseURL:  "http://jenkins:8080",
					Username: "admin",
					Token:    "token",
				},
				Webhook: WebhookConfig{TimeoutSeconds: 300},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfig_GetRepositoryConfig(t *testing.T) {
	cfg := &Config{
		Repositories: []RepositoryConfig{
			{Owner: "org1", Name: "repo1", JobPattern: "pattern1"},
			{Owner: "org2", Name: "repo2", JobPattern: "pattern2"},
		},
	}

	tests := []struct {
		name     string
		owner    string
		repoName string
		want     *RepositoryConfig
	}{
		{
			name:     "found",
			owner:    "org1",
			repoName: "repo1",
			want:     &RepositoryConfig{Owner: "org1", Name: "repo1", JobPattern: "pattern1"},
		},
		{
			name:     "not found",
			owner:    "org3",
			repoName: "repo3",
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cfg.GetRepositoryConfig(tt.owner, tt.repoName)
			if tt.want == nil {
				assert.Nil(t, got)
			} else {
				assert.Equal(t, tt.want.Owner, got.Owner)
				assert.Equal(t, tt.want.Name, got.Name)
				assert.Equal(t, tt.want.JobPattern, got.JobPattern)
			}
		})
	}
}

func TestConfig_GetTimeout(t *testing.T) {
	cfg := &Config{
		Webhook: WebhookConfig{TimeoutSeconds: 300},
		Repositories: []RepositoryConfig{
			{Owner: "org1", Name: "repo1", TimeoutSeconds: 600},
		},
	}

	tests := []struct {
		name       string
		repoConfig *RepositoryConfig
		want       time.Duration
	}{
		{
			name:       "with repo config",
			repoConfig: &RepositoryConfig{TimeoutSeconds: 600},
			want:       600 * time.Second,
		},
		{
			name:       "without repo config",
			repoConfig: nil,
			want:       300 * time.Second,
		},
		{
			name:       "repo config with zero timeout",
			repoConfig: &RepositoryConfig{TimeoutSeconds: 0},
			want:       300 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cfg.GetTimeout(tt.repoConfig)
			assert.Equal(t, tt.want, got)
		})
	}
}

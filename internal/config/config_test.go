package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitea_jenkins_integ/internal/config"
	"gopkg.in/yaml.v3"
)

func TestDurationUnmarshal(t *testing.T) {
	type wrapper struct {
		Value config.Duration `yaml:"value"`
	}

	data := []byte("value: 5s")
	var w wrapper
	if err := yaml.Unmarshal(data, &w); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.Value.Duration != 5*time.Second {
		t.Fatalf("expected 5s, got %v", w.Value.Duration)
	}
}

func TestLoad(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	const content = `
server:
  address: ":9999"
gitea:
  base_url: "https://gitea.example.com"
  token: "dummy"
jenkins:
  base_url: "https://jenkins.example.com"
  user: "user"
  api_token: "token"
repositories:
  - full_name: "org/repo"
    patterns:
      - name: "default"
        regex_template: "^build-pr-{{ .PullRequest }}$"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Server.Address != ":9999" {
		t.Fatalf("unexpected server address: %s", cfg.Server.Address)
	}

	repo, ok := cfg.Repo("org/repo")
	if !ok {
		t.Fatalf("expected repo configuration")
	}

	if got := cfg.SuccessCommentTemplate(repo, &repo.Patterns[0]); got == "" {
		t.Fatalf("expected default success template")
	}

	wait := cfg.WaitTimeoutForPattern(repo, &repo.Patterns[0])
	if wait <= 0 {
		t.Fatalf("expected wait timeout > 0")
	}
}

func TestResolveTokenFromEnv(t *testing.T) {
	cfg := config.GiteaConfig{
		TokenEnv: "GITEA_TOKEN_TEST",
	}
	t.Setenv("GITEA_TOKEN_TEST", "secret")

	token, err := cfg.ResolveToken()
	if err != nil {
		t.Fatalf("ResolveToken error: %v", err)
	}
	if token != "secret" {
		t.Fatalf("unexpected token %q", token)
	}
}

func TestResolveCredentialsFromEnv(t *testing.T) {
	cfg := config.JenkinsConfig{
		UserEnv:     "JENKINS_USER_TEST",
		APITokenEnv: "JENKINS_TOKEN_TEST",
	}
	t.Setenv("JENKINS_USER_TEST", "user")
	t.Setenv("JENKINS_TOKEN_TEST", "token")

	user, token, err := cfg.ResolveCredentials()
	if err != nil {
		t.Fatalf("ResolveCredentials error: %v", err)
	}
	if user != "user" || token != "token" {
		t.Fatalf("unexpected credentials %q %q", user, token)
	}
}

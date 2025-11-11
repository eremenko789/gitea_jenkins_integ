package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/example/gitea-jenkins-webhook/internal/config"
)

func TestLoad(t *testing.T) {
	cfgContent := `
server:
  listen_addr: ":9000"
jenkins:
  base_url: "https://jenkins.example.com"
  username: "john"
  api_token: "token"
gitea:
  base_url: "https://gitea.example.com"
  token: "secret"
repositories:
  - name: "org/repo"
    job_patterns:
      - "^build-{{ .Number }}$"
    poll_interval: 1s
    timeout: 5s
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(cfgContent), 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.Server.ListenAddr != ":9000" {
		t.Fatalf("unexpected listen addr: %s", cfg.Server.ListenAddr)
	}
	if cfg.Server.WorkerPoolSize != 4 {
		t.Fatalf("default worker pool should be 4, got %d", cfg.Server.WorkerPoolSize)
	}
	if cfg.Repositories[0].PollInterval != time.Second {
		t.Fatalf("expected poll interval of 1s, got %s", cfg.Repositories[0].PollInterval)
	}
	if _, ok := cfg.GetRepositoryRule("org/repo"); !ok {
		t.Fatalf("expected repository rule to be registered")
	}
}

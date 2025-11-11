package webhook

import (
	"testing"

	"github.com/gitea-jenkins-integ/internal/config"
	"github.com/gitea-jenkins-integ/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestHandler_buildJobPattern(t *testing.T) {
	cfg := &config.Config{
		Webhook: config.WebhookConfig{TimeoutSeconds: 300},
	}
	handler := NewHandler(cfg)

	tests := []struct {
		name     string
		pattern  string
		prNumber int
		want     string
	}{
		{
			name:     "with placeholder",
			pattern:  "^test-repo-pr-{pr_number}$",
			prNumber: 123,
			want:     "^test-repo-pr-123$",
		},
		{
			name:     "without placeholder",
			pattern:  "^test-repo-pr-\\d+$",
			prNumber: 123,
			want:     "^test-repo-pr-\\d+$",
		},
		{
			name:     "multiple placeholders",
			pattern:  "pr-{pr_number}-build-{pr_number}",
			prNumber: 456,
			want:     "pr-456-build-456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handler.buildJobPattern(tt.pattern, tt.prNumber)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHandler_HandleWebhook_IgnoresNonOpenedActions(t *testing.T) {
	cfg := &config.Config{
		Webhook: config.WebhookConfig{TimeoutSeconds: 300},
	}
	handler := NewHandler(cfg)

	payload := &models.GiteaWebhookPayload{
		Action: "closed",
	}

	err := handler.HandleWebhook(payload)
	assert.NoError(t, err) // Должен игнорировать без ошибки
}

func TestHandler_HandleWebhook_IgnoresUnconfiguredRepos(t *testing.T) {
	cfg := &config.Config{
		Webhook: config.WebhookConfig{TimeoutSeconds: 300},
		Repositories: []config.RepositoryConfig{
			{Owner: "test-org", Name: "test-repo"},
		},
	}
	handler := NewHandler(cfg)

	payload := &models.GiteaWebhookPayload{
		Action:      "opened",
		PullRequest: &models.PullRequest{Number: 1},
		Repository: &models.Repository{
			Owner: &models.Owner{Login: "other-org"},
			Name:  "other-repo",
		},
	}

	err := handler.HandleWebhook(payload)
	assert.NoError(t, err) // Должен игнорировать без ошибки
}

// Package gitea предоставляет клиент для взаимодействия с API Gitea.
package gitea

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Client представляет клиент для работы с API Gitea.
type Client struct {
	baseURL string
	token   string
	client  *http.Client
	log     *slog.Logger
}

// commentRequest представляет запрос на создание комментария в Gitea.
type commentRequest struct {
	Body string `json:"body"` // Текст комментария
}

// NewClient создает новый клиент для работы с API Gitea.
// Если httpClient равен nil, создается клиент с таймаутом 10 секунд.
// Если logger равен nil, используется логгер по умолчанию.
func NewClient(baseURL, token string, httpClient *http.Client, logger *slog.Logger) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		client:  httpClient,
		log:     logger,
	}
}

// PostComment публикует комментарий в указанном issue или pull request репозитория Gitea.
// repoFullName должен быть в формате "owner/repo", issueIndex - номер issue/PR.
func (c *Client) PostComment(ctx context.Context, repoFullName string, issueIndex int64, body string) error {
	c.log.Info("posting comment to Gitea",
		"repo", repoFullName,
		"issue_index", issueIndex,
		"comment_length", len(body))

	owner, repo, err := splitRepoFullName(repoFullName)
	if err != nil {
		c.log.Error("failed to split repo full name", "err", err, "repo", repoFullName)
		return err
	}

	path := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", c.baseURL, owner, repo, issueIndex)
	payload := commentRequest{Body: body}
	data, err := json.Marshal(payload)
	if err != nil {
		c.log.Error("failed to marshal comment payload", "err", err)
		return fmt.Errorf("marshal comment payload: %w", err)
	}

	c.log.Debug("Gitea request prepared",
		"method", http.MethodPost,
		"url", path,
		"payload", string(data))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, bytes.NewReader(data))
	if err != nil {
		c.log.Error("failed to create request", "err", err)
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("token %s", c.token))

	c.log.Debug("Gitea request headers",
		"content_type", req.Header.Get("Content-Type"),
		"authorization", "token ***",
		"url", path)

	resp, err := c.client.Do(req)
	if err != nil {
		c.log.Error("failed to execute Gitea request", "err", err, "url", path)
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	c.log.Debug("Gitea response received",
		"status_code", resp.StatusCode,
		"status", resp.Status,
		"headers", resp.Header,
		"body", string(respBody),
		"body_length", len(respBody))

	if resp.StatusCode >= 400 {
		c.log.Error("Gitea API error",
			"status_code", resp.StatusCode,
			"status", resp.Status,
			"response_body", string(respBody))
		return fmt.Errorf("post comment failed: status %s", resp.Status)
	}

	c.log.Info("comment posted to Gitea successfully",
		"repo", repoFullName,
		"issue_index", issueIndex,
		"status_code", resp.StatusCode)
	return nil
}

// splitRepoFullName разделяет полное имя репозитория (формат "owner/repo") на владельца и имя репозитория.
func splitRepoFullName(fullName string) (string, string, error) {
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repo full name: %s", fullName)
	}
	return parts[0], parts[1], nil
}

// CheckAccessibility проверяет доступность Gitea, выполняя запрос к эндпоинту /user.
// Возвращает ошибку, если Gitea недоступен или аутентификация не удалась.
func (c *Client) CheckAccessibility(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	endpoint := fmt.Sprintf("%s/user", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("token %s", c.token))

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("gitea api request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("authentication failed: status %s", resp.Status)
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("gitea api not found: status %s", resp.Status)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("gitea api error: status %s", resp.Status)
	}

	return nil
}

// GetRepository проверяет существование репозитория в Gitea.
// Возвращает ошибку, если репозиторий не найден, доступ запрещен или произошла другая ошибка API.
func (c *Client) GetRepository(ctx context.Context, owner, repo string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	endpoint := fmt.Sprintf("%s/repos/%s/%s", c.baseURL, owner, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("token %s", c.token))

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("gitea api request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("repository not found: status %s", resp.Status)
	}
	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("access denied to repository: status %s", resp.Status)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("authentication failed: status %s", resp.Status)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("gitea api error: status %s", resp.Status)
	}

	return nil
}

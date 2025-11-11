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

type Client struct {
	baseURL string
	token   string
	client  *http.Client
	log     *slog.Logger
}

type commentRequest struct {
	Body string `json:"body"`
}

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

func splitRepoFullName(fullName string) (string, string, error) {
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repo full name: %s", fullName)
	}
	return parts[0], parts[1], nil
}

package gitea

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"log/slog"
)

// Client interacts with the Gitea REST API.
type Client struct {
	baseURL    *url.URL
	token      string
	httpClient *http.Client
	logger     *slog.Logger
}

type commentRequest struct {
	Body string `json:"body"`
}

// New creates a new Gitea API client.
func New(baseURL, token string, httpClient *http.Client, logger *slog.Logger) (*Client, error) {
	if baseURL == "" {
		return nil, errors.New("gitea baseURL is required")
	}
	if token == "" {
		return nil, errors.New("gitea token is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse gitea base URL: %w", err)
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		baseURL:    parsed,
		token:      token,
		httpClient: httpClient,
		logger:     logger,
	}, nil
}

// CreateComment posts a comment to the specified pull request.
func (c *Client) CreateComment(ctx context.Context, repoFullName string, prNumber int, body string) error {
	if repoFullName == "" {
		return errors.New("repoFullName is required")
	}
	if prNumber <= 0 {
		return fmt.Errorf("invalid pull request number: %d", prNumber)
	}
	trimmed := strings.TrimSuffix(repoFullName, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 {
		return fmt.Errorf("repository full name must be in the form owner/name, got %q", repoFullName)
	}
	owner := url.PathEscape(parts[0])
	name := url.PathEscape(strings.Join(parts[1:], "/"))
	path := fmt.Sprintf("/api/v1/repos/%s/%s/issues/%d/comments", owner, name, prNumber)
	endpoint := c.baseURL.ResolveReference(&url.URL{Path: path})

	payload, err := json.Marshal(commentRequest{Body: body})
	if err != nil {
		return fmt.Errorf("marshal comment body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gitea request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("gitea returned status %s", resp.Status)
	}

	return nil
}

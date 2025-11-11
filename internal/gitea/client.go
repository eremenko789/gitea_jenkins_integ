package gitea

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	token   string
	client  *http.Client
}

type commentRequest struct {
	Body string `json:"body"`
}

func NewClient(baseURL, token string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		client:  httpClient,
	}
}

func (c *Client) PostComment(ctx context.Context, repoFullName string, issueIndex int64, body string) error {
	owner, repo, err := splitRepoFullName(repoFullName)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", c.baseURL, owner, repo, issueIndex)
	payload := commentRequest{Body: body}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal comment payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("token %s", c.token))

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("post comment failed: status %s", resp.Status)
	}

	return nil
}

func splitRepoFullName(fullName string) (string, string, error) {
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repo full name: %s", fullName)
	}
	return parts[0], parts[1], nil
}

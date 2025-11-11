package jenkins

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	username   string
	apiToken   string
	jobTree    string
	httpClient *http.Client
}

type Job struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	FullName string `json:"fullName"`
}

type jobsResponse struct {
	Jobs []Job `json:"jobs"`
}

func NewClient(baseURL, username, apiToken, jobTree string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	if jobTree == "" {
		jobTree = "jobs[name,url,fullName]"
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		username:   username,
		apiToken:   apiToken,
		jobTree:    jobTree,
		httpClient: httpClient,
	}
}

func (c *Client) WaitForJob(ctx context.Context, pattern *regexp.Regexp, timeout, interval time.Duration) (*Job, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		job, err := c.findJob(ctx, pattern)
		if err != nil {
			return nil, err
		}
		if job != nil {
			return job, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *Client) findJob(ctx context.Context, pattern *regexp.Regexp) (*Job, error) {
	endpoint, err := url.Parse(fmt.Sprintf("%s/api/json", c.baseURL))
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}

	query := endpoint.Query()
	query.Set("tree", c.jobTree)
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if c.username != "" || c.apiToken != "" {
		req.SetBasicAuth(c.username, c.apiToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jenkins api request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("jenkins api status: %s", resp.Status)
	}

	var jobs jobsResponse
	if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
		return nil, fmt.Errorf("decode jenkins response: %w", err)
	}

	for _, job := range jobs.Jobs {
		if pattern.MatchString(job.Name) || pattern.MatchString(job.FullName) {
			return &job, nil
		}
	}

	return nil, nil
}

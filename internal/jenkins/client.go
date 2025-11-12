package jenkins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
	httpClient *http.Client
	log        *slog.Logger
}

type Job struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	FullName string `json:"fullName"`
}

type jobsResponse struct {
	Jobs []Job `json:"jobs"`
}

func NewClient(baseURL string, username string, apiToken string, httpClient *http.Client, logger *slog.Logger) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		username:   username,
		apiToken:   apiToken,
		httpClient: httpClient,
		log:        logger,
	}
}

func (c *Client) WaitForJob(ctx context.Context, pattern *regexp.Regexp, jobRoot string, timeout, interval time.Duration) (*Job, error) {
	c.log.Debug("waiting for Jenkins job",
		"pattern", pattern.String(),
		"job_root", jobRoot,
		"timeout", timeout,
		"poll_interval", interval)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	attempt := 0
	for {
		attempt++
		c.log.Debug("polling Jenkins for job", "attempt", attempt, "pattern", pattern.String(), "job_root", jobRoot)

		job, err := c.findJob(ctx, pattern, jobRoot)
		if err != nil {
			c.log.Debug("error finding job", "err", err, "attempt", attempt)
			return nil, err
		}
		if job != nil {
			c.log.Info("Jenkins job found",
				"job", job.Name,
				"url", job.URL,
				"full_name", job.FullName,
				"attempt", attempt)
			return job, nil
		}

		c.log.Debug("job not found, waiting for next poll", "attempt", attempt, "interval", interval)

		select {
		case <-ctx.Done():
			c.log.Debug("waiting for job cancelled or timeout", "err", ctx.Err(), "attempt", attempt)
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *Client) findJob(ctx context.Context, pattern *regexp.Regexp, jobRoot string) (*Job, error) {
	jobs, err := c.GetJobs(ctx, jobRoot)
	if err != nil {
		return nil, err
	}

	c.log.Debug("Jenkins jobs retrieved",
		"jobs_count", len(jobs),
		"pattern", pattern.String(),
		"job_root", jobRoot)

	for _, job := range jobs {
		matchesName := pattern.MatchString(job.Name)
		matchesFullName := pattern.MatchString(job.FullName)
		c.log.Debug("checking job against pattern",
			"job_name", job.Name,
			"job_full_name", job.FullName,
			"pattern", pattern.String(),
			"matches_name", matchesName,
			"matches_full_name", matchesFullName)

		if matchesName || matchesFullName {
			c.log.Debug("job matched pattern",
				"job_name", job.Name,
				"job_full_name", job.FullName,
				"job_url", job.URL)
			return &job, nil
		}
	}

	c.log.Debug("no jobs matched pattern", "pattern", pattern.String(), "jobs_checked", len(jobs))
	return nil, nil
}

// CheckAccessibility checks if Jenkins is accessible by making a request to /api/json
func (c *Client) CheckAccessibility(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	endpoint := fmt.Sprintf("%s/api/json", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	if c.username != "" || c.apiToken != "" {
		req.SetBasicAuth(c.username, c.apiToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("jenkins api request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("authentication failed: status %s", resp.Status)
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("jenkins not found: status %s", resp.Status)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("jenkins api error: status %s", resp.Status)
	}

	return nil
}

// GetJobs retrieves a list of jobs from the specified job root
func (c *Client) GetJobs(ctx context.Context, jobRoot string) ([]Job, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	apiPath := "/api/json"
	if jobRoot != "" {
		parts := strings.Split(strings.Trim(jobRoot, "/"), "/")
		var pathBuilder strings.Builder
		for _, part := range parts {
			if part != "" {
				pathBuilder.WriteString("/job/")
				pathBuilder.WriteString(part)
			}
		}
		pathBuilder.WriteString(apiPath)
		apiPath = pathBuilder.String()
	}

	endpoint, err := url.Parse(fmt.Sprintf("%s%s", c.baseURL, apiPath))
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}

	query := endpoint.Query()
	query.Set("tree", "jobs[name,url,fullName]")
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

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("jenkins api status: %s", resp.Status)
	}

	var jobs jobsResponse
	if err := json.NewDecoder(bytes.NewReader(respBody)).Decode(&jobs); err != nil {
		return nil, fmt.Errorf("decode jenkins response: %w", err)
	}

	return jobs.Jobs, nil
}

// CheckJobRootExists checks if the specified job root exists in Jenkins
func (c *Client) CheckJobRootExists(ctx context.Context, jobRoot string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if jobRoot == "" {
		return nil // Empty job root is valid (means root level)
	}

	parts := strings.Split(strings.Trim(jobRoot, "/"), "/")
	var pathBuilder strings.Builder
	for _, part := range parts {
		if part != "" {
			pathBuilder.WriteString("/job/")
			pathBuilder.WriteString(part)
		}
	}
	pathBuilder.WriteString("/api/json")
	apiPath := pathBuilder.String()

	endpoint := fmt.Sprintf("%s%s", c.baseURL, apiPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	if c.username != "" || c.apiToken != "" {
		req.SetBasicAuth(c.username, c.apiToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("jenkins api request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("job root not found: status %s", resp.Status)
	}
	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("access denied to job root: status %s", resp.Status)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("jenkins api error: status %s", resp.Status)
	}

	return nil
}

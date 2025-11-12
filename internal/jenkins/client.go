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
	// Build URL with JobRoot if provided
	apiPath := "/api/json"
	if jobRoot != "" {
		// Split JobRoot by "/" and build path: /job/part1/job/part2/api/json
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
		c.log.Error("failed to parse Jenkins base URL", "err", err, "base_url", c.baseURL, "api_path", apiPath)
		return nil, fmt.Errorf("parse base url: %w", err)
	}

	query := endpoint.Query()
	query.Set("tree", "jobs[name,url,fullName]")
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		c.log.Error("failed to create Jenkins request", "err", err)
		return nil, fmt.Errorf("create request: %w", err)
	}

	if c.username != "" || c.apiToken != "" {
		req.SetBasicAuth(c.username, c.apiToken)
		c.log.Debug("Jenkins request with basic auth",
			"username", c.username,
			"has_token", c.apiToken != "")
	}

	c.log.Debug("Jenkins request prepared",
		"method", http.MethodGet,
		"url", endpoint.String())

	authHeader := req.Header.Get("Authorization")
	if authHeader != "" {
		c.log.Debug("Jenkins request headers",
			"authorization", "Basic ***",
			"url", endpoint.String())
	} else {
		c.log.Debug("Jenkins request headers",
			"authorization", "none",
			"url", endpoint.String())
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Error("failed to execute Jenkins request", "err", err, "url", endpoint.String())
		return nil, fmt.Errorf("jenkins api request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	c.log.Debug("Jenkins response received",
		"status_code", resp.StatusCode,
		"status", resp.Status,
		"headers", resp.Header,
		"body_length", len(respBody))

	if resp.StatusCode >= 400 {
		c.log.Error("Jenkins API error",
			"status_code", resp.StatusCode,
			"status", resp.Status,
			"response_body", string(respBody))
		return nil, fmt.Errorf("jenkins api status: %s", resp.Status)
	}

	var jobs jobsResponse
	if err := json.NewDecoder(bytes.NewReader(respBody)).Decode(&jobs); err != nil {
		c.log.Error("failed to decode Jenkins response", "err", err, "body", string(respBody))
		return nil, fmt.Errorf("decode jenkins response: %w", err)
	}

	c.log.Debug("Jenkins jobs decoded",
		"jobs_count", len(jobs.Jobs),
		"pattern", pattern.String())

	for _, job := range jobs.Jobs {
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

	c.log.Debug("no jobs matched pattern", "pattern", pattern.String(), "jobs_checked", len(jobs.Jobs))
	return nil, nil
}

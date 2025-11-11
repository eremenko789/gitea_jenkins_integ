package jenkins

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"log/slog"
)

const (
	maxFolderDepth = 3
)

// Client communicates with the Jenkins REST API.
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
	authHeader string
	logger     *slog.Logger
}

// Job represents a Jenkins job with its name and URL.
type Job struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type apiJob struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Class string `json:"_class"`
}

type apiResponse struct {
	Jobs []apiJob `json:"jobs"`
}

// New creates a Jenkins API client.
func New(baseURL, username, token string, httpClient *http.Client, logger *slog.Logger) (*Client, error) {
	if baseURL == "" {
		return nil, errors.New("jenkins baseURL is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse jenkins base URL: %w", err)
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if logger == nil {
		logger = slog.Default()
	}

	cred := username + ":" + token
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte(cred))

	return &Client{
		baseURL:    parsed,
		httpClient: httpClient,
		authHeader: authHeader,
		logger:     logger,
	}, nil
}

// FindJob returns the first job whose name matches the supplied regular expression.
func (c *Client) FindJob(ctx context.Context, rx *regexp.Regexp) (*Job, bool, error) {
	jobs, err := c.ListJobs(ctx)
	if err != nil {
		return nil, false, err
	}
	for _, job := range jobs {
		if rx.MatchString(job.Name) {
			return &job, true, nil
		}
	}
	return nil, false, nil
}

// ListJobs retrieves all top-level and nested jobs (up to maxFolderDepth).
func (c *Client) ListJobs(ctx context.Context) ([]Job, error) {
	var result []Job
	visited := make(map[string]struct{})
	if err := c.collectJobs(ctx, c.baseURL, 0, visited, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) collectJobs(ctx context.Context, current *url.URL, depth int, visited map[string]struct{}, accumulator *[]Job) error {
	if depth > maxFolderDepth {
		return nil
	}
	apiURL := current.ResolveReference(&url.URL{Path: strings.TrimSuffix(current.Path, "/") + "/api/json"})
	q := apiURL.Query()
	q.Set("tree", "jobs[name,url,_class]")
	apiURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL.String(), nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("jenkins api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("jenkins api returned status %s", resp.Status)
	}

	var payload apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("decode jenkins response: %w", err)
	}

	for _, job := range payload.Jobs {
		if _, seen := visited[job.URL]; seen {
			continue
		}
		visited[job.URL] = struct{}{}
		*accumulator = append(*accumulator, Job{Name: job.Name, URL: job.URL})

		if strings.Contains(strings.ToLower(job.Class), "folder") {
			jobURL, err := url.Parse(job.URL)
			if err != nil {
				c.logger.Warn("invalid job URL", slog.String("url", job.URL), slog.String("error", err.Error()))
				continue
			}
			if err := c.collectJobs(ctx, jobURL, depth+1, visited, accumulator); err != nil {
				return err
			}
		}
	}

	return nil
}

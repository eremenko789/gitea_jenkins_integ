package jenkins

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"
)

type Client struct {
	baseURL  string
	username string
	token    string
	client   *http.Client
}

func NewClient(baseURL, username, token string) *Client {
	return &Client{
		baseURL:  baseURL,
		username: username,
		token:    token,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// FindJobByPattern ищет джобу по регулярному выражению
func (c *Client) FindJobByPattern(pattern string) (*Job, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}

	jobs, err := c.ListJobs()
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}

	for _, job := range jobs {
		if re.MatchString(job.Name) {
			return &job, nil
		}
	}

	return nil, nil // Джоба не найдена
}

// ListJobs получает список всех джоб из Jenkins
func (c *Client) ListJobs() ([]Job, error) {
	url := fmt.Sprintf("%s/api/json?tree=jobs[name,url,color]", c.baseURL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.username, c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("jenkins API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var jobList JobList
	if err := json.NewDecoder(resp.Body).Decode(&jobList); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return jobList.Jobs, nil
}

// WaitForJob ожидает появления джобы по паттерну в течение указанного времени
func (c *Client) WaitForJob(pattern string, timeout time.Duration) (*Job, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			job, err := c.FindJobByPattern(pattern)
			if err != nil {
				return nil, err
			}
			if job != nil {
				return job, nil
			}

			if time.Now().After(deadline) {
				return nil, nil // Таймаут, джоба не найдена
			}
		case <-time.After(timeout):
			return nil, nil // Таймаут
		}
	}
}

type Job struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Color string `json:"color"`
}

type JobList struct {
	Jobs []Job `json:"jobs"`
}

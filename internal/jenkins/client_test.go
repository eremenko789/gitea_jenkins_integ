package jenkins_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync/atomic"
	"testing"
	"time"

	"github.com/example/gitea-jenkins-webhook/internal/jenkins"
)

func TestWaitForJob(t *testing.T) {
	var callCount int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		var jobs []jenkins.Job
		if count >= 2 {
			jobs = []jenkins.Job{{Name: "job-123", URL: "http://jenkins/job-123"}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jobs": jobs,
		})
	}))
	defer ts.Close()

	client := jenkins.NewClient(ts.URL, "user", "token", "", &http.Client{
		Timeout: time.Second,
	}, nil)

	ctx := context.Background()
	re := regexp.MustCompile(`job-123`)
	job, err := client.WaitForJob(ctx, re, "", 2*time.Second, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if job == nil || job.Name != "job-123" {
		t.Fatalf("unexpected job: %#v", job)
	}
}

func TestWaitForJobTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jobs": []jenkins.Job{},
		})
	}))
	defer ts.Close()

	client := jenkins.NewClient(ts.URL, "", "", "", &http.Client{Timeout: time.Second}, nil)
	ctx := context.Background()
	re := regexp.MustCompile(`job`)
	_, err := client.WaitForJob(ctx, re, "", 300*time.Millisecond, 100*time.Millisecond)
	if err == nil {
		t.Fatalf("expected timeout error")
	}
}

func TestWaitForJobWithJobRoot(t *testing.T) {
	var requestedPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		jobs := []jenkins.Job{{Name: "test-job", URL: "http://jenkins/test-job"}}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jobs": jobs,
		})
	}))
	defer ts.Close()

	client := jenkins.NewClient(ts.URL, "user", "token", "", &http.Client{
		Timeout: time.Second,
	}, nil)

	ctx := context.Background()
	re := regexp.MustCompile(`test-job`)
	job, err := client.WaitForJob(ctx, re, "test_webhook/test_webhooks", 2*time.Second, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if job == nil || job.Name != "test-job" {
		t.Fatalf("unexpected job: %#v", job)
	}
	expectedPath := "/job/test_webhook/job/test_webhooks/api/json"
	if requestedPath != expectedPath {
		t.Fatalf("expected path %s, got %s", expectedPath, requestedPath)
	}
}

package jenkins

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_FindJobByPattern_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Contains(t, r.URL.Path, "/api/json")

		jobList := JobList{
			Jobs: []Job{
				{Name: "test-repo-pr-123", URL: "http://jenkins/job/test-repo-pr-123", Color: "blue"},
				{Name: "other-job", URL: "http://jenkins/job/other-job", Color: "blue"},
				{Name: "test-repo-pr-456", URL: "http://jenkins/job/test-repo-pr-456", Color: "blue"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jobList)
	}))
	defer server.Close()

	client := NewClient(server.URL, "admin", "token")
	job, err := client.FindJobByPattern("^test-repo-pr-123$")

	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, "test-repo-pr-123", job.Name)
}

func TestClient_FindJobByPattern_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jobList := JobList{
			Jobs: []Job{
				{Name: "other-job", URL: "http://jenkins/job/other-job", Color: "blue"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jobList)
	}))
	defer server.Close()

	client := NewClient(server.URL, "admin", "token")
	job, err := client.FindJobByPattern("^test-repo-pr-123$")

	require.NoError(t, err)
	assert.Nil(t, job)
}

func TestClient_FindJobByPattern_InvalidRegex(t *testing.T) {
	client := NewClient("http://jenkins:8080", "admin", "token")
	job, err := client.FindJobByPattern("[invalid regex")

	assert.Error(t, err)
	assert.Nil(t, job)
	assert.Contains(t, err.Error(), "invalid regex pattern")
}

func TestClient_WaitForJob_Found(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		jobList := JobList{
			Jobs: []Job{},
		}
		if callCount >= 2 {
			jobList.Jobs = []Job{
				{Name: "test-repo-pr-123", URL: "http://jenkins/job/test-repo-pr-123", Color: "blue"},
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jobList)
	}))
	defer server.Close()

	client := NewClient(server.URL, "admin", "token")
	job, err := client.WaitForJob("^test-repo-pr-123$", 10*time.Second)

	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, "test-repo-pr-123", job.Name)
}

func TestClient_WaitForJob_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jobList := JobList{
			Jobs: []Job{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jobList)
	}))
	defer server.Close()

	client := NewClient(server.URL, "admin", "token")
	job, err := client.WaitForJob("^test-repo-pr-123$", 1*time.Second)

	require.NoError(t, err)
	assert.Nil(t, job) // Таймаут, джоба не найдена
}

func TestClient_ListJobs_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "admin", "token")
	jobs, err := client.ListJobs()

	assert.Error(t, err)
	assert.Nil(t, jobs)
	assert.Contains(t, err.Error(), "jenkins API error")
}

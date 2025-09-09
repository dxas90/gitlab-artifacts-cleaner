package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// getEnv returns the value of an environment variable or a default if it's not set
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// repoExists checks if a GitLab project exists
func projectExists(server, token string, projectID int) (bool, error) {
	url := fmt.Sprintf("https://%s/api/v4/projects/%d", server, projectID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("PRIVATE-TOKEN", token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, nil
	} else if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return false, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
}

// deleteArtifact deletes a job artifact with retries
func deleteArtifact(server, token string, projectID, jobID int, wg *sync.WaitGroup, sem chan struct{}, successCounter, failureCounter *int, mu *sync.Mutex) {
	defer wg.Done()

	sem <- struct{}{}        // acquire semaphore
	defer func() { <-sem }() // release semaphore

	url := fmt.Sprintf("https://%s/api/v4/projects/%d/jobs/%d/artifacts", server, projectID, jobID)
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Set("PRIVATE-TOKEN", token)

	client := &http.Client{Timeout: 10 * time.Second}
	var resp *http.Response
	var err error

	for i := 0; i < 3; i++ { // retry 3 times
		resp, err = client.Do(req)
		if err == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		fmt.Printf("Job %d: request failed after retries: %v\n", jobID, err)
		mu.Lock()
		*failureCounter++
		mu.Unlock()
		return
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent:
		fmt.Printf("Job %d: artifact deleted successfully\n", jobID)
		mu.Lock()
		*successCounter++
		mu.Unlock()
	case http.StatusNotFound:
		fmt.Printf("Job %d: no artifacts found\n", jobID)
	default:
		fmt.Printf("Job %d: failed to delete artifact (status: %s)\n", jobID, resp.Status)
		mu.Lock()
		*failureCounter++
		mu.Unlock()
	}
}

func main() {
	// Read config from env vars
	server := getEnv("GITLAB_SERVER", "gitlab.example.com")
	token := getEnv("GITLAB_TOKEN", "your_token_here")

	projectIDStr := getEnv("GITLAB_PROJECT_ID", "24")
	startJobStr := getEnv("GITLAB_START_JOB", "100")
	endJobStr := getEnv("GITLAB_END_JOB", "120")
	concurrencyStr := getEnv("GITLAB_CONCURRENCY", "10")

	projectID, err := strconv.Atoi(projectIDStr)
	if err != nil {
		fmt.Printf("Invalid GITLAB_PROJECT_ID: %s\n", projectIDStr)
		return
	}
	startJob, err := strconv.Atoi(startJobStr)
	if err != nil {
		fmt.Printf("Invalid GITLAB_START_JOB: %s\n", startJobStr)
		return
	}
	endJob, err := strconv.Atoi(endJobStr)
	if err != nil {
		fmt.Printf("Invalid GITLAB_END_JOB: %s\n", endJobStr)
		return
	}
	concurrency, err := strconv.Atoi(concurrencyStr)
	if err != nil || concurrency <= 0 {
		fmt.Printf("Invalid GITLAB_CONCURRENCY: %s\n", concurrencyStr)
		return
	}

	// Validate GitLab project exists
	exists, err := projectExists(server, token, projectID)
	if err != nil {
		fmt.Printf("Error checking project: %v\n", err)
		return
	}
	if !exists {
		fmt.Printf("Project %d does not exist on %s\n", projectID, server)
		return
	}

	// Delete artifacts concurrently
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)
	var successCounter, failureCounter int
	var mu sync.Mutex

	for jobID := startJob; jobID <= endJob; jobID++ {
		wg.Add(1)
		go deleteArtifact(server, token, projectID, jobID, &wg, sem, &successCounter, &failureCounter, &mu)
	}

	wg.Wait()
	fmt.Printf("All artifact deletions attempted. Successes: %d, Failures: %d\n", successCounter, failureCounter)
}


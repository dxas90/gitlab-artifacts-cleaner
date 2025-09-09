package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
)

// getEnv returns the value of an environment variable or a default if it's not set
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func deleteArtifact(server, token string, projectID, jobID int, wg *sync.WaitGroup, sem chan struct{}) {
	defer wg.Done()

	// acquire semaphore
	sem <- struct{}{}
	defer func() { <-sem }() // release semaphore

	url := fmt.Sprintf("https://%s/api/v4/projects/%d/jobs/%d/artifacts", server, projectID, jobID)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		fmt.Printf("Job %d: failed to create request: %v\n", jobID, err)
		return
	}
	req.Header.Set("PRIVATE-TOKEN", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("Job %d: request failed: %v\n", jobID, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		fmt.Printf("Job %d: artifact deleted successfully\n", jobID)
	} else {
		fmt.Printf("Job %d: failed to delete artifact (status: %s)\n", jobID, resp.Status)
	}
}

func main() {
	// Read config from env vars (with defaults)
	server := getEnv("GITLAB_SERVER", "gitlab.example.com")
	token := getEnv("GITLAB_TOKEN", "your_token_here")

	projectIDStr := getEnv("GITLAB_PROJECT_ID", "24")
	startJobStr := getEnv("GITLAB_START_JOB", "100")
	endJobStr := getEnv("GITLAB_END_JOB", "120")
	concurrencyStr := getEnv("GITLAB_CONCURRENCY", "10") // default: 5 workers

	projectID, _ := strconv.Atoi(projectIDStr)
	startJob, _ := strconv.Atoi(startJobStr)
	endJob, _ := strconv.Atoi(endJobStr)
	concurrency, _ := strconv.Atoi(concurrencyStr)

	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency) // concurrency limit

	for jobID := startJob; jobID <= endJob; jobID++ {
		wg.Add(1)
		go deleteArtifact(server, token, projectID, jobID, &wg, sem)
	}

	wg.Wait()
	fmt.Println("All artifact deletions attempted.")
}


package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

// getEnv returns the value of an environment variable or a default if it's not set
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// projectExists checks if a GitLab project exists
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

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
}

// deleteArtifact deletes a job artifact with retries and logs
func deleteArtifact(server, token string, projectID, jobID int, wg *sync.WaitGroup, sem chan struct{}, successCounter, failureCounter *int, mu *sync.Mutex, logger *log.Logger) {
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
		msg := fmt.Sprintf("Job %d: request failed after retries: %v", jobID, err)
		fmt.Println(msg)
		logger.Println(msg)
		mu.Lock()
		*failureCounter++
		mu.Unlock()
		return
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent:
		msg := fmt.Sprintf("Job %d: artifact deleted successfully", jobID)
		fmt.Println(msg)
		logger.Println(msg)
		mu.Lock()
		*successCounter++
		mu.Unlock()
	case http.StatusNotFound:
		msg := fmt.Sprintf("Job %d: no artifacts found", jobID)
		fmt.Println(msg)
		logger.Println(msg)
	default:
		msg := fmt.Sprintf("Job %d: failed to delete artifact (status: %s)", jobID, resp.Status)
		fmt.Println(msg)
		logger.Println(msg)
		mu.Lock()
		*failureCounter++
		mu.Unlock()
	}
}

func main() {
	var server, token string
	var projectID, startJob, endJob, concurrency int
	var logFile string

	rootCmd := &cobra.Command{
		Use:   "artifact-cleaner",
		Short: "Delete GitLab job artifacts concurrently",
		Run: func(cmd *cobra.Command, args []string) {
			// Open log file
			f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				fmt.Printf("Failed to open log file %s: %v\n", logFile, err)
				return
			}
			defer f.Close()
			logger := log.New(f, "", log.LstdFlags)

			// Validate project exists
			exists, err := projectExists(server, token, projectID)
			if err != nil {
				logger.Printf("Error checking project: %v\n", err)
				fmt.Printf("Error checking project: %v\n", err)
				return
			}
			if !exists {
				msg := fmt.Sprintf("Project %d does not exist on %s", projectID, server)
				logger.Println(msg)
				fmt.Println(msg)
				return
			}

			// Delete artifacts concurrently
			var wg sync.WaitGroup
			sem := make(chan struct{}, concurrency)
			var successCounter, failureCounter int
			var mu sync.Mutex

			for jobID := startJob; jobID <= endJob; jobID++ {
				wg.Add(1)
				go deleteArtifact(server, token, projectID, jobID, &wg, sem, &successCounter, &failureCounter, &mu, logger)
			}

			wg.Wait()
			summary := fmt.Sprintf("All artifact deletions attempted. Successes: %d, Failures: %d", successCounter, failureCounter)
			fmt.Println(summary)
			logger.Println(summary)
		},
	}

	// Convert environment variables to int, with defaults
	projectID, _ = strconv.Atoi(getEnv("GITLAB_PROJECT_ID", "1"))
	startJobDefault, _ := strconv.Atoi(getEnv("GITLAB_START_JOB", "1"))
	endJobDefault, _ := strconv.Atoi(getEnv("GITLAB_END_JOB", "120"))
	concurrencyDefault, _ := strconv.Atoi(getEnv("GITLAB_CONCURRENCY", "100"))

	// Flags with environment variable defaults
	rootCmd.Flags().StringVar(&server, "gitlab-server", getEnv("GITLAB_SERVER", "gitlab.example.com"), "GitLab server URL")
	rootCmd.Flags().StringVar(&token, "gitlab-token", getEnv("GITLAB_TOKEN", ""), "GitLab private token")
	rootCmd.Flags().IntVar(&projectID, "project", projectID, "GitLab project ID")
	rootCmd.Flags().IntVar(&startJob, "gitlab-start-job", startJobDefault, "Starting job ID")
	rootCmd.Flags().IntVar(&endJob, "gitlab-end-job", endJobDefault, "Ending job ID")
	rootCmd.Flags().IntVar(&concurrency, "gitlab-concurrency", concurrencyDefault, "Maximum concurrent deletions")
	rootCmd.Flags().StringVar(&logFile, "log-file", "artifact-cleaner.log", "Path to log file")

	// Only mark required if no env var is set
	if getEnv("GITLAB_TOKEN", "") == "" {
		rootCmd.MarkFlagRequired("gitlab-token")
	}
	if getEnv("GITLAB_PROJECT_ID", "") == "" {
		rootCmd.MarkFlagRequired("project")
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

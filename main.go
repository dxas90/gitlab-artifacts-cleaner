package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/k0kubun/go-ansi"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

// Shared HTTP client with connection pooling for reuse across all requests.
// Creating a new client for each request is inefficient and can lead to port exhaustion.
var httpClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	},
}

// getEnv returns the value of an environment variable or a default value if not set.
// This is useful for providing fallback values when environment variables are not configured.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// projectExists checks if a GitLab project exists using the GitLab API.
// It returns true if the project exists, false if not found, and an error if the request fails.
func projectExists(ctx context.Context, server, token string, projectID int) (bool, error) {
	url := fmt.Sprintf("https://%s/api/v4/projects/%d", server, projectID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}
	req = req.WithContext(ctx)
	req.Header.Set("PRIVATE-TOKEN", token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("failed to close response body: %v", err)
		}
	}()

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
}

// deleteArtifact deletes a job artifact from GitLab with automatic retries.
// It supports dry-run mode where no actual deletion occurs, and uses context for cancellation.
// Results are logged both to console and a log file, with separate counters for tracking.
func deleteArtifact(ctx context.Context, server, token string, projectID, jobID int, dryRun, verbose bool, wg *sync.WaitGroup, sem chan struct{}, successCounter, failureCounter, skippedCounter, processedCounter *int64, bar *progressbar.ProgressBar, logger *log.Logger) {
	defer wg.Done()

	select {
	case sem <- struct{}{}: // acquire semaphore
	case <-ctx.Done():
		return
	}
	defer func() { <-sem }() // release semaphore

	// Check if context is cancelled before starting
	if ctx.Err() != nil {
		return
	}

	url := fmt.Sprintf("https://%s/api/v4/projects/%d/jobs/%d/artifacts", server, projectID, jobID)

	if dryRun {
		msg := fmt.Sprintf("Job %d: [DRY-RUN] Would delete artifact", jobID)
		if verbose {
			fmt.Println(msg)
		} else if bar != nil {
			bar.Add(1)
		}
		logger.Println(msg)
		atomic.AddInt64(skippedCounter, 1)
		atomic.AddInt64(processedCounter, 1)
		return
	}

	var resp *http.Response
	var err error

	// Retry logic with exponential backoff
	// Recreate request on each attempt to ensure fresh context
	for i := 0; i < 3; i++ {
		if ctx.Err() != nil {
			return
		}

		// Create new request for each retry attempt
		req, reqErr := http.NewRequest("DELETE", url, nil)
		if reqErr != nil {
			msg := fmt.Sprintf("Job %d: failed to create request: %v", jobID, reqErr)
			if verbose {
				fmt.Println(msg)
			} else if bar != nil {
				_ = bar.Add(1)
			}
			logger.Println(msg)
			atomic.AddInt64(failureCounter, 1)
			atomic.AddInt64(processedCounter, 1)
			return
		}
		req = req.WithContext(ctx)
		req.Header.Set("PRIVATE-TOKEN", token)

		resp, err = httpClient.Do(req)
		if err == nil {
			break
		}

		if i < 2 { // Don't sleep on last iteration
			time.Sleep(time.Duration(i+1) * 2 * time.Second)
		}
	}

	if err != nil {
		msg := fmt.Sprintf("Job %d: request failed after retries: %v", jobID, err)
		if verbose {
			fmt.Println(msg)
		} else if bar != nil {
			_ = bar.Add(1)
		}
		logger.Println(msg)
		atomic.AddInt64(failureCounter, 1)
		atomic.AddInt64(processedCounter, 1)
		return
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Printf("Job %d: failed to close response body: %v", jobID, err)
		}
	}()

	switch resp.StatusCode {
	case http.StatusNoContent:
		msg := fmt.Sprintf("Job %d: artifact deleted successfully", jobID)
		if verbose {
			fmt.Println(msg)
		} else if bar != nil {
			_ = bar.Add(1)
		}
		logger.Println(msg)
		atomic.AddInt64(successCounter, 1)
		atomic.AddInt64(processedCounter, 1)
	case http.StatusNotFound:
		msg := fmt.Sprintf("Job %d: no artifacts found", jobID)
		if verbose {
			fmt.Println(msg)
		} else if bar != nil {
			_ = bar.Add(1)
		}
		logger.Println(msg)
		atomic.AddInt64(skippedCounter, 1)
		atomic.AddInt64(processedCounter, 1)
	default:
		msg := fmt.Sprintf("Job %d: failed to delete artifact (status: %s)", jobID, resp.Status)
		if verbose {
			fmt.Println(msg)
		} else if bar != nil {
			_ = bar.Add(1)
		}
		logger.Println(msg)
		atomic.AddInt64(failureCounter, 1)
		atomic.AddInt64(processedCounter, 1)
	}
}

// validateInputs performs validation on all input parameters.
// Returns an error if any validation fails.
func validateInputs(server, token string, projectID, startJob, endJob, concurrency int) error {
	if server == "" {
		return fmt.Errorf("gitlab-server cannot be empty")
	}
	if token == "" {
		return fmt.Errorf("gitlab-token is required")
	}
	if projectID <= 0 {
		return fmt.Errorf("project ID must be positive, got %d", projectID)
	}
	if startJob <= 0 {
		return fmt.Errorf("start job ID must be positive, got %d", startJob)
	}
	if endJob < startJob {
		return fmt.Errorf("end job ID (%d) cannot be less than start job ID (%d)", endJob, startJob)
	}
	if concurrency < 1 || concurrency > 1000 {
		return fmt.Errorf("concurrency must be between 1 and 1000, got %d", concurrency)
	}
	totalJobs := endJob - startJob + 1
	if totalJobs > 1000000 {
		return fmt.Errorf("job range too large (%d jobs), maximum is 1,000,000", totalJobs)
	}
	return nil
}

func main() {
	var server, token string
	var projectID, startJob, endJob, concurrency int
	var logFile string
	var dryRun, verbose bool

	rootCmd := &cobra.Command{
		Use:   "gitlab-artifacts-cleaner",
		Short: "Delete GitLab job artifacts concurrently",
		Long: `A tool to delete GitLab job artifacts for a range of job IDs.

Supports concurrent deletions with configurable rate limiting, dry-run mode,
and graceful shutdown on interrupt signals.`,
		Run: func(cmd *cobra.Command, args []string) {
			// Input validation
			if err := validateInputs(server, token, projectID, startJob, endJob, concurrency); err != nil {
				fmt.Printf("Validation error: %v\n", err)
				os.Exit(1)
			}

			// Open log file
			f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				fmt.Printf("Failed to open log file %s: %v\n", logFile, err)
				os.Exit(1)
			}
			defer func() {
				if err := f.Close(); err != nil {
					fmt.Printf("Failed to close log file: %v\n", err)
				}
			}()
			logger := log.New(f, "", log.LstdFlags)

			// Setup context with cancellation
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Handle interrupt signals for graceful shutdown
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
			go func() {
				<-sigChan
				fmt.Println("\n\nReceived interrupt signal. Shutting down gracefully...")
				logger.Println("Received interrupt signal. Shutting down gracefully...")
				cancel()
			}()

			// Display configuration
			fmt.Printf("GitLab Artifacts Cleaner\n")
			fmt.Printf("========================\n")
			fmt.Printf("Server:      %s\n", server)
			fmt.Printf("Project ID:  %d\n", projectID)
			fmt.Printf("Job Range:   %d - %d\n", startJob, endJob)
			fmt.Printf("Concurrency: %d\n", concurrency)
			fmt.Printf("Dry Run:     %v\n", dryRun)
			fmt.Printf("Verbose:     %v\n", verbose)
			fmt.Printf("Log File:    %s\n", logFile)
			fmt.Printf("========================\n\n")

			logger.Printf("Starting artifact cleanup: server=%s, project=%d, jobs=%d-%d, concurrency=%d, dryRun=%v",
				server, projectID, startJob, endJob, concurrency, dryRun)

			// Validate project exists
			fmt.Println("Validating project...")
			exists, err := projectExists(ctx, server, token, projectID)
			if err != nil {
				logger.Printf("Error checking project: %v\n", err)
				fmt.Printf("Error checking project: %v\n", err)
				os.Exit(1)
			}
			if !exists {
				msg := fmt.Sprintf("Project %d does not exist on %s", projectID, server)
				logger.Println(msg)
				fmt.Println(msg)
				os.Exit(1)
			}
			fmt.Printf("✓ Project validated\n\n")

			// Delete artifacts concurrently
			var wg sync.WaitGroup
			sem := make(chan struct{}, concurrency)
			var successCounter, failureCounter, skippedCounter, processedCounter int64

			totalJobs := endJob - startJob + 1
			if dryRun {
				fmt.Printf("[DRY-RUN MODE] Would process %d jobs\n\n", totalJobs)
			} else {
				fmt.Printf("Processing %d jobs...\n\n", totalJobs)
			}

			startTime := time.Now()

			// Create progress bar for non-verbose mode
			var bar *progressbar.ProgressBar
			if !verbose {
				bar = progressbar.NewOptions(totalJobs,
					progressbar.OptionSetWriter(ansi.NewAnsiStdout()),
					progressbar.OptionEnableColorCodes(true),
					progressbar.OptionSetWidth(40),
					progressbar.OptionSetDescription("[cyan]Deleting artifacts[reset]"),
					progressbar.OptionSetTheme(progressbar.Theme{
						Saucer:        "[green]=[reset]",
						SaucerHead:    "[green]>[reset]",
						SaucerPadding: " ",
						BarStart:      "[",
						BarEnd:        "]",
					}),
					progressbar.OptionShowCount(),
					progressbar.OptionShowIts(),
					progressbar.OptionSetItsString("jobs"),
					progressbar.OptionThrottle(100*time.Millisecond),
				)
			}

			for jobID := startJob; jobID <= endJob; jobID++ {
				if ctx.Err() != nil {
					fmt.Println("\nCancellation requested, waiting for ongoing operations...")
					break
				}
				wg.Add(1)
				go deleteArtifact(ctx, server, token, projectID, jobID, dryRun, verbose, &wg, sem, &successCounter, &failureCounter, &skippedCounter, &processedCounter, bar, logger)
			}

			wg.Wait()
			if bar != nil {
				if err := bar.Finish(); err != nil {
					fmt.Printf("Failed to finish progress bar: %v\n", err)
				}
				fmt.Println() // Add spacing after progress bar
			}
			duration := time.Since(startTime)

			// Print summary
			summary := fmt.Sprintf("Completed in %v. Successes: %d, Failures: %d, Skipped/NotFound: %d, Total: %d",
				duration.Round(time.Millisecond), successCounter, failureCounter, skippedCounter, totalJobs)
			fmt.Println(summary)
			logger.Println(summary)

			if failureCounter > 0 {
				os.Exit(1)
			}
		},
	}

	// Convert environment variables to int, with defaults
	projectID, _ = strconv.Atoi(getEnv("GITLAB_PROJECT_ID", "1"))
	startJobDefault, _ := strconv.Atoi(getEnv("GITLAB_START_JOB", "1"))
	endJobDefault, _ := strconv.Atoi(getEnv("GITLAB_END_JOB", "120"))
	concurrencyDefault, _ := strconv.Atoi(getEnv("GITLAB_CONCURRENCY", "100"))

	// Flags with environment variable defaults
	rootCmd.Flags().StringVar(&server, "gitlab-server", getEnv("GITLAB_SERVER", "gitlab.example.com"), "GitLab server hostname (without https://)")
	rootCmd.Flags().StringVar(&token, "gitlab-token", getEnv("GITLAB_TOKEN", ""), "GitLab private access token (required)")
	rootCmd.Flags().IntVar(&projectID, "project", projectID, "GitLab project ID (required)")
	rootCmd.Flags().IntVar(&startJob, "gitlab-start-job", startJobDefault, "Starting job ID (inclusive)")
	rootCmd.Flags().IntVar(&endJob, "gitlab-end-job", endJobDefault, "Ending job ID (inclusive)")
	rootCmd.Flags().IntVar(&concurrency, "gitlab-concurrency", concurrencyDefault, "Maximum concurrent deletions (1-1000)")
	rootCmd.Flags().StringVar(&logFile, "log-file", "artifact-cleaner.log", "Path to log file")
	rootCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview what would be deleted without actually deleting")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	// Only mark required if no env var is set
	if getEnv("GITLAB_TOKEN", "") == "" {
		if err := rootCmd.MarkFlagRequired("gitlab-token"); err != nil {
			fmt.Printf("Failed to mark gitlab-token as required: %v\n", err)
		}
	}
	if getEnv("GITLAB_PROJECT_ID", "") == "" {
		if err := rootCmd.MarkFlagRequired("project"); err != nil {
			fmt.Printf("Failed to mark project as required: %v\n", err)
		}
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

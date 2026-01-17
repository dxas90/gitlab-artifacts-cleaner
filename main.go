package main

import (
	"context"
	"encoding/json"
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
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	},
}

// Job represents a GitLab CI/CD job from the API
type Job struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Artifacts []struct {
		FileType string `json:"file_type"`
		Size     int    `json:"size"`
	} `json:"artifacts"`
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// projectExists checks if a GitLab project exists using the GitLab API.
func projectExists(ctx context.Context, server, token string, projectID int) (bool, error) {
	apiURL := fmt.Sprintf("https://%s/api/v4/projects/%d", server, projectID)
	req, err := http.NewRequest("GET", apiURL, nil)
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

// listJobs fetches all jobs from a GitLab project with pagination
func listJobs(ctx context.Context, server, token string, projectID, pageLimit int, logger *log.Logger) ([]Job, error) {
	var allJobs []Job
	page := 1
	perPage := 100

	logger.Printf("Fetching jobs from project %d (page limit: %d)...", projectID, pageLimit)

	for {
		if ctx.Err() != nil {
			return allJobs, ctx.Err()
		}

		apiURL := fmt.Sprintf("https://%s/api/v4/projects/%d/jobs?per_page=%d&page=%d",
			server, projectID, perPage, page)

		req, err := http.NewRequest("GET", apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		req = req.WithContext(ctx)
		req.Header.Set("PRIVATE-TOKEN", token)

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to execute request: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		var jobs []Job
		if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		_ = resp.Body.Close()

		if len(jobs) == 0 {
			break
		}

		allJobs = append(allJobs, jobs...)
		logger.Printf("Fetched page %d: %d jobs (total so far: %d)", page, len(jobs), len(allJobs))
		fmt.Printf("\rFetching jobs... %d found so far", len(allJobs))

		// Check for next page
		if len(jobs) < perPage {
			break
		}
		page++
		if pageLimit > 0 && page > pageLimit {
			logger.Printf("Reached page limit (%d), stopping job discovery", pageLimit)
			break
		}
	}

	fmt.Println() // newline after progress
	logger.Printf("Total jobs fetched: %d", len(allJobs))
	return allJobs, nil
}

// deleteArtifact deletes a job artifact from GitLab with automatic retries.
func deleteArtifact(ctx context.Context, server, token string, projectID, jobID int, dryRun, verbose bool, wg *sync.WaitGroup, sem chan struct{}, successCounter, failureCounter, skippedCounter, processedCounter *int64, bar *progressbar.ProgressBar, logger *log.Logger) {
	defer wg.Done()

	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		return
	}
	defer func() { <-sem }()

	if ctx.Err() != nil {
		return
	}

	apiURL := fmt.Sprintf("https://%s/api/v4/projects/%d/jobs/%d/artifacts", server, projectID, jobID)

	if dryRun {
		msg := fmt.Sprintf("Job %d: [DRY-RUN] Would delete artifact", jobID)
		if verbose {
			fmt.Println(msg)
		} else if bar != nil {
			_ = bar.Add(1)
		}
		logger.Println(msg)
		atomic.AddInt64(skippedCounter, 1)
		atomic.AddInt64(processedCounter, 1)
		return
	}

	var resp *http.Response
	var err error

	// Retry logic with exponential backoff
	for i := 0; i < 3; i++ {
		if ctx.Err() != nil {
			return
		}

		req, reqErr := http.NewRequest("DELETE", apiURL, nil)
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

		if i < 2 {
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

func validateInputs(server, token string, projectID, concurrency int) error {
	if server == "" {
		return fmt.Errorf("gitlab-server cannot be empty")
	}
	if token == "" {
		return fmt.Errorf("gitlab-token is required")
	}
	if projectID <= 0 {
		return fmt.Errorf("project ID must be positive, got %d", projectID)
	}
	if concurrency < 1 || concurrency > 1000 {
		return fmt.Errorf("concurrency must be between 1 and 1000, got %d", concurrency)
	}
	return nil
}

func main() {
	var server, token string
	var projectID, concurrency, pageLimit int
	var logFile string
	var dryRun, verbose bool

	rootCmd := &cobra.Command{
		Use:   "gitlab-artifacts-cleaner",
		Short: "Delete all GitLab job artifacts from a project",
		Long: `A tool to automatically discover and delete all job artifacts from a GitLab project.

The tool uses the GitLab Jobs API to discover all jobs in the project, then
deletes their artifacts concurrently with configurable rate limiting.`,
		Run: func(cmd *cobra.Command, args []string) {
			// Input validation
			if err := validateInputs(server, token, projectID, concurrency); err != nil {
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

			// Handle interrupt signals
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
			fmt.Printf("Concurrency: %d\n", concurrency)
			fmt.Printf("Dry Run:     %v\n", dryRun)
			fmt.Printf("Verbose:     %v\n", verbose)
			fmt.Printf("Log File:    %s\n", logFile)
			fmt.Printf("========================\n\n")

			logger.Printf("Starting artifact cleanup: server=%s, project=%d, concurrency=%d, dryRun=%v",
				server, projectID, concurrency, dryRun)

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

			// Fetch all jobs
			fmt.Println("Discovering jobs from GitLab API...")
			jobs, err := listJobs(ctx, server, token, projectID, pageLimit, logger)
			if err != nil {
				logger.Printf("Error fetching jobs: %v\n", err)
				fmt.Printf("Error fetching jobs: %v\n", err)
				os.Exit(1)
			}

			if len(jobs) == 0 {
				fmt.Println("No jobs found in project")
				logger.Println("No jobs found in project")
				return
			}

			fmt.Printf("✓ Discovered %d jobs\n\n", len(jobs))
			logger.Printf("Discovered %d jobs", len(jobs))

			// Delete artifacts concurrently
			var wg sync.WaitGroup
			sem := make(chan struct{}, concurrency)
			var successCounter, failureCounter, skippedCounter, processedCounter int64

			totalJobs := len(jobs)
			if dryRun {
				fmt.Printf("[DRY-RUN MODE] Would process %d jobs\n\n", totalJobs)
			} else {
				fmt.Printf("Processing %d jobs...\n\n", totalJobs)
			}

			startTime := time.Now()

			// Create progress bar
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

			for _, job := range jobs {
				if ctx.Err() != nil {
					fmt.Println("\nCancellation requested, waiting for ongoing operations...")
					break
				}
				wg.Add(1)
				go deleteArtifact(ctx, server, token, projectID, job.ID, dryRun, verbose, &wg, sem, &successCounter, &failureCounter, &skippedCounter, &processedCounter, bar, logger)
			}

			wg.Wait()
			if bar != nil {
				if err := bar.Finish(); err != nil {
					fmt.Printf("Failed to finish progress bar: %v\n", err)
				}
				fmt.Println()
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

	// Convert environment variables to int
	projectID, _ = strconv.Atoi(getEnv("GITLAB_PROJECT_ID", "1"))
	concurrencyDefault, _ := strconv.Atoi(getEnv("GITLAB_CONCURRENCY", "70"))
	pageLimitDefault, _ := strconv.Atoi(getEnv("GITLAB_JOB_PAGE_LIMIT", "0"))

	// Flags
	rootCmd.Flags().StringVar(&server, "gitlab-server", getEnv("GITLAB_SERVER", "gitlab.example.com"), "GitLab server hostname (without https://)")
	rootCmd.Flags().StringVar(&token, "gitlab-token", getEnv("GITLAB_TOKEN", ""), "GitLab private access token (required)")
	rootCmd.Flags().IntVar(&projectID, "project", projectID, "GitLab project ID (required)")
	rootCmd.Flags().IntVar(&concurrency, "concurrency", concurrencyDefault, "Maximum concurrent deletions (1-1000)")
	rootCmd.Flags().IntVar(&pageLimit, "page-limit", pageLimitDefault, "Maximum pages to fetch from Jobs API (0 = unlimited)")
	rootCmd.Flags().StringVar(&logFile, "log-file", "artifact-cleaner.log", "Path to log file")
	rootCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview what would be deleted without actually deleting")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	// Mark required flags
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

# GitLab Artifact Cleaner

A robust Go utility to delete artifacts from a GitLab project with concurrent processing,
graceful shutdown, and comprehensive error handling.

## Features

- **Concurrent Processing**: Delete artifacts for a range of job IDs with configurable parallelism
- **Dry-Run Mode**: Preview what would be deleted without making actual changes
- **Verbose Output**: Optional detailed logging for each operation
- **Graceful Shutdown**: Handles interrupt signals (Ctrl+C) gracefully, waiting for ongoing operations
- **Retry Logic**: Automatic retries with exponential backoff for failed requests
- **Input Validation**: Comprehensive validation of all parameters before execution
- **Progress Reporting**: Clear summary with success/failure counts and execution time
- **Flexible Configuration**: Environment variables or command-line flags
- **Detailed Logging**: All operations logged to a file for audit trail

## Installation

1. Make sure you have [Go](https://go.dev/dl/) installed (1.20+ recommended).
2. Clone this repository:

    git clone <https://github.com/your-username/gitlab-artifact-cleaner.git>
    cd gitlab-artifact-cleaner

3. Build the binary:

```bash
go build -o gitlab-artifacts-cleaner
```

## Usage

You can run the tool directly or configure it with environment variables.

### Environment Variables

| Variable              | Default               | Description                                   |
|-----------------------|-----------------------|-----------------------------------------------|
| `GITLAB_SERVER`       | `gitlab.example.com`  | GitLab instance hostname (without `https://`) |
| `GITLAB_TOKEN`        | (required)            | GitLab personal access token (PAT)            |
| `GITLAB_PROJECT_ID`   | `1`                   | GitLab project ID                             |
| `GITLAB_START_JOB`    | `1`                   | Starting job ID (inclusive)                   |
| `GITLAB_END_JOB`      | `120`                 | Ending job ID (inclusive)                     |
| `GITLAB_CONCURRENCY`  | `100`                 | Maximum concurrent deletions (1-1000)         |

### Command-Line Flags

All settings can be configured via command-line flags, which override environment variables:

| Flag                   | Type    | Description                                        |
|------------------------|---------|----------------------------------------------------|
| `--gitlab-server`      | string  | GitLab server hostname (without https://)          |
| `--gitlab-token`       | string  | GitLab private access token (required)             |
| `--project`            | int     | GitLab project ID (required)                       |
| `--gitlab-start-job`   | int     | Starting job ID (inclusive)                        |
| `--gitlab-end-job`     | int     | Ending job ID (inclusive)                          |
| `--gitlab-concurrency` | int     | Maximum concurrent deletions (1-1000)              |
| `--log-file`           | string  | Path to log file (default: "artifact-cleaner.log") |
| `--dry-run`            | boolean | Preview mode - don't actually delete anything      |
| `--verbose`, `-v`      | boolean | Enable verbose output                              |

### Examples

#### Basic Usage with Environment Variables

```bash
export GITLAB_SERVER=gitlab.mycompany.com
export GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxx
export GITLAB_PROJECT_ID=42
export GITLAB_START_JOB=200
export GITLAB_END_JOB=210
export GITLAB_CONCURRENCY=10

./gitlab-artifacts-cleaner
```

#### Using Command-Line Flags

```bash
./gitlab-artifacts-cleaner \
  --gitlab-server=gitlab.mycompany.com \
  --gitlab-token=glpat-xxxxxxxxxxxxxxxx \
  --project=42 \
  --gitlab-start-job=200 \
  --gitlab-end-job=210 \
  --gitlab-concurrency=10
```

#### Dry-Run Mode (Preview Only)

Test what would be deleted without actually deleting:

```bash
./gitlab-artifacts-cleaner \
  --gitlab-server=gitlab.mycompany.com \
  --gitlab-token=glpat-xxxxxxxxxxxxxxxx \
  --project=42 \
  --gitlab-start-job=200 \
  --gitlab-end-job=210 \
  --dry-run
```

#### Verbose Mode

See detailed output for every job processed:

```bash
./gitlab-artifacts-cleaner \
  --gitlab-server=gitlab.mycompany.com \
  --gitlab-token=glpat-xxxxxxxxxxxxxxxx \
  --project=42 \
  --gitlab-start-job=200 \
  --gitlab-end-job=210 \
  --verbose
```

### Sample Output

```text
GitLab Artifacts Cleaner
========================
Server:      gitlab.mycompany.com
Project ID:  42
Job Range:   200 - 210
Concurrency: 10
Dry Run:     false
Verbose:     false
Log File:    artifact-cleaner.log
========================

Validating project...
✓ Project validated

Processing 11 jobs...

Job 201: artifact deleted successfully
Job 203: artifact deleted successfully
Job 205: failed to delete artifact (status: 500 Internal Server Error)
...

========================
Summary
========================
Completed in 2.345s. Successes: 9, Failures: 1, Skipped/NotFound: 1, Total: 11
```

## Graceful Shutdown

The tool handles interrupt signals (Ctrl+C, SIGTERM) gracefully:

- Stops launching new deletion requests
- Waits for ongoing operations to complete
- Logs the shutdown event
- Provides a summary of operations completed before shutdown

## Configuration Priority

Settings are resolved in the following order (highest priority first):

1. Command-line flags
2. Environment variables
3. Default values

## Input Validation

The tool validates all inputs before execution:

- Server hostname must not be empty
- Access token is required
- Project ID must be positive
- Start job ID must be positive
- End job ID must be >= start job ID
- Concurrency must be between 1 and 1000
- Total job range cannot exceed 1,000,000 jobs

## API Token Permissions

Your GitLab personal access token must have the `api` scope to delete artifacts.
You can create a token at: `https://your-gitlab-instance.com/-/profile/personal_access_tokens`

## Notes

- The tool uses the GitLab REST API (v4)
- Only job artifacts are deleted — jobs and their logs remain
- If a job has no artifacts, it's counted as "Skipped/NotFound"
- Failed requests are automatically retried up to 3 times with exponential backoff
- All operations are logged to the specified log file
- The tool exits with code 1 if any deletions failed, otherwise 0

## Troubleshooting

**Authentication errors**: Ensure your token has the `api` scope and hasn't expired

**Rate limiting**: GitLab may rate-limit requests. Reduce `--gitlab-concurrency` if you encounter 429 errors

**Timeouts**: Individual requests timeout after 10 seconds. The tool will retry automatically

**Permission denied**: Ensure you have at least Maintainer role on the project

## Development

### Running Tests

```bash
go test -v ./...
```

### Code Structure

- `getEnv()`: Helper for reading environment variables with defaults
- `validateInputs()`: Validates all input parameters before execution
- `projectExists()`: Verifies the GitLab project exists via API
- `deleteArtifact()`: Core function that deletes a single artifact with retries
- `main()`: Orchestrates the entire cleanup process

### Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes with clear commit messages
4. Add tests if applicable
5. Update documentation as needed
6. Submit a pull request

## License

MIT

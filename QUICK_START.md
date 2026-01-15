# Quick Start Guide

## Prerequisites

- Go 1.23 or higher
- GitLab personal access token with `api` scope
- Maintainer or Owner role on the target GitLab project

## Installation

```bash
# Clone the repository
git clone https://github.com/dxas90/gitlab-artifacts-cleaner.git
cd gitlab-artifacts-cleaner

# Build the binary
make build

# Or just use go build
go build -o gitlab-artifacts-cleaner .
```

## Quick Examples

### Example 1: Dry Run (Safe Preview)

Preview what would be deleted before actually doing it:

```bash
./gitlab-artifacts-cleaner \
  --gitlab-server=gitlab.example.com \
  --gitlab-token=glpat-xxxxxxxxxxxxx \
  --project=123 \
  --gitlab-start-job=1000 \
  --gitlab-end-job=1050 \
  --dry-run
```

### Example 2: Delete with Verbose Output

Delete artifacts with detailed output for each job:

```bash
./gitlab-artifacts-cleaner \
  --gitlab-server=gitlab.example.com \
  --gitlab-token=glpat-xxxxxxxxxxxxx \
  --project=123 \
  --gitlab-start-job=1000 \
  --gitlab-end-job=1050 \
  --verbose
```

### Example 3: Using Environment Variables

```bash
export GITLAB_SERVER=gitlab.example.com
export GITLAB_TOKEN=glpat-xxxxxxxxxxxxx
export GITLAB_PROJECT_ID=123
export GITLAB_START_JOB=1000
export GITLAB_END_JOB=1050
export GITLAB_CONCURRENCY=50

./gitlab-artifacts-cleaner
```

### Example 4: Low Concurrency for Rate Limiting

If GitLab is rate-limiting you, reduce concurrency:

```bash
./gitlab-artifacts-cleaner \
  --gitlab-server=gitlab.example.com \
  --gitlab-token=glpat-xxxxxxxxxxxxx \
  --project=123 \
  --gitlab-start-job=1000 \
  --gitlab-end-job=2000 \
  --gitlab-concurrency=10
```

### Example 5: Large Cleanup with Custom Log File

```bash
./gitlab-artifacts-cleaner \
  --gitlab-server=gitlab.example.com \
  --gitlab-token=glpat-xxxxxxxxxxxxx \
  --project=123 \
  --gitlab-start-job=1 \
  --gitlab-end-job=10000 \
  --gitlab-concurrency=100 \
  --log-file=cleanup-$(date +%Y%m%d).log
```

## Common Workflows

### Workflow 1: Safe Cleanup Process

```bash
# Step 1: Preview what would be deleted
./gitlab-artifacts-cleaner --project=123 --gitlab-start-job=1 --gitlab-end-job=100 --dry-run

# Step 2: If happy, run for real
./gitlab-artifacts-cleaner --project=123 --gitlab-start-job=1 --gitlab-end-job=100

# Step 3: Check the log file
tail -f artifact-cleaner.log
```

### Workflow 2: Clean Old Pipeline Artifacts

```bash
# Get the job ID range from GitLab CI/CD → Jobs page
# Then clean all artifacts for that range

./gitlab-artifacts-cleaner \
  --project=123 \
  --gitlab-start-job=5000 \
  --gitlab-end-job=8000 \
  --gitlab-concurrency=50
```

### Workflow 3: Emergency Stop

If you need to stop the process:

1. Press `Ctrl+C` once
2. The tool will stop launching new deletions
3. Wait for ongoing operations to complete
4. Check the summary and log for what was processed

## Troubleshooting

### "Validation error: gitlab-token is required"

Set the token via environment variable or flag:

```bash
export GITLAB_TOKEN=glpat-xxxxxxxxxxxxx
# or
./gitlab-artifacts-cleaner --gitlab-token=glpat-xxxxxxxxxxxxx ...
```

### "Error checking project: unexpected status code: 401"

Your token is invalid or expired. Create a new one at:
`https://your-gitlab-instance.com/-/profile/personal_access_tokens`

### "Error checking project: unexpected status code: 403"

You don't have sufficient permissions. You need at least Maintainer role.

### "Error checking project: unexpected status code: 404"

The project ID is incorrect or the project doesn't exist.

### Rate Limiting (429 errors)

Reduce concurrency:

```bash
./gitlab-artifacts-cleaner ... --gitlab-concurrency=5
```

## Tips

1. **Always test with --dry-run first** on a small range
2. **Use verbose mode** (`-v`) when troubleshooting
3. **Check the log file** for a complete audit trail
4. **Start with low concurrency** and increase if performance is good
5. **Use environment variables** for frequently used settings
6. **Set a custom log file name** for different cleanup sessions

## Getting Your Job ID Range

To find the job IDs you want to clean:

1. Go to your GitLab project
2. Navigate to **CI/CD → Jobs**
3. Look at the job IDs in the list (they're sequential numbers)
4. Note the range you want to clean (e.g., 1000-5000)
5. Use those values for `--gitlab-start-job` and `--gitlab-end-job`

Alternatively, you can use the GitLab API:

```bash
curl --header "PRIVATE-TOKEN: glpat-xxxxxxxxxxxxx" \
  "https://gitlab.example.com/api/v4/projects/123/jobs?per_page=1" | jq '.[0].id'
```

This shows the most recent job ID. Older jobs have lower IDs.

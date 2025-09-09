# GitLab Artifact Cleaner

A simple Go utility to delete artifacts from a GitLab project.  
It uses the GitLab API and runs deletions concurrently with configurable limits.

## Features

- Deletes artifacts for a range of job IDs.
- Uses goroutines for concurrency.
- Configurable concurrency limit (to avoid overloading GitLab).
- Reads configuration from environment variables (with defaults).
- Clear output for each job: success or failure.

## Installation

1. Make sure you have [Go](https://go.dev/dl/) installed (1.20+ recommended).  
2. Clone this repository:

    git clone https://github.com/your-username/gitlab-artifact-cleaner.git  
    cd gitlab-artifact-cleaner  

3. Build the binary:

    go build -o artifact-cleaner  

## Usage

You can run the tool directly or configure it with environment variables.

### Environment Variables

| Variable              | Default             | Description                                   |
|-----------------------|---------------------|-----------------------------------------------|
| `GITLAB_SERVER`       | `gitlab.example.com`| GitLab instance hostname (no `https://`).     |
| `GITLAB_TOKEN`        | `your_token_here`   | GitLab personal access token.                 |
| `GITLAB_PROJECT_ID`   | `24`                | GitLab project ID.                            |
| `GITLAB_START_JOB`    | `100`               | Starting job ID (inclusive).                  |
| `GITLAB_END_JOB`      | `120`               | Ending job ID (inclusive).                    |
| `GITLAB_CONCURRENCY`  | `5`                 | Max concurrent deletions.                     |

### Example

Delete artifacts for jobs `200` through `210` in project `42`:

    export GITLAB_SERVER=gitlab.mycompany.com  
    export GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxx  
    export GITLAB_PROJECT_ID=42  
    export GITLAB_START_JOB=200  
    export GITLAB_END_JOB=210  
    export GITLAB_CONCURRENCY=10  

    ./artifact-cleaner  

Sample output:

    Job 200: artifact deleted successfully  
    Job 201: artifact deleted successfully  
    Job 202: failed to delete artifact (status: 404 Not Found)  
    ...  
    All artifact deletions attempted.  

## Notes

- The tool makes direct calls to the GitLab REST API.  
- Only job artifacts are deleted — jobs themselves remain.  
- If a job has no artifacts, GitLab returns `404 Not Found`.  

## License

MIT

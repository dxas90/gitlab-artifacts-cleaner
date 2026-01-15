# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Dry-run mode** (`--dry-run`): Preview what would be deleted without actually deleting
- **Verbose mode** (`--verbose`, `-v`): Enable detailed output for each operation
- **Graceful shutdown**: Handle SIGINT/SIGTERM signals to stop gracefully
- **Context support**: Proper cancellation propagation throughout the application
- **Input validation**: Comprehensive validation of all parameters before execution
- **Progress reporting**: Display configuration and summary with execution time
- **GoDoc comments**: Complete documentation for all public functions
- **Unit tests**: Test coverage for core functions (`getEnv`, `validateInputs`)
- **Makefile**: Convenient targets for build, test, lint, and more
- **CI workflow**: GitHub Actions workflow for automated testing and linting
- **CHANGELOG**: Track project changes following Keep a Changelog format

### Changed

- **Error handling**: Improved error messages with proper error wrapping
- **Retry logic**: Enhanced with exponential backoff (2s, 4s, 6s delays)
- **Concurrency**: Use atomic operations instead of mutex for counters
- **Command name**: Updated from `artifact-cleaner` to `gitlab-artifacts-cleaner`
- **Output format**: Cleaner, more structured console output with sections
- **Exit codes**: Return 1 if any deletions failed, 0 otherwise
- **Flag descriptions**: More detailed help text for all command-line flags

### Fixed

- **Unchecked errors**: All HTTP request creation errors are now properly handled
- **Context propagation**: Requests now properly respect cancellation signals
- **Resource leaks**: Ensure all HTTP response bodies are closed
- **Default values**: README now accurately reflects code defaults

## [1.0.0] - Initial Release

### New Added

- Basic artifact deletion functionality
- Concurrent processing with configurable limits
- Environment variable configuration
- Command-line flags support
- Retry logic for failed requests
- Project existence validation
- Logging to file

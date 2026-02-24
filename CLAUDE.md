# CLAUDE.md

## Project Overview

Server Health Check Telegram Bot -- a Go application that monitors server availability via HTTP checks and sends notifications to a Telegram chat. It runs periodic health checks on user-configured URLs and alerts when servers go down, respond slowly, or have expiring SSL certificates.

## Repository Structure

```
.
├── main.go                          # Entry point: CLI flag parsing, bot init, cron setup, graceful shutdown
├── go.mod / go.sum                  # Go module (go 1.23+, toolchain go1.24)
├── Dockerfile                       # Multi-stage Alpine build, exposes port 8081
├── docker/docker-compose.yml        # Docker Compose deployment config
├── .github/workflows/main.yml       # CI: builds and pushes Docker image on master/tags
└── app/
    ├── checks/
    │   ├── check.go                 # HTTP health check logic, SSL cert inspection, alert thresholds
    │   ├── check_test.go            # Tests for check logic (httptest-based)
    │   ├── storage.go               # JSON file-based persistence (data/checks.json)
    │   └── storage_test.go          # Tests for storage (uses temp dirs)
    ├── events/
    │   ├── telegram.go              # Telegram command handler and callback query processor
    │   ├── telegram_test.go         # Tests for URL parsing and server extraction
    │   ├── superuser.go             # SuperUser type for access control
    │   └── superuser_test.go        # Tests for superuser matching
    └── healthcheck/
        └── healthcheck.go           # /health HTTP endpoint for container health checks
```

## Build & Run

```bash
# Build
go build -o app .

# Run (requires Telegram bot token and chat ID)
./app --telegram.token=<TOKEN> --telegram.chat=<CHAT_ID> --super=<USERNAME>

# Run with Docker Compose
docker-compose -f docker/docker-compose.yml up -d
```

## Testing

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests for a specific package
go test ./app/checks/
go test ./app/events/
```

Tests use the standard `testing` package with `httptest` for HTTP mocking. Storage tests use `t.TempDir()` to isolate file operations. There are no external test dependencies.

## Verification After Implementing a Feature

After implementing a feature, always run the full check via Makefile:

```bash
make all        # runs: lint -> test -> build (full pipeline)
```

Or individual steps:

```bash
make lint       # golangci-lint run ./... (uses .golangci.yml config)
make test       # go test -v ./...
make build      # CGO_ENABLED=0 go build
```

Install linter if missing: `make tools` (installs golangci-lint v2.7.2).

## Key Architecture Decisions

- **No database**: Server check data is persisted as JSON in `data/checks.json` with a mutex for concurrency safety.
- **Cron-based scheduling**: Uses `robfig/cron/v3` with second-precision cron expressions (6-field format). Default: `*/30 * * * * *` (every 30 seconds).
- **Alert threshold**: Failures must exceed the threshold (default 3) before sending a "server down" notification. Recovery messages are only sent if a failure notification was previously sent.
- **SSL notifications**: Limited to once per 24 hours per server to prevent spam.
- **SuperUser access control**: Only Telegram users listed via `--super` flags can issue bot commands. Matching is case-insensitive.
- **Graceful shutdown**: Uses `signal.NotifyContext` for SIGINT/SIGTERM handling. Stops cron, stops Telegram update listener, sends shutdown message.

## Package Responsibilities

- **`checks`**: Core health check logic. `PerformCheck` iterates all monitored servers, makes HTTP requests, checks status codes/content/SSL, updates availability stats, and sends Telegram alerts. Package-level vars hold failure counters and the shared HTTP client.
- **`events`**: Telegram bot command routing. Handles `/add`, `/remove`, `/list`, `/stats`, `/details`, `/help`, and configuration commands. Also processes inline button callbacks.
- **`healthcheck`**: Lightweight HTTP server exposing `/health` endpoint on configurable port (default 8081). Checks Telegram API connectivity. Used by Docker HEALTHCHECK.

## Configuration (Environment Variables / CLI Flags)

| Variable | Flag | Default | Description |
|---|---|---|---|
| `TELEGRAM_TOKEN` | `--telegram.token` | (required) | Telegram bot API token |
| `TELEGRAM_CHAT` | `--telegram.chat` | (required) | Target Telegram chat ID |
| `ALERT_THRESHOLD` | `--alert-threshold` | `3` | Consecutive failures before alerting |
| `CHECKS_CRON` | `--checks-cron` | `*/30 * * * * *` | 6-field cron (with seconds) |
| `HTTP_TIMEOUT` | `--http-timeout` | `10` | HTTP request timeout in seconds |
| `SSL_EXPIRY_ALERT` | `--ssl-expiry-alert` | `30` | Days before SSL expiry to alert |
| `DEFAULT_RESPONSE_TIME` | `--default-response-time` | `0` | Response time threshold in ms (0=disabled) |
| `HEALTH_PORT` | `--health-port` | `8081` | Port for the /health endpoint |
| `DEBUG` | `--debug` | `false` | Enable debug logging |

SuperUsers are specified via repeated `--super=<username>` flags (not environment variables).

## CI/CD

GitHub Actions workflow (`.github/workflows/main.yml`) runs on pushes to `master` and version tags (`v*`). It builds the Docker image and pushes to:
- Docker Hub: `trueromancha/server-healthcheck`
- GitHub Container Registry: `ghcr.io/romancha/server-healthcheck`

No automated test step in CI -- tests should be run locally before pushing.

## Conventions

- **Go version**: 1.23+ (toolchain 1.24)
- **Logging**: Uses `go-pkgz/lgr` with level prefixes (`[DEBUG]`, `[INFO]`, `[ERROR]`). Use `log.Printf` with these prefixes.
- **Error handling**: Errors in background operations are logged, not returned. Fatal errors during startup use `log.Fatalf`.
- **Test style**: Table-driven tests with subtests (`t.Run`). No assertion libraries -- use standard `t.Errorf`/`t.Fatalf`.
- **Storage isolation in tests**: Use `setupTestStorage(t)` helper which redirects `storageLocation` to a temp directory.
- **Default branch**: `master` (not `main`).

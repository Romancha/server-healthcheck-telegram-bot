# Release Notes

## Version 1.1.0 — 24 Feb 2026

### 🎉 Major Features

**Health Check Endpoint and Graceful Shutdown**
- Added HTTP health check server (`/health`) that verifies Telegram API connectivity.
- Implemented graceful shutdown via SIGTERM/SIGINT signal handling — stops cron scheduler, Telegram polling, and health server; sends notification to chat on bot stop.
- Added configurable `HEALTH_PORT` (default: 8081) and `HEALTHCHECK` directive to Dockerfile and docker-compose.yml.

### 🆕 New
- Added CI pipeline with lint (`go vet`) and test (`go test -race`) steps in GitHub Actions workflow.
- Added comprehensive unit tests for all packages — coverage improved from 18.7% to 55.7%.

### ✨ Improvements
- Modernized Dockerfile with Go 1.26 and separate dependency download step.
- Switched Docker base image from `scratch` to `alpine` for curl availability.
- Updated docker-compose version and enhanced README with health check details and development instructions.
- Renamed `HttpTimeout` to `HTTPTimeout` to follow Go naming conventions.
- Added `ReadHeaderTimeout` to health check server to satisfy `gosec`.
- Improved error wrapping with `fmt.Errorf` across the codebase.

### 🐞 Fixes
- Fixed nil pointer panic on channel posts when `Message.From` is nil.
- Fixed data races in `serverFailureCount`/`serverSendFaultMessage` maps by adding mutex protection.
- Fixed callback query handler bypassing superuser authorization for inline buttons.
- Fixed "server removed" confirmation being sent before the save actually succeeded.
- Fixed `storageLocation` not being read under mutex in `InitStorage`.

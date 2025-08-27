# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a TCP relay service that receives Zscaler ZPA LSS (Log Streaming Service) data and forwards it to Splunk HEC (HTTP Event Collector). The application acts as a streaming middleware that persists logs locally as NDJSON files and optionally forwards them to Splunk HEC in real-time.

## Build and Development Commands

- `task build` - Build the binary to `.build/relay`
- `task test` - Run tests
- `task coverage` - Generate test coverage report
- `task fmt` - Format Go code
- `task vet` - Lint Go code with go vet
- `task check` - Run all security scans (SAST, vet, vuln)
- `task clean` - Clean build artifacts
- `task release` - Build release artifacts for multiple platforms
- `go run cmd/relay/main.go` - Run the application directly

## Application Architecture

### Package Structure

The project follows standard Go project layout with business logic properly separated:
- `cmd/relay/` - Main entry point (minimal, orchestrates internal packages)
- `internal/acl/` - CIDR-based access control
- `internal/config/` - Configuration management (YAML loading, validation)
- `internal/forwarder/` - Splunk HEC integration with retry logic
- `internal/processor/` - Data processing utilities (JSON validation, line reading)
- `internal/server/` - TCP/TLS server and connection handling
- `internal/storage/` - File persistence with daily rotation
- `version.go` - Version information for builds
- `spec/` - ZPA LSS log format specifications and examples

### Core Components

1. **Main Application** (`cmd/relay/main.go`): Minimal orchestrator that initializes and coordinates internal packages. Handles command-line flags and dependency injection.

2. **Server Package** (`internal/server/`): Manages TCP/TLS listeners, connection acceptance, and per-connection data processing. Coordinates storage and forwarding operations.

3. **Storage Package** (`internal/storage/`): Handles local file persistence with automatic daily rotation based on UTC dates.

4. **Forwarder Package** (`internal/forwarder/`): Manages Splunk HEC integration with configurable gzip compression and exponential backoff retry logic.

5. **ACL Package** (`internal/acl/`): Provides CIDR-based IP filtering for incoming connections.

6. **Processor Package** (`internal/processor/`): Utilities for data validation, line-limited reading, and text processing.

7. **Configuration Package** (`internal/config/`): Handles YAML configuration loading with embedded template support.

8. **Version Management** (`version.go`): Provides version information that gets injected during build via ldflags.

### Data Flow

1. TCP/TLS server accepts connections on configurable address (default `:9015`)
2. Optional CIDR-based access control for incoming connections
3. Incoming NDJSON data validated and line-limited for security
4. Data persisted locally to daily-rotated files (`zpa-YYYY-MM-DD.ndjson`)
5. Optional real-time forwarding to Splunk HEC raw endpoint with retry logic
6. Optional gzip compression for HEC payloads

### Configuration Approaches

The application uses command-line flags only (no YAML config file):
- Network: `-listen`, `-tls-cert`, `-tls-key`, `-allow-cidrs`
- Storage: `-out` (output directory)
- Splunk HEC: `-hec-url`, `-hec-token`, `-hec-sourcetype`, `-hec-gzip`
- Limits: `-max-line-bytes`

### Key Features

- **Daily Log Rotation**: Automatic file rotation based on UTC date
- **TLS Support**: Optional TLS encryption for incoming connections
- **Access Control**: CIDR-based IP filtering
- **Real-time Forwarding**: Concurrent local storage and HEC forwarding
- **Retry Logic**: Built-in retry mechanism for HEC failures with exponential backoff
- **Data Validation**: JSON validation for incoming log lines

## Dependencies

- `gopkg.in/yaml.v3` - YAML configuration parsing (config package only)
- Go standard library only for runtime dependencies
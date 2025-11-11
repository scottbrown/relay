![logo](relay.logo.large.png)

Relay is a high-performance TCP relay service that receives Zscaler ZPA LSS (Log Streaming Service) data and forwards it to Splunk HEC (HTTP Event Collector). The application acts as a streaming middleware that persists logs locally as NDJSON files and optionally forwards them to Splunk HEC in real-time.

## Features

- **Multi-Listener Support**: Configure multiple ports for different ZPA log types
- **TCP Server**: Accepts incoming connections from Zscaler ZPA LSS
- **Data Validation**: JSON validation for incoming log lines
- **Local Storage**: Daily-rotated NDJSON file persistence with configurable prefixes
- **Splunk HEC Integration**: Optional real-time forwarding to Splunk's HTTP Event Collector
- **Circuit Breaker**: Automatic failure detection and recovery for HEC forwarding resilience
- **TLS Support**: Optional TLS encryption for incoming connections
- **Access Control**: CIDR-based IP filtering per listener
- **YAML Configuration**: Required configuration file for all settings
- **Template Generation**: Built-in configuration template generator
- **Health Checks**: Smoke testing for Splunk HEC connectivity
- **Graceful Shutdown**: Handles system signals for clean service termination

## How it Works

```mermaid
sequenceDiagram
    autonumber
    participant AC as ZPA App Connector (appliance)
    participant ZC as Zscaler Cloud
    participant LSS as ZPA Log Streaming Service
    participant R as Relay
    participant FS as Local Storage (NDJSON)
    participant HEC as Splunk HEC (/services/collector/raw)

    Note over AC,ZC: Operational & audit data flows from App Connector up to Zscaler Cloud
    AC-->>ZC: Telemetry, audit, user/app metrics (proprietary)

    Note over LSS,R: LSS initiates outbound connection to your receiver
    LSS-)R: TCP/TLS connect to relay:port
    activate R

    loop Streaming (long-lived socket)
      LSS-->>R: NDJSON event "\n" (one JSON object per line)
      R->>FS: append line to zpa-YYYY-MM-DD.ndjson
      alt HEC forwarding enabled
        R->>HEC: HTTPS POST (optional gzip) with original JSON line
        HEC-->>R: 2xx (ingested)  / non-2xx (retry/backoff)
      end
    end
    deactivate R

    Note over R,HEC: Relay may batch/retry. Splunk indexes by event time or ingest time depending on props
```

## Requirements

- Go 1.21 or later
- Access to a Splunk instance with HEC enabled
- Network connectivity between Zscaler ZPA and the relay service

## Installation

### From Source

```bash
git clone https://github.com/scottbrown/relay.git
cd relay
go build -o relay cmd/relay/main.go
```

### Using Task Runner

If you have [Task](https://taskfile.dev/) installed:

```bash
task build
```

This creates the binary at `.build/relay`.

## Configuration

### Configuration File (Required)

The application requires a YAML configuration file. Create one using the template generator:

```bash
./relay template > config.yml
```

Example configuration with multiple listeners:

```yaml
# Global Splunk HEC configuration (shared across all listeners unless overridden)
splunk:
  hec_url: "https://your-instance.splunkcloud.com:8088/services/collector/raw"
  hec_token: "your-hec-token-here"
  gzip: true
  # Circuit breaker configuration for HEC forwarding resilience
  circuit_breaker:
    enabled: true                 # Enable/disable circuit breaker (default: true)
    failure_threshold: 5          # Open circuit after N consecutive failures (default: 5)
    success_threshold: 2          # Close circuit after N consecutive successes (default: 2)
    timeout_seconds: 30           # Seconds before testing recovery (default: 30)
    half_open_max_calls: 1        # Max concurrent test calls in half-open state (default: 1)

# Global healthcheck configuration
health_check_enabled: true
health_check_addr: ":9099"

# Listener configurations (one per ZPA log type)
listeners:
  # User Activity logs
  - name: "user-activity"
    listen_addr: ":9015"
    log_type: "user-activity"
    output_dir: "./zpa-logs"
    file_prefix: "zpa-user-activity"
    allowed_cidrs: "10.0.0.0/8"
    max_line_bytes: 1048576
    splunk:
      source_type: "zpa:user:activity"

  # User Status logs
  - name: "user-status"
    listen_addr: ":9016"
    log_type: "user-status"
    output_dir: "./zpa-logs"
    file_prefix: "zpa-user-status"
    allowed_cidrs: "10.0.0.0/8"
    max_line_bytes: 1048576
    splunk:
      source_type: "zpa:user:status"
```

### Generate Configuration Template

```bash
./relay template > config.yml
```

### Global Configuration Options

| Option | Description | Required | Default |
|--------|-------------|----------|---------|
| `splunk.hec_url` | Global Splunk HEC raw endpoint URL | No | - |
| `splunk.hec_token` | Global Splunk HEC authentication token | No | - |
| `splunk.gzip` | Global gzip compression for HEC | No | - |
| `splunk.circuit_breaker.enabled` | Enable circuit breaker for HEC | No | `true` |
| `splunk.circuit_breaker.failure_threshold` | Failures before opening circuit | No | `5` |
| `splunk.circuit_breaker.success_threshold` | Successes before closing circuit | No | `2` |
| `splunk.circuit_breaker.timeout_seconds` | Seconds before testing recovery | No | `30` |
| `splunk.circuit_breaker.half_open_max_calls` | Max concurrent calls in half-open | No | `1` |
| `health_check_enabled` | Enable healthcheck endpoint | No | `false` |
| `health_check_addr` | Healthcheck listen address | No | `:9099` |

### Per-Listener Configuration Options

| Option | Description | Required | Default |
|--------|-------------|----------|---------|
| `name` | Friendly identifier for the listener | Yes | - |
| `listen_addr` | TCP listen address | Yes | - |
| `log_type` | ZPA log type (must be valid) | Yes | - |
| `output_dir` | Directory for NDJSON files | Yes | - |
| `file_prefix` | File naming prefix | Yes | - |
| `tls.cert_file` | TLS certificate file | No | - |
| `tls.key_file` | TLS key file | No | - |
| `allowed_cidrs` | Comma-separated allowed CIDRs | No | - |
| `max_line_bytes` | Max bytes per JSON line | No | `1048576` |
| `splunk.source_type` | Splunk sourcetype for this listener | Yes* | - |
| `splunk.hec_url` | Override global HEC URL | No | - |
| `splunk.hec_token` | Override global HEC token | No | - |
| `splunk.gzip` | Override global gzip setting | No | - |

\* Required if global or per-listener HEC is configured

### Valid Log Types

- `user-activity`
- `user-status`
- `app-connector-status`
- `pse-status`
- `browser-access`
- `audit`
- `app-connector-metrics`
- `pse-metrics`

### Startup Configuration Validation

The application performs comprehensive "fail fast" validation during startup to detect configuration issues before beginning normal operation. This ensures runtime failures are minimised and problems are caught early.

**Validations Performed:**

1. **TLS Certificate Validation**
   - Verifies TLS certificate and key files exist and are readable
   - Loads the certificate to ensure it's valid and properly formatted
   - Both cert and key must be specified together

2. **Storage Directory Validation**
   - Creates output directories if they don't exist (including nested paths)
   - Tests directory writability by creating a temporary test file
   - Ensures proper permissions before accepting connections

3. **Splunk HEC Configuration Validation**
   - Validates HEC URL format (must be HTTP or HTTPS with valid host)
   - Ensures HEC token is present when HEC URL is configured
   - Verifies sourcetype is specified when HEC forwarding is enabled

4. **Network Configuration Validation**
   - Verifies listen addresses are available by attempting to bind
   - Detects port conflicts and address-in-use errors at startup
   - Tests actual network connectivity before starting service

5. **CIDR Access Control Validation**
   - Parses and validates CIDR notation for allowed_cidrs
   - Ensures proper IP address and subnet mask format
   - Detects invalid CIDR expressions early

**Example Error Messages:**

```
Error: listener user-activity: TLS cert file not accessible: open /path/to/cert.pem: no such file or directory
Error: listener user-activity: failed to load TLS certificate: tls: failed to find any PEM data
Error: listener user-activity: output directory not writable: permission denied
Error: listener user-activity: invalid HEC URL: HEC URL must use http or https scheme
Error: listener user-activity: HEC token required when HEC URL is specified
Error: listener user-activity: invalid CIDR list: invalid CIDR address: invalid-cidr
Error: listener user-activity: cannot bind to listen address: address already in use
```

**Benefits:**

- Detects configuration errors immediately at startup
- Provides clear, actionable error messages
- Prevents runtime failures during normal operation
- Reduces mean time to resolution for configuration issues

## Usage

### Basic Usage

```bash
# Run with configuration file (required)
./relay --config /path/to/config.yml

# Short form
./relay -f config.yml

# Generate configuration template
./relay template > config.yml

# Test Splunk HEC connectivity for all listeners
./relay smoke-test --config config.yml
```

### Commands

| Command | Description |
|---------|-------------|
| (default) | Start the relay service |
| `template` | Generate configuration template and exit |
| `smoke-test` | Test Splunk HEC connectivity for all listeners and exit |

### Command-Line Options

```bash
./relay [command] --config <path>

Options:
  -f, --config string
        Path to configuration file (required)
```

### Running Directly with Go

```bash
go run cmd/relay/main.go --config config.yml
```

## Architecture

### Data Flow

1. **Multi-Listener Setup**: Configure multiple TCP/TLS listeners, one per ZPA log type
2. **Access Control**: Optional CIDR-based filtering for incoming connections per listener
3. **Data Validation**: Incoming NDJSON data is validated and line-limited for security
4. **Local Storage**: Data is persisted locally to daily-rotated files ({file_prefix}-YYYY-MM-DD.ndjson)
5. **Real-time Forwarding**: Optional concurrent forwarding to Splunk HEC raw endpoint with retry logic and circuit breaker protection

### Circuit Breaker Pattern

The circuit breaker protects the application from cascading failures when Splunk HEC is down or experiencing issues. It implements a state machine with three states:

**States:**

1. **Closed (Normal Operation)**
   - All HEC forward attempts proceed normally
   - Consecutive failures are tracked
   - Circuit opens after reaching failure threshold (default: 5 failures)

2. **Open (Failing Fast)**
   - HEC forward attempts are immediately rejected without trying
   - Logs continue to be stored locally
   - After timeout period (default: 30s), circuit transitions to half-open

3. **Half-Open (Testing Recovery)**
   - Limited number of test requests allowed through (default: 1)
   - If test succeeds: circuit closes and normal operation resumes
   - If test fails: circuit reopens and timeout restarts

**Example Behaviour:**

```
# HEC is healthy
[INFO] forwarding to HEC (state=closed)

# HEC starts failing
[WARN] HEC forward failed, attempt 1/5
[WARN] HEC forward failed, attempt 5/5
[WARN] circuit breaker opened after 5 consecutive failures

# Circuit is open (failing fast, logs still stored locally)
[WARN] circuit breaker open, skipping HEC forward

# After 30s timeout
[INFO] circuit breaker half-open, testing recovery
[INFO] HEC forward successful
[INFO] circuit breaker closed, HEC recovered
```

**Benefits:**
- Prevents wasting resources on doomed requests
- Enables faster failure detection
- Automatic recovery when HEC becomes healthy
- Logs are always stored locally regardless of HEC state
- Can be disabled by setting `failure_threshold: 0`

### Event Format

Data is forwarded to Splunk HEC as raw JSON events (one per line) without additional wrapping. The sourcetype is configurable per listener (e.g., "zpa:user:activity", "zpa:audit").

## Monitoring and Logging

The service logs to stdout and includes:
- Connection status messages
- Batch processing information
- Error conditions
- Graceful shutdown notifications

## Development

### Prerequisites

- Go 1.24.4+
- Task runner (optional)

### Building

```bash
# Using Go directly
go build -o relay cmd/relay/main.go

# Using Task
task build
```

### Running Tests

```bash
# Run unit tests
go test -v ./...

# Run with coverage
task coverage

# Run integration tests
task integration
```

### Integration Testing

The project includes a comprehensive integration test harness that validates the complete pipeline without requiring live ZPA App Connectors or Splunk HEC instances.

**Running Integration Tests:**

```bash
# Using Task runner (recommended)
task integration

# Or directly with Go
go test -tags=integration -v ./internal/integration/...
```

**Test Coverage:**

The integration tests validate:
- End-to-end data flow (happy path)
- Malformed JSON handling
- Oversized line rejection
- HEC failure and retry logic
- Gzip compression
- CIDR-based access control

**Test Infrastructure:**

- Mock ZPA client for streaming NDJSON logs
- Mock Splunk HEC server with configurable responses
- Relay launcher for temporary test instances
- Test fixtures for various scenarios

For detailed information about the integration test harness, see [TESTING.SPEC.md](TESTING.SPEC.md).

### Performance Benchmarks

The project includes comprehensive benchmarks for critical performance paths to help identify bottlenecks and prevent regressions.

**Running Benchmarks:**

```bash
# Using Task runner (recommended)
task bench

# Or directly with Go
go test -bench=. -benchmem -run=^$ ./internal/...

# Run specific package benchmarks
go test -bench=. -benchmem ./internal/processor/
go test -bench=. -benchmem ./internal/storage/
go test -bench=. -benchmem ./internal/forwarder/
go test -bench=. -benchmem ./internal/server/

# Save baseline for comparison
go test -bench=. -benchmem ./internal/... > baseline.txt

# Compare with previous baseline using benchstat
go install golang.org/x/perf/cmd/benchstat@latest
benchstat baseline.txt new.txt
```

**Benchmark Coverage:**

The benchmarks validate performance of:
- **Processor Package**: Line reading with various sizes (100B to 1MB), JSON validation, oversized line handling
- **Storage Package**: Write operations with different payload sizes, concurrent writes, file rotation logic
- **Forwarder Package**: HEC forwarding with/without gzip, different payload sizes, retry logic overhead
- **Server Package**: End-to-end connection handling with various scenarios

**Interpreting Results:**

Benchmark results show:
- **ns/op**: Nanoseconds per operation (lower is better)
- **B/op**: Bytes allocated per operation (lower is better)
- **allocs/op**: Number of allocations per operation (lower is better)
- **MB/s**: Throughput in megabytes per second (when applicable, higher is better)

**Example Output:**

```
BenchmarkReadLineLimited_Small-8       1000000    1234 ns/op    512 B/op    4 allocs/op
BenchmarkWrite_Medium-8                 500000    2345 ns/op   1024 B/op    2 allocs/op
BenchmarkForward_Large_Gzip-8           100000   12345 ns/op   2048 B/op    8 allocs/op
```

### Dependencies

- `gopkg.in/yaml.v3` - YAML configuration parsing
- Go standard library only

## Deployment

### Systemd Service

Create `/etc/systemd/system/relay.service`:

```ini
[Unit]
Description=Zscaler ZPA LSS Relay Service
After=network.target

[Service]
Type=simple
User=relay
ExecStart=/usr/local/bin/relay -f /etc/relay/config.yml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl enable relay
sudo systemctl start relay
```

### Docker

```dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o relay cmd/relay/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/relay .
COPY config.yml .
CMD ["./relay", "-f", "config.yml"]
```

## Troubleshooting

### Common Issues

1. **Connection Refused**: Ensure the listen port is not in use and firewall allows connections
2. **Splunk HEC Errors**: Verify HEC URL and token are correct
3. **Permission Denied**: Check file permissions for configuration file
4. **Memory Usage**: Monitor batch size settings for high-volume environments

### Logs

Check service logs for diagnostic information:

```bash
# Systemd
journalctl -u relay -f

# Docker
docker logs <container_id>
```

## Documentation

Documentation is organised using the [Diátaxis framework](https://diataxis.fr/) for clarity and discoverability. See [docs/](docs/) for the complete documentation structure.

### Architecture Decision Records

Key architectural decisions and their rationale are documented as Architecture Decision Records (ADRs). See [docs/explanation/adr/](docs/explanation/adr/) for the complete list of decisions including:

- Why we use Go Task instead of Make
- Daily log rotation based on UTC
- NDJSON format for local storage
- Store-first, forward-second approach
- And more...

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Support

For issues and questions:
- Create an issue in the GitHub repository
- Check the troubleshooting section above
- Review logs for error messages

# Configuration Reference

*Complete reference for all relay configuration options.*

This document provides comprehensive technical descriptions of all configuration parameters available in the relay service configuration file.

## Table of Contents

- [Configuration File Format](#configuration-file-format)
- [Top-Level Configuration](#top-level-configuration)
- [Listener Configuration](#listener-configuration)
- [Splunk HEC Configuration](#splunk-hec-configuration)
- [Multi-Target HEC Configuration](#multi-target-hec-configuration)
- [TLS Configuration](#tls-configuration)
- [Batch Configuration](#batch-configuration)
- [Circuit Breaker Configuration](#circuit-breaker-configuration)
- [Retry Configuration](#retry-configuration)
- [Timeout Configuration](#timeout-configuration)
- [Dead Letter Queue Configuration](#dead-letter-queue-configuration)
- [Log Retention Configuration](#log-retention-configuration)
- [Configuration Hierarchy](#configuration-hierarchy)
- [Validation Rules](#validation-rules)
- [Configuration Examples](#configuration-examples)

## Configuration File Format

The relay service uses YAML format for configuration. The configuration file must be specified via the `--config` flag when starting the service.

**File Location**: User-specified via `--config` flag (e.g., `./relay --config /etc/relay/config.yml`)

**Format**: YAML 1.2

**Character Encoding**: UTF-8

**Comments**: Supported using `#` character

**Generate Template**:
```bash
./relay template > config.yml
```

## Top-Level Configuration

Configuration parameters that apply to the entire relay service.

| Parameter | Type | Required | Default | Reloadable | Description |
|-----------|------|----------|---------|------------|-------------|
| `splunk` | [SplunkConfig](#splunk-hec-configuration) | No | - | Partial* | Global Splunk HEC configuration (inherited by listeners) |
| `health_check_enabled` | boolean | No | `false` | No | Enable health check HTTP server |
| `health_check_addr` | string | No | `:9099` | No | Address for health check server (format: `:port` or `host:port`) |
| `listeners` | [][ListenerConfig](#listener-configuration) | Yes | - | No** | Array of listener configurations (minimum 1 required) |

\* Only `hec_token`, `source_type`, and `gzip` are reloadable within the `splunk` section.
\** Cannot add/remove listeners via reload, but individual listener parameters may be reloadable.

### Example: Minimal Top-Level Configuration

```yaml
health_check_enabled: true
health_check_addr: ":9099"

listeners:
  - name: "user-activity"
    listen_addr: ":9015"
    log_type: "user-activity"
    output_dir: "./logs"
    file_prefix: "zpa"
```

## Listener Configuration

Each listener accepts connections for a specific log type on a designated port.

| Parameter | Type | Required | Default | Reloadable | Description |
|-----------|------|----------|---------|------------|-------------|
| `name` | string | Yes | - | No | Unique identifier for this listener (used in logs) |
| `listen_addr` | string | Yes | - | No | TCP address to listen on (format: `:port` or `host:port`) |
| `log_type` | string | Yes | - | No | ZPA log type (see [valid log types](#valid-log-types)) |
| `output_dir` | string | Yes | - | No | Local directory for NDJSON file storage |
| `file_prefix` | string | Yes | - | No | Prefix for daily log files (e.g., `zpa-user` creates `zpa-user-2025-11-14.ndjson`) |
| `tls` | [TLSConfig](#tls-configuration) | No | - | No | TLS encryption configuration for incoming connections |
| `allowed_cidrs` | string | No | `""` | **Yes** | Comma-separated CIDR ranges for access control (empty = allow all) |
| `max_line_bytes` | integer | No | `1048576` (1 MiB) | No | Maximum bytes per log line (prevents DoS) |
| `timeout` | [TimeoutConfig](#timeout-configuration) | No | - | No | Connection timeout configuration |
| `dlq` | [DLQConfig](#dead-letter-queue-configuration) | No | - | No | Dead letter queue configuration for failed forwards |
| `splunk` | [SplunkConfig](#splunk-hec-configuration) | No | - | Partial* | Per-listener Splunk HEC configuration (overrides global) |

\* Only `hec_token`, `source_type`, and `gzip` are reloadable.

### Valid Log Types

| Log Type | Description |
|----------|-------------|
| `user-activity` | User access and activity logs |
| `user-status` | User connection status events |
| `app-connector-status` | App Connector health and status |
| `pse-status` | Private Service Edge status |
| `browser-access` | Browser access logs |
| `audit` | Administrative audit logs |
| `app-connector-metrics` | App Connector performance metrics |
| `pse-metrics` | Private Service Edge metrics |

### Example: Listener Configuration

```yaml
listeners:
  - name: "user-activity"
    listen_addr: ":9015"
    log_type: "user-activity"
    output_dir: "/var/log/relay/zpa"
    file_prefix: "zpa-user-activity"
    allowed_cidrs: "10.0.0.0/8, 172.16.0.0/12"
    max_line_bytes: 2097152  # 2 MiB
    tls:
      cert_file: "/etc/relay/certs/relay.crt"
      key_file: "/etc/relay/certs/relay.key"
```

## Splunk HEC Configuration

Configuration for forwarding logs to Splunk HTTP Event Collector.

Splunk configuration can be specified at two levels:
1. **Global** (`splunk` at top level): Inherited by all listeners
2. **Per-Listener** (`splunk` within listener): Overrides global settings

### Single-Target Configuration

For forwarding to a single Splunk HEC endpoint.

| Parameter | Type | Required | Default | Reloadable | Description |
|-----------|------|----------|---------|------------|-------------|
| `hec_url` | string | Yes* | - | No | Splunk HEC endpoint URL (must be `http://` or `https://`) |
| `hec_token` | string | Yes* | - | **Yes** | Splunk HEC authentication token (keep secret) |
| `source_type` | string | Yes* | - | **Yes** | Splunk sourcetype for events (e.g., `zpa:user:activity`) |
| `gzip` | boolean | No | `false` | **Yes** | Enable gzip compression for HEC requests |
| `client_timeout_seconds` | integer | No | `15` | No | HTTP client timeout for HEC requests in seconds |
| `batch` | [BatchConfig](#batch-configuration) | No | See defaults | No | Batch forwarding configuration |
| `circuit_breaker` | [CircuitBreakerConfig](#circuit-breaker-configuration) | No | See defaults | No | Circuit breaker configuration |
| `retry` | [RetryConfig](#retry-configuration) | No | See defaults | No | Retry configuration for failed requests |

\* Required if HEC forwarding is enabled. Can be omitted entirely to disable forwarding.

### Example: Single-Target Splunk Configuration

```yaml
# Global configuration (inherited by all listeners)
splunk:
  hec_url: "https://splunk.example.com:8088/services/collector/raw"
  hec_token: "your-secret-token-here"
  gzip: true
  batch:
    enabled: true
    max_size: 100
    max_bytes: 1048576
    flush_interval_seconds: 1
```

## Multi-Target HEC Configuration

For forwarding to multiple Splunk HEC endpoints with configurable routing.

**Note**: Cannot mix single-target (`hec_url`/`hec_token`) and multi-target (`hec_targets`) configuration in the same scope.

| Parameter | Type | Required | Default | Reloadable | Description |
|-----------|------|----------|---------|------------|-------------|
| `hec_targets` | [][HECTarget](#hec-target-configuration) | Yes | - | No | Array of HEC target configurations |
| `routing` | [RoutingConfig](#routing-configuration) | No | `{mode: "all"}` | No | Routing strategy for multi-target forwarding |

### HEC Target Configuration

Individual HEC target within `hec_targets` array.

| Parameter | Type | Required | Default | Reloadable | Description |
|-----------|------|----------|---------|------------|-------------|
| `name` | string | Yes | - | No | Unique identifier for this target |
| `hec_url` | string | Yes | - | No | Splunk HEC endpoint URL for this target |
| `hec_token` | string | Yes | - | **Yes** | HEC authentication token for this target |
| `source_type` | string | Yes | - | **Yes** | Splunk sourcetype for this target |
| `gzip` | boolean | No | `false` | **Yes** | Enable gzip compression for this target |
| `client_timeout_seconds` | integer | No | `15` | No | HTTP client timeout for this target in seconds |
| `batch` | [BatchConfig](#batch-configuration) | No | See defaults | No | Per-target batch configuration |
| `circuit_breaker` | [CircuitBreakerConfig](#circuit-breaker-configuration) | No | See defaults | No | Per-target circuit breaker configuration |
| `retry` | [RetryConfig](#retry-configuration) | No | See defaults | No | Per-target retry configuration |

### Routing Configuration

| Parameter | Type | Required | Default | Reloadable | Description |
|-----------|------|----------|---------|------------|-------------|
| `mode` | string | No | `"all"` | No | Routing mode: `"all"`, `"primary-failover"`, or `"round-robin"` |

**Routing Modes:**

- **`all`** (Broadcast): Send logs to all targets concurrently
  - Use case: High availability, disaster recovery, multi-tenancy
  - Behaviour: Continues even if some targets fail

- **`primary-failover`**: Try targets in order, failover on error
  - Use case: Primary/backup Splunk configuration
  - Behaviour: Only uses secondary if primary fails

- **`round-robin`**: Distribute logs evenly across targets
  - Use case: Load balancing across multiple indexers
  - Behaviour: Each log sent to one target in rotation

### Example: Multi-Target HEC Configuration

```yaml
splunk:
  hec_targets:
    - name: "primary"
      hec_url: "https://splunk1.example.com:8088/services/collector/raw"
      hec_token: "primary-token"
      source_type: "zpa:logs"
      gzip: true
    - name: "backup"
      hec_url: "https://splunk2.example.com:8088/services/collector/raw"
      hec_token: "backup-token"
      source_type: "zpa:logs"
      gzip: true
  routing:
    mode: "primary-failover"
```

## TLS Configuration

Configuration for TLS encryption on incoming connections.

| Parameter | Type | Required | Default | Reloadable | Description |
|-----------|------|----------|---------|------------|-------------|
| `cert_file` | string | Yes* | - | No | Path to TLS certificate file (PEM format) |
| `key_file` | string | Yes* | - | No | Path to TLS private key file (PEM format) |

\* Both `cert_file` and `key_file` must be specified together or both omitted.

**TLS Version**: Minimum TLS 1.2 (enforced by application)

**Certificate Format**: PEM (text format starting with `-----BEGIN CERTIFICATE-----`)

**File Permissions**: Recommended `600` for key file, `644` for certificate file

### Example: TLS Configuration

```yaml
listeners:
  - name: "user-activity"
    listen_addr: ":9015"
    log_type: "user-activity"
    output_dir: "./logs"
    file_prefix: "zpa"
    tls:
      cert_file: "/etc/relay/tls/server.crt"
      key_file: "/etc/relay/tls/server.key"
```

## Batch Configuration

Configuration for batching multiple log lines before forwarding to HEC.

Batching improves throughput by reducing HTTP request overhead.

| Parameter | Type | Required | Default | Reloadable | Description |
|-----------|------|----------|---------|------------|-------------|
| `enabled` | boolean | No | `false` | No | Enable batch forwarding |
| `max_size` | integer | No | `100` | No | Maximum lines per batch |
| `max_bytes` | integer | No | `1048576` (1 MiB) | No | Maximum bytes per batch |
| `flush_interval_seconds` | integer | No | `1` | No | Maximum seconds before flushing batch |

**Flush Triggers**: Batch is flushed when ANY of these conditions is met:
1. Line count reaches `max_size`
2. Total bytes reach `max_bytes`
3. Time since first line reaches `flush_interval_seconds`
4. Service shutdown initiated

**Performance Impact**:
- Enabled: Higher throughput, slightly higher latency
- Disabled: Lower latency, higher HTTP overhead

### Example: Batch Configuration

```yaml
splunk:
  hec_url: "https://splunk.example.com:8088/services/collector/raw"
  hec_token: "token"
  batch:
    enabled: true
    max_size: 500               # Flush after 500 lines
    max_bytes: 5242880          # Flush after 5 MiB
    flush_interval_seconds: 5   # Flush after 5 seconds
```

## Circuit Breaker Configuration

Configuration for circuit breaker pattern to protect against cascading failures.

The circuit breaker prevents wasting resources on failed HEC endpoints.

| Parameter | Type | Required | Default | Reloadable | Description |
|-----------|------|----------|---------|------------|-------------|
| `enabled` | boolean | No | `true` | No | Enable circuit breaker protection |
| `failure_threshold` | integer | No | `5` | No | Consecutive failures before opening circuit |
| `success_threshold` | integer | No | `2` | No | Consecutive successes in half-open to close circuit |
| `timeout_seconds` | integer | No | `30` | No | Seconds in open state before testing recovery |
| `half_open_max_calls` | integer | No | `1` | No | Maximum concurrent test requests in half-open state |

**Circuit States**:

1. **Closed** (Normal): All requests proceed
2. **Open** (Failing): Requests immediately rejected, logs stored locally only
3. **Half-Open** (Testing): Limited test requests allowed to check recovery

**State Transitions**:
- Closed → Open: After `failure_threshold` consecutive failures
- Open → Half-Open: After `timeout_seconds` elapsed
- Half-Open → Closed: After `success_threshold` consecutive successes
- Half-Open → Open: After any failure

### Example: Circuit Breaker Configuration

```yaml
splunk:
  hec_url: "https://splunk.example.com:8088/services/collector/raw"
  hec_token: "token"
  circuit_breaker:
    enabled: true
    failure_threshold: 5       # Open after 5 failures
    success_threshold: 2       # Close after 2 successes
    timeout_seconds: 30        # Test recovery after 30s
    half_open_max_calls: 1     # Only 1 test request at a time
```

## Retry Configuration

Configuration for retry behaviour with exponential backoff when HEC requests fail.

The retry mechanism automatically retries failed HEC requests with increasing delays.

| Parameter | Type | Required | Default | Reloadable | Description |
|-----------|------|----------|---------|------------|-------------|
| `max_attempts` | integer | No | `5` | No | Maximum number of retry attempts per request |
| `initial_backoff_ms` | integer | No | `250` | No | Initial backoff duration in milliseconds |
| `backoff_multiplier` | float | No | `2.0` | No | Exponential backoff multiplier |
| `max_backoff_seconds` | integer | No | `30` | No | Maximum backoff duration in seconds |

**Backoff Calculation**:

The backoff duration grows exponentially with each retry:
```
backoff = min(initial_backoff_ms * (backoff_multiplier ^ attempt), max_backoff_seconds * 1000)
```

**Example**: With defaults (initial: 250ms, multiplier: 2.0, max: 30s):
- Attempt 1: No delay (initial attempt)
- Attempt 2: 250ms delay
- Attempt 3: 500ms delay
- Attempt 4: 1000ms delay
- Attempt 5: 2000ms delay

**Retry Triggers**: Requests are retried when:
- Network connection fails
- HTTP response status is not 2xx
- Request timeout occurs

**Performance Impact**:
- Higher `max_attempts`: More resilient but longer delay on persistent failures
- Lower `initial_backoff_ms`: Faster recovery but higher load on failing endpoint
- Higher `backoff_multiplier`: Faster backoff growth, reduces load on failing endpoint

### Example: Retry Configuration

```yaml
splunk:
  hec_url: "https://splunk.example.com:8088/services/collector/raw"
  hec_token: "token"
  retry:
    max_attempts: 3             # Try up to 3 times
    initial_backoff_ms: 100     # Start with 100ms delay
    backoff_multiplier: 2.0     # Double the delay each time
    max_backoff_seconds: 10     # Cap delay at 10 seconds
```

### Example: Aggressive Retry for Reliable Endpoints

For endpoints with intermittent issues that resolve quickly:

```yaml
splunk:
  hec_url: "https://splunk.example.com:8088/services/collector/raw"
  hec_token: "token"
  retry:
    max_attempts: 10            # Many attempts
    initial_backoff_ms: 50      # Fast initial retry
    backoff_multiplier: 1.5     # Slower growth
    max_backoff_seconds: 5      # Low maximum delay
```

### Example: Conservative Retry for Overloaded Endpoints

For endpoints that need time to recover:

```yaml
splunk:
  hec_url: "https://splunk.example.com:8088/services/collector/raw"
  hec_token: "token"
  retry:
    max_attempts: 3             # Fewer attempts
    initial_backoff_ms: 1000    # Start with 1 second
    backoff_multiplier: 3.0     # Aggressive backoff
    max_backoff_seconds: 60     # Allow long delays
```

## Timeout Configuration

Configuration for connection and HTTP client timeouts to prevent resource exhaustion and hung connections.

### TCP Connection Timeouts

TCP connection timeouts prevent slow or hung clients from exhausting server resources.

| Parameter | Type | Required | Default | Reloadable | Description |
|-----------|------|----------|---------|------------|-------------|
| `read_seconds` | integer | No | None | No | Maximum seconds to wait for each read operation |
| `idle_seconds` | integer | No | None | No | Maximum idle seconds between reads before closing connection |

**Timeout Behaviour**:
- **read_seconds**: Applied to each individual read operation. If a single read takes longer than this, the connection is closed.
- **idle_seconds**: Maximum time connection can be idle between reads. If no data arrives within this time, the connection is closed.
- **Both configured**: The shorter timeout is used for each read operation.
- **Neither configured**: No timeouts applied (connections can remain idle indefinitely).

**When to Use**:
- **read_seconds**: Protect against slow clients that take too long per read
- **idle_seconds**: Protect against idle connections that hold resources
- **Production**: Recommend configuring both to prevent resource exhaustion

### HEC Client Timeout

HTTP client timeout for requests to Splunk HEC endpoints.

| Parameter | Type | Required | Default | Reloadable | Description |
|-----------|------|----------|---------|------------|-------------|
| `client_timeout_seconds` | integer | No | `15` | No | Total HTTP request timeout including connection, send, and response |

**Timeout Scope**: Covers entire HTTP request lifecycle:
1. DNS resolution
2. TCP connection establishment
3. TLS handshake (if HTTPS)
4. Sending request body
5. Waiting for response headers
6. Reading response body

**Performance Impact**:
- **Too short**: Requests may timeout on slow networks or busy Splunk instances
- **Too long**: Failed requests take longer to detect, increasing latency
- **Recommendation**: Set based on network latency + expected Splunk response time + buffer

### Example: TCP Connection Timeouts

Basic connection timeout configuration:

```yaml
listeners:
  - name: "user-activity"
    listen_addr: ":9015"
    log_type: "user-activity"
    output_dir: "/var/log/relay"
    file_prefix: "zpa-user-activity"
    timeout:
      read_seconds: 300     # 5 minutes per read operation
      idle_seconds: 600     # 10 minutes idle before disconnect
```

### Example: Aggressive Timeouts for High-Volume

For high-volume environments where connections should not linger:

```yaml
listeners:
  - name: "user-activity"
    listen_addr: ":9015"
    log_type: "user-activity"
    output_dir: "/var/log/relay"
    file_prefix: "zpa-user-activity"
    timeout:
      read_seconds: 30      # 30 seconds per read
      idle_seconds: 60      # 1 minute idle before disconnect
```

### Example: Lenient Timeouts for Slow Clients

For environments with potentially slow or intermittent clients:

```yaml
listeners:
  - name: "user-activity"
    listen_addr: ":9015"
    log_type: "user-activity"
    output_dir: "/var/log/relay"
    file_prefix: "zpa-user-activity"
    timeout:
      read_seconds: 900     # 15 minutes per read
      idle_seconds: 1800    # 30 minutes idle before disconnect
```

### Example: HEC Client Timeout

Adjust HEC timeout for slow or distant Splunk instances:

```yaml
splunk:
  hec_url: "https://splunk-distant.example.com:8088/services/collector/raw"
  hec_token: "token"
  client_timeout_seconds: 30   # Increase from 15s default for slow network
```

Per-target HEC timeout for multi-target configuration:

```yaml
splunk:
  hec_targets:
    - name: "local"
      hec_url: "https://splunk-local.example.com:8088/services/collector/raw"
      hec_token: "local-token"
      client_timeout_seconds: 10    # Fast local network
    - name: "remote"
      hec_url: "https://splunk-remote.example.com:8088/services/collector/raw"
      hec_token: "remote-token"
      client_timeout_seconds: 45    # Slow remote network
```

## Dead Letter Queue Configuration

Configuration for dead letter queue (DLQ) to capture failed HEC forwards for later analysis or replay.

When HEC forwards fail after all retry attempts are exhausted, the original log data is written to the DLQ with metadata about the failure. This prevents data loss during extended HEC outages and provides visibility into forwarding failures.

### DLQ Parameters

| Parameter | Type | Required | Default | Reloadable | Description |
|-----------|------|----------|---------|------------|-------------|
| `enabled` | boolean | No | `false` | No | Enable/disable dead letter queue |
| `directory` | string | No | `{output_dir}/dlq` | No | Directory for DLQ files |

**File Format**: DLQ entries are written as NDJSON files named `dlq-YYYY-MM-DD.ndjson` with daily rotation.

**Entry Structure**: Each DLQ entry contains:
```json
{
  "timestamp": "2025-11-14T15:30:00Z",
  "conn_id": "connection-id",
  "error": "error message describing the failure",
  "data": "original log line that failed to forward"
}
```

**When DLQ Entries are Written**:
- After all retry attempts are exhausted
- After circuit breaker opens (prevents forward attempts)
- Network errors, HTTP errors, timeout errors
- Both single-line forwards and batch forwards

**DLQ vs Local Storage**:
- **Local storage**: All received logs (successful or not)
- **DLQ**: Only logs that failed to forward after retries
- DLQ provides forensics and replay capability for failures

### Example: Basic DLQ Configuration

Enable DLQ with default directory:

```yaml
listeners:
  - name: "user-activity"
    listen_addr: ":9015"
    log_type: "user-activity"
    output_dir: "/var/log/relay"
    file_prefix: "zpa-user-activity"
    dlq:
      enabled: true
      # directory defaults to /var/log/relay/dlq
    splunk:
      hec_url: "https://splunk.example.com:8088/services/collector/raw"
      hec_token: "token"
```

### Example: Custom DLQ Directory

Specify custom DLQ directory:

```yaml
listeners:
  - name: "user-activity"
    listen_addr: ":9015"
    log_type: "user-activity"
    output_dir: "/var/log/relay"
    file_prefix: "zpa-user-activity"
    dlq:
      enabled: true
      directory: "/var/log/relay-dlq"  # Custom DLQ location
    splunk:
      hec_url: "https://splunk.example.com:8088/services/collector/raw"
      hec_token: "token"
```

### Example: Per-Listener DLQ

Enable DLQ only for critical listeners:

```yaml
listeners:
  - name: "user-activity"
    listen_addr: ":9015"
    log_type: "user-activity"
    output_dir: "/var/log/relay"
    file_prefix: "zpa-user-activity"
    dlq:
      enabled: true  # Critical logs - enable DLQ
    splunk:
      hec_url: "https://splunk.example.com:8088/services/collector/raw"
      hec_token: "token"

  - name: "app-connector-metrics"
    listen_addr: ":9018"
    log_type: "app-connector-metrics"
    output_dir: "/var/log/relay"
    file_prefix: "zpa-metrics"
    # No DLQ - metrics can be lost
    splunk:
      hec_url: "https://splunk.example.com:8088/services/collector/raw"
      hec_token: "token"
```

### DLQ Operations

**Monitoring DLQ**: Check for DLQ files to detect forwarding issues:
```bash
ls -lh /var/log/relay/dlq/
# dlq-2025-11-14.ndjson - presence indicates failures
```

**Analyzing Failures**: Parse DLQ entries to understand failure patterns:
```bash
jq -r '.error' /var/log/relay/dlq/dlq-2025-11-14.ndjson | sort | uniq -c
# Shows error frequency distribution
```

**Replaying DLQ**: Extract original data for replay to HEC:
```bash
jq -r '.data' /var/log/relay/dlq/dlq-2025-11-14.ndjson > failed-logs.ndjson
# Send failed-logs.ndjson to HEC manually or with script
```

**DLQ Retention**: Manage DLQ file retention based on operational needs:
- Keep until successfully replayed
- Retain for compliance/audit requirements
- Delete after investigation complete

**Performance Considerations**:
- DLQ writes are synchronous (slight latency impact on failure path)
- Minimal disk I/O under normal operation (only on failures)
- DLQ files accumulate during extended HEC outages
- Monitor DLQ directory disk usage

## Log Retention Configuration

Configuration for automatic cleanup of old log files to prevent disk space exhaustion.

Log files (both regular logs and DLQ files) can be automatically deleted or compressed based on age. This prevents unbounded disk usage growth over time. Retention policies are **optional and disabled by default**, allowing users to choose between built-in retention or external tools like logrotate.

### Retention Parameters

| Parameter | Type | Required | Default | Reloadable | Description |
|-----------|------|----------|---------|------------|-------------|
| `enabled` | boolean | No | `false` | No | Enable/disable retention policy |
| `max_age_days` | integer | No | `30` | No | Delete files older than N days |
| `check_interval_seconds` | integer | No | `3600` | No | How often to check for old files (in seconds) |
| `compress_age_days` | integer | No | `0` | No | Compress files older than N days (0 = disabled) |

**Scope**: Global configuration applies to all log directories (output directories and DLQ directories).

**File Patterns**: Matches files with pattern `*-YYYY-MM-DD.ndjson` and `*-YYYY-MM-DD.ndjson.gz`.

**Cleanup Behaviour**:
- Files older than `max_age_days` are deleted
- Files older than `compress_age_days` (if enabled) are compressed with gzip before deletion threshold
- Cleanup runs immediately on startup, then periodically based on `check_interval_seconds`
- Compressed size calculation accounts for gzip overhead

**Compression**:
- Original file is deleted after successful compression
- Compressed files have `.gz` extension added
- Already compressed files (.gz) are not recompressed
- Compression savings logged for monitoring

### Example: Disabled Retention (Default)

Leave retention entirely to external tools:

```yaml
# No retention configuration - use logrotate or similar tools
```

### Example: Basic Retention

Delete files older than 30 days, check every hour:

```yaml
retention:
  enabled: true
  max_age_days: 30
  check_interval_seconds: 3600  # 1 hour
```

### Example: Retention with Compression

Compress after 7 days, delete after 30 days:

```yaml
retention:
  enabled: true
  max_age_days: 30
  compress_age_days: 7
  check_interval_seconds: 3600
```

Disk space timeline:
- Days 0-7: Files stored uncompressed
- Days 8-30: Files compressed (typically 70-90% size reduction for log data)
- Day 31+: Files deleted

### Example: Short Retention (Testing)

For development or testing environments:

```yaml
retention:
  enabled: true
  max_age_days: 7
  check_interval_seconds: 1800  # 30 minutes
```

### Example: Long Retention (Compliance)

For compliance or audit requirements:

```yaml
retention:
  enabled: true
  max_age_days: 365  # 1 year
  compress_age_days: 90
  check_interval_seconds: 21600  # 6 hours
```

### Alternative: External Log Rotation

Instead of built-in retention, use system tools like logrotate:

**Example logrotate configuration** (`/etc/logrotate.d/relay`):

```
/var/log/relay/zpa-logs/*.ndjson {
    daily
    rotate 30
    compress
    delaycompress
    notifempty
    missingok
    create 0640 relay relay
    dateext
    dateformat -%Y-%m-%d
}

/var/log/relay/zpa-logs/dlq/*.ndjson {
    daily
    rotate 90
    compress
    delaycompress
    notifempty
    missingok
    create 0640 relay relay
    dateext
    dateformat -%Y-%m-%d
}
```

**Benefits of logrotate**:
- Standard Unix tool with proven reliability
- More flexible rotation policies (size-based, time-based)
- Can execute custom scripts before/after rotation
- Centralised log management across system
- Can handle log files from other applications

**Benefits of built-in retention**:
- Single application to manage
- Unified configuration
- No external dependencies
- Simpler for containerised deployments
- Consistent behaviour across platforms

### Retention Operations

**Monitoring Retention**:

Check logs for retention activity:
```bash
grep "retention cleanup complete" /var/log/relay/relay.log
# Shows files deleted, compressed, and bytes freed
```

**Disk Space Savings**:

Example calculation for 100MB/day logs with 30-day retention and 7-day compression:
- Days 0-7: 700MB uncompressed
- Days 8-30: 23 days × 10MB (compressed at 90% reduction) = 230MB
- **Total: 930MB vs 3000MB uncompressed (69% savings)**

**Performance Impact**:
- Cleanup runs in background goroutine
- Does not block log processing
- File operations use standard I/O (not memory-mapped)
- Minimal CPU usage during cleanup

**Troubleshooting**:

If retention isn't working:
1. Check `retention.enabled: true` in configuration
2. Verify `max_age_days` is set appropriately
3. Check relay logs for retention activity
4. Ensure file patterns match expected format (`zpa-YYYY-MM-DD.ndjson`)
5. Verify relay process has write permissions to log directories

## Configuration Hierarchy

Configuration follows an inheritance hierarchy where per-listener settings override global settings.

### Precedence Rules

1. **Per-Listener Configuration**: Highest priority
2. **Global Configuration**: Default for all listeners
3. **Built-in Defaults**: Used if not specified anywhere

### Inheritance Behaviour

| Parameter | Inheritance | Override Behaviour |
|-----------|-------------|-------------------|
| `hec_url` | Yes | Per-listener completely replaces global |
| `hec_token` | Yes | Per-listener completely replaces global |
| `source_type` | Yes | Per-listener completely replaces global |
| `gzip` | Yes | Per-listener completely replaces global |
| `batch` | Yes | Per-listener values override specific fields only |
| `circuit_breaker` | Yes | Per-listener values override specific fields only |
| `retry` | Yes | Per-listener values override specific fields only |
| `client_timeout_seconds` | Yes | Per-listener completely replaces global |
| `hec_targets` | No | Per-listener completely replaces global |
| `routing` | No | Per-listener completely replaces global |

### Example: Configuration Hierarchy

```yaml
# Global Splunk configuration
splunk:
  hec_url: "https://splunk-global.example.com:8088/services/collector/raw"
  hec_token: "global-token"
  gzip: true
  batch:
    enabled: true
    max_size: 100

listeners:
  # Listener 1: Uses global configuration entirely
  - name: "user-activity"
    listen_addr: ":9015"
    log_type: "user-activity"
    output_dir: "./logs"
    file_prefix: "zpa-user-activity"
    splunk:
      source_type: "zpa:user:activity"  # Only override sourcetype

  # Listener 2: Overrides HEC URL and token
  - name: "audit"
    listen_addr: ":9016"
    log_type: "audit"
    output_dir: "./logs"
    file_prefix: "zpa-audit"
    splunk:
      hec_url: "https://splunk-audit.example.com:8088/services/collector/raw"
      hec_token: "audit-specific-token"
      source_type: "zpa:audit"
      # Still inherits gzip: true and batch config from global
```

## Validation Rules

The relay service performs comprehensive validation at startup and during configuration reload.

### Startup Validation

All parameters are validated when the service starts:

1. **Configuration File**
   - File must exist and be readable
   - Must be valid YAML syntax
   - Must not be empty

2. **Listener Validation**
   - At least one listener required
   - Each listener must have unique `name`
   - Each listener must have unique `listen_addr`
   - `listen_addr` must be available (not in use)
   - `log_type` must be valid (see [valid log types](#valid-log-types))
   - `output_dir` must be writable (created if doesn't exist)
   - `max_line_bytes` must be positive if specified

3. **TLS Validation**
   - Both `cert_file` and `key_file` must be specified together
   - Files must exist and be readable
   - Certificate and key must be valid PEM format
   - Certificate and key must match each other

4. **Splunk HEC Validation**
   - If `hec_url` specified, `hec_token` must also be specified
   - If `hec_token` specified, `hec_url` must also be specified
   - `hec_url` must use `http://` or `https://` scheme
   - `hec_url` must include host
   - Cannot mix single-target and multi-target config in same scope
   - For multi-target: at least one target required
   - For multi-target: each target must have unique `name`
   - For multi-target: routing mode must be valid

5. **ACL Validation**
   - `allowed_cidrs` must be valid CIDR notation if specified
   - Empty string is valid (allows all connections)

### Reload Validation

Additional validation during configuration reload via SIGHUP:

1. **Structural Validation**
   - Listener count must not change
   - Listener names must not change
   - Listener order must remain same

2. **Non-Reloadable Parameter Check**
   - `listen_addr` must not change
   - `log_type` must not change
   - `output_dir` must not change
   - `file_prefix` must not change
   - `max_line_bytes` must not change
   - TLS configuration must not change
   - Batch configuration must not change
   - Circuit breaker configuration must not change
   - Retry configuration must not change
   - Timeout configuration must not change
   - HEC client timeout must not change
   - DLQ configuration must not change

3. **Reloadable Parameter Validation**
   - `allowed_cidrs` must be valid CIDR notation if changed
   - `hec_token` can change freely
   - `source_type` can change freely
   - `gzip` can change freely

**Validation Failure Behaviour**:
- At startup: Service exits with error message
- During reload: Old configuration remains active, error logged

## Configuration Examples

### Example 1: Minimal Configuration

Single listener with local storage only (no HEC forwarding).

```yaml
listeners:
  - name: "user-activity"
    listen_addr: ":9015"
    log_type: "user-activity"
    output_dir: "./zpa-logs"
    file_prefix: "zpa-user-activity"
```

### Example 2: Single Listener with HEC

Global HEC configuration, single listener.

```yaml
splunk:
  hec_url: "https://splunk.example.com:8088/services/collector/raw"
  hec_token: "your-hec-token-here"
  gzip: true

listeners:
  - name: "user-activity"
    listen_addr: ":9015"
    log_type: "user-activity"
    output_dir: "./zpa-logs"
    file_prefix: "zpa-user-activity"
    splunk:
      source_type: "zpa:user:activity"
```

### Example 3: Multiple Listeners with Shared HEC

Multiple log types forwarding to same Splunk instance.

```yaml
splunk:
  hec_url: "https://splunk.example.com:8088/services/collector/raw"
  hec_token: "shared-token"
  gzip: true

listeners:
  - name: "user-activity"
    listen_addr: ":9015"
    log_type: "user-activity"
    output_dir: "./zpa-logs"
    file_prefix: "zpa-user-activity"
    splunk:
      source_type: "zpa:user:activity"

  - name: "user-status"
    listen_addr: ":9016"
    log_type: "user-status"
    output_dir: "./zpa-logs"
    file_prefix: "zpa-user-status"
    splunk:
      source_type: "zpa:user:status"

  - name: "audit"
    listen_addr: ":9017"
    log_type: "audit"
    output_dir: "./zpa-logs"
    file_prefix: "zpa-audit"
    splunk:
      source_type: "zpa:audit"
```

### Example 4: TLS with ACL

Secure listener with access control.

```yaml
listeners:
  - name: "user-activity"
    listen_addr: ":9015"
    log_type: "user-activity"
    output_dir: "/var/log/relay/zpa"
    file_prefix: "zpa-user-activity"
    allowed_cidrs: "10.0.0.0/8, 172.16.0.0/12"
    tls:
      cert_file: "/etc/relay/tls/server.crt"
      key_file: "/etc/relay/tls/server.key"
    splunk:
      hec_url: "https://splunk.example.com:8088/services/collector/raw"
      hec_token: "secure-token"
      source_type: "zpa:user:activity"
      gzip: true
```

### Example 5: High-Throughput Configuration

Optimized for high volume with batching.

```yaml
splunk:
  hec_url: "https://splunk.example.com:8088/services/collector/raw"
  hec_token: "high-volume-token"
  gzip: true
  batch:
    enabled: true
    max_size: 500
    max_bytes: 5242880  # 5 MiB
    flush_interval_seconds: 5
  circuit_breaker:
    enabled: true
    failure_threshold: 10
    timeout_seconds: 60

listeners:
  - name: "user-activity"
    listen_addr: ":9015"
    log_type: "user-activity"
    output_dir: "/var/log/relay/zpa"
    file_prefix: "zpa-user-activity"
    max_line_bytes: 2097152  # 2 MiB
    splunk:
      source_type: "zpa:user:activity"
```

### Example 6: Multi-Target HA Configuration

High availability with primary/failover routing.

```yaml
splunk:
  hec_targets:
    - name: "primary"
      hec_url: "https://splunk-primary.example.com:8088/services/collector/raw"
      hec_token: "primary-token"
      source_type: "zpa:logs"
      gzip: true
    - name: "secondary"
      hec_url: "https://splunk-secondary.example.com:8088/services/collector/raw"
      hec_token: "secondary-token"
      source_type: "zpa:logs"
      gzip: true
  routing:
    mode: "primary-failover"

listeners:
  - name: "user-activity"
    listen_addr: ":9015"
    log_type: "user-activity"
    output_dir: "/var/log/relay/zpa"
    file_prefix: "zpa-user-activity"
```

### Example 7: Per-Listener HEC Targets

Different listeners forwarding to different Splunk instances.

```yaml
listeners:
  # User activity logs go to main Splunk
  - name: "user-activity"
    listen_addr: ":9015"
    log_type: "user-activity"
    output_dir: "/var/log/relay/zpa"
    file_prefix: "zpa-user-activity"
    splunk:
      hec_url: "https://splunk-main.example.com:8088/services/collector/raw"
      hec_token: "main-token"
      source_type: "zpa:user:activity"
      gzip: true

  # Audit logs go to security Splunk
  - name: "audit"
    listen_addr: ":9016"
    log_type: "audit"
    output_dir: "/var/log/relay/zpa"
    file_prefix: "zpa-audit"
    splunk:
      hec_url: "https://splunk-security.example.com:8088/services/collector/raw"
      hec_token: "security-token"
      source_type: "zpa:audit"
      gzip: true
```

## Best Practices

### Security

1. **Protect Secrets**: Set restrictive file permissions on configuration file
   ```bash
   chmod 600 config.yml
   ```

2. **Use TLS**: Enable TLS for production deployments
   ```yaml
   tls:
     cert_file: "/etc/relay/tls/server.crt"
     key_file: "/etc/relay/tls/server.key"
   ```

3. **Restrict Access**: Use ACLs to limit connections
   ```yaml
   allowed_cidrs: "10.0.0.0/8"
   ```

4. **Separate Sensitive Logs**: Use per-listener HEC targets for audit/security logs

### Performance

1. **Enable Batching**: For high-volume environments
   ```yaml
   batch:
     enabled: true
     max_size: 500
     max_bytes: 5242880
   ```

2. **Enable Gzip**: Reduce network bandwidth
   ```yaml
   gzip: true
   ```

3. **Tune Max Line Bytes**: Set appropriate limits based on expected log size
   ```yaml
   max_line_bytes: 2097152  # 2 MiB
   ```

4. **Use Round-Robin**: For load distribution across multiple indexers
   ```yaml
   routing:
     mode: "round-robin"
   ```

### Reliability

1. **Enable Circuit Breaker**: Protect against cascading failures
   ```yaml
   circuit_breaker:
     enabled: true
     failure_threshold: 5
   ```

2. **Configure Health Checks**: Monitor service availability
   ```yaml
   health_check_enabled: true
   health_check_addr: ":9099"
   ```

3. **Tune Retry Behaviour**: Configure retries based on endpoint characteristics
   ```yaml
   retry:
     max_attempts: 5
     initial_backoff_ms: 250
     backoff_multiplier: 2.0
   ```

4. **Configure Connection Timeouts**: Prevent resource exhaustion from hung connections
   ```yaml
   listeners:
     - name: "user-activity"
       timeout:
         read_seconds: 300
         idle_seconds: 600
   splunk:
     client_timeout_seconds: 30
   ```

5. **Enable Dead Letter Queue**: Capture failed forwards for analysis and replay
   ```yaml
   listeners:
     - name: "user-activity"
       dlq:
         enabled: true
   ```

6. **Use Multi-Target HA**: Ensure redundancy
   ```yaml
   routing:
     mode: "primary-failover"
   ```

### Operations

1. **Use Descriptive Names**: Make logs easier to understand
   ```yaml
   listeners:
     - name: "prod-user-activity-datacenter-1"
   ```

2. **Organize Output Directories**: Use clear, hierarchical structure
   ```yaml
   output_dir: "/var/log/relay/production/zpa"
   ```

3. **Use Version Control**: Track configuration changes in git

4. **Test Before Applying**: Validate configuration in staging environment

5. **Document Customizations**: Comment configuration for team understanding

## See Also

- [How to Reload Configuration](../how-to/reload-configuration.md) - Runtime configuration updates
- [How to Set Up TLS](../how-to/setup-tls.md) - TLS configuration guide
- [ADR-0015: Configuration Reload](../explanation/adr/0015-configuration-reload.md) - Design decisions
- [Main README](../../README.md) - Overview and quick start

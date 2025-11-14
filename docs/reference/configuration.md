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

4. **Use Multi-Target HA**: Ensure redundancy
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

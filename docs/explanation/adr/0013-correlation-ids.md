# ADR-0013: Correlation IDs for Request Tracing

## Status

Accepted

## Context

When debugging production issues or analysing system behaviour, it's difficult to trace a single connection's journey through the relay pipeline (accept → validate → store → forward) because log messages from different stages lack a unified identifier linking them together.

Key challenges without correlation IDs:
- Cannot trace a specific connection's complete lifecycle
- Cannot correlate relay logs with Splunk HEC ingestion logs
- Difficult to debug issues for individual connections in high-volume environments
- Cannot measure end-to-end latency for specific requests
- Troubleshooting requires time-consuming log correlation by IP address and timestamp

We need a way to uniquely identify and track each connection throughout its entire lifecycle.

Options considered:
1. No correlation (status quo) - rely on timestamp and client IP correlation
2. Sequential integer IDs - simple but not globally unique
3. Timestamp-based IDs - prone to collisions under load
4. UUID v4 (random) - cryptographically unique, standard format
5. ULID - sortable but adds dependency

## Decision

We will generate a unique correlation ID (UUID v4) for each incoming connection and propagate it through the entire processing pipeline.

Implementation details:
- Generate UUID v4 using `crypto/rand` when connection is accepted
- Include `conn_id` field in all structured log messages (slog)
- Pass correlation ID as parameter through storage and forwarder functions
- Send correlation ID as `X-Correlation-ID` HTTP header when forwarding to Splunk HEC
- Format: `xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx` (standard UUID format)
- Fallback to timestamp-based ID if random generation fails

## Consequences

### Positive

- **End-to-end tracing**: Can trace entire lifecycle of a connection by grepping logs for one ID
- **Splunk correlation**: HEC header enables correlation between relay logs and Splunk ingestion logs
- **Debugging efficiency**: Isolate issues to specific connections without noise from others
- **Performance analysis**: Measure per-connection latency and identify bottlenecks
- **Audit trail**: Clear chain of custody for each log event through the pipeline
- **No dependencies**: Uses only Go standard library (`crypto/rand`)
- **Collision-free**: UUID v4 provides 122 bits of randomness, virtually eliminating collisions
- **Standard format**: UUID is widely recognised and supported by log aggregation tools
- **Structured logging benefit**: Leverages existing slog infrastructure for consistent field inclusion

### Negative

- **Signature change**: Requires updating function signatures for `storage.Write()` and `forwarder.Forward()`
- **Test updates**: All tests must be updated to pass correlation ID parameter
- **Slight overhead**: Small memory allocation per connection (~36 bytes) for UUID string
- **Not sortable**: Unlike ULID, UUIDs don't encode timestamp information
- **Log verbosity**: Adds 47 characters per log line (field name + UUID)

### Neutral

- UUID generation has negligible CPU cost compared to I/O operations
- Fallback mechanism provides reliability if `crypto/rand` fails
- Can switch to ULID in future if sortability becomes requirement
- Compatible with distributed tracing standards (though not implementing full tracing)

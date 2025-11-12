# ADR-0014: Multi-Target HEC Support

## Status

Accepted

## Context

The relay service originally supported forwarding logs to a single Splunk HEC endpoint. However, users have several scenarios requiring multiple HEC targets:

1. **High Availability**: Forward to both primary and backup Splunk instances to prevent data loss during maintenance or outages
2. **Multi-Tenancy**: Send logs to different Splunk instances or indexes based on organisational requirements
3. **Disaster Recovery**: Maintain redundant log copies across geographies for compliance and resilience
4. **Load Distribution**: Distribute logs across multiple Splunk indexers to balance ingestion load

Options considered:

1. **External load balancer**: Deploy HAProxy/nginx in front of multiple HEC endpoints
   - Simple but requires additional infrastructure
   - No intelligent routing logic (all or nothing)
   - Cannot handle per-target configuration differences

2. **Multiple relay instances**: Run separate relay instances for each target
   - Simple implementation
   - Wastes resources (duplicate TCP listeners, storage)
   - Difficult to manage and coordinate

3. **Built-in multi-target support**: Extend relay to support multiple HEC endpoints natively
   - More complex implementation
   - Flexible routing modes
   - Single point of configuration and management

4. **Message queue pattern**: Forward to queue, separate workers forward to targets
   - Requires external dependencies (breaks ADR-0006)
   - Adds operational complexity
   - Better suited for asynchronous processing than streaming relay

## Decision

We implement built-in multi-target HEC support with the following design:

### 1. Forwarder Interface

Create a `Forwarder` interface that both single-target (`HEC`) and multi-target (`MultiHEC`) implementations satisfy:

```go
type Forwarder interface {
    Forward(connID string, data []byte) error
    HealthCheck() error
    Shutdown(ctx context.Context) error
}
```

This allows the server to treat single and multi-target forwarding uniformly.

### 2. Three Routing Modes

Support three distinct routing strategies:

- **All (Broadcast)**: Send to all targets concurrently
  - Use case: HA, DR, multi-tenancy
  - Behaviour: Forwards succeed even if some targets fail

- **Primary-Failover**: Try targets in order, fail over on error
  - Use case: Primary/backup configurations
  - Behaviour: Only uses secondary if primary unavailable

- **Round-Robin**: Distribute logs evenly across targets
  - Use case: Load balancing across indexers
  - Behaviour: Each log goes to exactly one target in rotation

### 3. Configuration Structure

Define targets as an array with per-target configuration:

```yaml
splunk:
  hec_targets:
    - name: primary
      hec_url: "https://splunk1.example.com:8088/services/collector/raw"
      hec_token: "token1"
      source_type: "zpa:logs"
      # Per-target batch, gzip, circuit_breaker config
  routing:
    mode: all  # all, primary-failover, round-robin
```

### 4. Backward Compatibility

Maintain full backward compatibility with existing single-target configuration. The legacy `hec_url` and `hec_token` fields continue to work unchanged. Users must explicitly opt into multi-target configuration.

### 5. Validation Rules

- Cannot mix single-target (`hec_url`/`hec_token`) and multi-target (`hec_targets`) configuration in the same scope
- Each target must have unique name
- All targets validated at startup (fail-fast)
- Health checks verify all targets are reachable

### 6. Per-Target Configuration

Each target supports independent configuration for:
- Gzip compression
- Batch settings (size, bytes, flush interval)
- Circuit breaker settings (thresholds, timeout)

This allows fine-tuning behaviour for different downstream systems.

## Consequences

### Positive

- **Flexibility**: Users can choose appropriate routing mode for their use case
- **Resilience**: Broadcast mode provides built-in redundancy without external infrastructure
- **Efficiency**: Round-robin enables load distribution across multiple indexers
- **Backward compatible**: Existing configurations continue to work unchanged
- **Testability**: Interface-based design makes testing straightforward
- **Observability**: Per-target logging shows which targets succeed/fail
- **Per-target control**: Independent configuration for each target (batch, circuit breaker, gzip)
- **Fail-fast validation**: Startup validation catches configuration errors early
- **No new dependencies**: Uses only standard library (adheres to ADR-0006)
- **Composable**: Can use multi-target at global or per-listener level

### Negative

- **Configuration complexity**: Multi-target configuration is more complex than single target
- **Testing burden**: Three routing modes require comprehensive test coverage
- **Resource usage**: Broadcast mode uses more network bandwidth and CPU
- **Maintenance**: More code to maintain (forwarder interface, MultiHEC implementation)
- **Cognitive load**: Developers must understand routing modes and their trade-offs
- **Error handling complexity**: Aggregating errors from multiple targets is non-trivial
- **Documentation burden**: Must document all routing modes and use cases

### Neutral

- Routing mode defaults to "all" if not specified
- Health checks must pass for all targets (strict validation)
- Each target maintains independent circuit breaker state
- Broadcast mode reports errors but continues forwarding to remaining targets
- Round-robin uses atomic counter for thread-safe distribution
- Primary-failover logs failover events for operational visibility
- Configuration validation prevents mixing single and multi-target approaches
- Interface allows future routing modes (e.g., rule-based, weighted round-robin)

## Related ADRs

- **ADR-0005**: Store First, Forward Second - Multi-target forwarding happens after storage succeeds
- **ADR-0010**: Optional HEC Forwarding - Multi-target forwarding is also optional
- **ADR-0006**: Minimal External Dependencies - Implementation uses only standard library

## Future Considerations

Potential enhancements not included in this decision:

1. **Rule-based routing**: Route based on log content (e.g., error logs to one target, info to another)
2. **Weighted round-robin**: Distribute logs proportionally based on target capacity
3. **Dynamic target management**: Add/remove targets without restart
4. **Per-target metrics**: Prometheus metrics for each target's success/failure rates
5. **Async forwarding**: Queue-based forwarding for additional decoupling

These can be added later without breaking the current design.

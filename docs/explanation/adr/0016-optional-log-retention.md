# ADR-0016: Optional Log Retention with Built-in and External Support

## Status

Accepted

## Context

Relay creates daily log files (NDJSON format) and DLQ files that accumulate indefinitely on disk. Without automatic cleanup, disk space will eventually be exhausted, causing operational failures.

Several forces are at play:

1. **Operational Maturity**: Organizations have varying levels of operational maturity:
   - Some prefer standard Unix tools (logrotate) for centralised log management
   - Others prefer self-contained applications with minimal external dependencies
   - Containerised deployments often lack access to system-level log rotation

2. **Flexibility Requirements**: Different use cases require different retention policies:
   - Development: Short retention (7 days)
   - Production: Medium retention (30-90 days)
   - Compliance: Long retention (1+ year)
   - Compression vs deletion trade-offs

3. **Operational Consistency**: Organizations want consistent behaviour:
   - Some manage all logs via logrotate across all applications
   - Others prefer per-application configuration
   - Container orchestration systems (Kubernetes) prefer self-managing applications

4. **Existing Standards**: logrotate is a proven, well-understood tool with:
   - Flexible policies (time-based, size-based, count-based)
   - Pre/post rotation hooks
   - Compression support
   - Centralised configuration
   - 30+ years of production use

## Decision

Implement **optional built-in log retention** while fully supporting external tools like logrotate:

1. **Disabled by default**: Retention policy is opt-in, not enforced
2. **Global scope**: Single retention policy applies to all log directories (output dirs and DLQ dirs)
3. **Simple configuration**: Four parameters (enabled, max_age_days, check_interval_seconds, compress_age_days)
4. **Non-intrusive**: Runs in background goroutine, doesn't block log processing
5. **Pattern-based**: Matches `*-YYYY-MM-DD.ndjson` format (consistent with daily rotation)
6. **Compression support**: Optional gzip compression before deletion
7. **Documentation parity**: Equal documentation for built-in and logrotate approaches

**Not implemented** (deferred for future consideration):
- Per-listener retention policies
- Custom file prefix matching
- Size-based rotation
- Configuration reloading for retention settings

## Consequences

### Positive

- **User choice**: Organizations can choose the approach that fits their operational model
- **Zero breaking changes**: Existing deployments continue working unchanged
- **Container-friendly**: Self-contained applications don't need external dependencies
- **Simple configuration**: Single YAML block enables retention
- **Proven patterns**: Follows established Unix tool behaviour (similar to logrotate)
- **Observability**: Retention activity logged for monitoring
- **Disk space management**: Prevents unbounded growth without user intervention

### Negative

- **Two approaches to maintain**: Documentation and support for both built-in and external
- **Code complexity**: Additional package and background worker
- **Testing burden**: Must test retention logic separately from core functionality
- **Migration consideration**: Users switching between approaches must plan transition
- **Non-reloadable**: Changing retention settings requires restart (acceptable for infrequent changes)

### Neutral

- **Global scope only**: Per-listener retention deferred to future (YAGNI until requested)
- **Pattern matching limitations**: Custom prefixes not supported initially (matches 99% of use cases)
- **Compression trade-offs**: Gzip compression effective for log data but adds CPU overhead
- **External tool integration**: logrotate works seamlessly without application awareness

## Implementation Notes

1. **File pattern matching**: Uses glob patterns `*-YYYY-MM-DD.ndjson` and `*-YYYY-MM-DD.ndjson.gz`
2. **Date extraction**: Parses date from filename suffix (last 3 dash-separated components)
3. **Background worker**: Runs in goroutine with context cancellation for clean shutdown
4. **Immediate cleanup**: Runs once on startup, then periodically based on check_interval
5. **Compression handling**: Already compressed files (.gz) not recompressed
6. **Error handling**: Logs errors but continues processing other files

## Alternatives Considered

### 1. Built-in only (no external tool support)

**Rejected**: Forces all users into single approach, ignores existing operational practices

### 2. External only (no built-in support)

**Rejected**: Creates dependency on system tools, complicates container deployments

### 3. Mandatory retention with default policy

**Rejected**: Breaking change for existing deployments, removes user choice

### 4. Per-listener retention policies

**Deferred**: YAGNI - no clear use case presented yet. Global policy handles common scenarios. Can add later if requested.

## Related Decisions

- [ADR-0002: Daily UTC Rotation](0002-daily-utc-rotation.md) - Defines file naming pattern that retention depends on
- [ADR-0004: NDJSON Storage](0004-ndjson-storage.md) - File format that retention manages

## References

- Issue #22: Implement log retention policies
- Configuration Reference: Log Retention Configuration
- How-To: Manage Log Retention

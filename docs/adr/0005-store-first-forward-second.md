# ADR-0005: Store First, Forward Second

## Status

Accepted

## Context

When relay receives log data, it needs to both store it locally and forward it to Splunk HEC. The order of these operations affects data durability and reliability.

Options considered:
1. **Store first, then forward**: Write to disk, then attempt HEC forward
2. **Forward first, then store**: Send to HEC, then write to disk if successful
3. **Concurrent**: Store and forward simultaneously
4. **Forward only**: Send to HEC without local storage

## Decision

We will always write log data to local storage first, then attempt to forward to HEC second. Local storage operations are synchronous and blocking; HEC forwarding happens after storage succeeds.

## Consequences

### Positive

- **Zero data loss**: Data is durable on disk even if HEC is down or network fails
- **Replay capability**: Can always replay from local files if HEC ingestion fails
- **Local storage is reliable**: Disk writes are more reliable than network operations
- **Audit trail**: Complete local record of all received data
- **Debugging**: Can inspect local files to troubleshoot issues
- **Independence from HEC**: Service remains operational even when HEC is unavailable

### Negative

- **Increased latency**: Must wait for disk write before forwarding
- **Disk I/O required**: Can't operate in forward-only mode even if local storage isn't needed
- **Storage requirements**: Must provision disk space even if HEC is the primary destination

### Neutral

- HEC forwarding failures don't affect local storage
- Can add dead letter queue for failed forwards later
- Storage is fast enough that latency impact is minimal (milliseconds)
- Aligns with "durability first" philosophy for log relay services

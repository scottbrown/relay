# ADR-0011: UTC Timestamps Everywhere

## Status

Accepted

## Context

The relay service handles log data with timestamps and generates its own timestamps for various operations (file rotation, logging, metrics). We need a consistent approach to timezone handling across the application.

Options considered:
1. **Local time**: Use server's local timezone
2. **UTC everywhere**: All timestamps in UTC
3. **Configurable**: Let users choose timezone
4. **Mixed**: UTC internal, local time for display

## Decision

All timestamps used throughout the application will be in UTC. This includes:
- File rotation timestamps (as per ADR-0002)
- Log file names (`zpa-2025-01-15.ndjson`)
- Internal logging timestamps
- Any generated timestamps in log data
- Metrics timestamps

## Consequences

### Positive

- **No timezone confusion**: Eliminates ambiguity, especially during DST transitions
- **Distributed systems standard**: UTC is the standard for distributed systems
- **Consistent across deployments**: Works the same regardless of server timezone
- **Sortable**: ISO 8601 UTC timestamps sort chronologically
- **Splunk compatible**: Splunk handles UTC and converts to display timezone
- **Avoids DST issues**: No ambiguous hours during DST spring forward/fall back
- **International**: Works well for globally distributed teams

### Negative

- **Not human-friendly**: Operators may need to convert UTC to local time mentally
- **Log correlation**: May be harder to correlate with local system logs
- **Business hour alignment**: File boundaries don't align with local business days

### Neutral

- Most monitoring and log aggregation tools expect and handle UTC well
- Operators working with distributed systems are typically familiar with UTC
- Can display in local time in dashboards while storing in UTC
- Aligns with RFC 3339 and ISO 8601 standards

# ADR-0002: Daily Log Rotation Based on UTC

## Status

Accepted

## Context

The relay service stores incoming logs to local files for durability and replay capability. We need a file rotation strategy that:
- Prevents files from growing indefinitely
- Makes it easy to identify and manage log files
- Works consistently across different deployment environments
- Avoids timezone-related confusion

Options considered:
1. Size-based rotation (rotate when file reaches X MB)
2. Hourly rotation
3. Daily rotation (local time)
4. Daily rotation (UTC)

## Decision

We will rotate log files daily at midnight UTC, with files named `zpa-YYYY-MM-DD.ndjson` where the date is in UTC.

## Consequences

### Positive

- **Predictable file names**: Files are consistently named with ISO 8601 dates
- **No timezone confusion**: UTC avoids ambiguity during daylight saving time changes
- **Easy chronological sorting**: File names sort naturally by date
- **Manageable file sizes**: Daily rotation balances file size (typically GB range) with file count
- **Standard for distributed systems**: UTC is the standard for log timestamps in distributed systems
- **Splunk compatibility**: Splunk can handle timezone conversion on ingestion

### Negative

- **May not align with business hours**: File boundaries don't align with local business day
- **Date confusion for operators**: Operators in different timezones need to convert to UTC mentally
- **Variable file sizes**: High and low volume times of day will create uneven file sizes

### Neutral

- Retention policies work on daily boundaries
- One day's worth of logs is a reasonable unit for replay/recovery operations

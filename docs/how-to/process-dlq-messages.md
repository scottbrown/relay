# How To: Process Dead Letter Queue Messages

This guide shows you how to monitor, analyze, and replay failed HEC forwards from the Dead Letter Queue (DLQ).

## Prerequisites

- DLQ enabled in relay configuration
- `jq` installed for JSON parsing
- Access to Splunk HEC endpoint for replay

## Understanding DLQ Files

DLQ files are created when HEC forwards fail after all retries are exhausted:
- **Location**: `{output_dir}/dlq/` (or custom directory if configured)
- **Format**: `dlq-YYYY-MM-DD.ndjson` (one file per day)
- **Content**: NDJSON with metadata and original log data

Each DLQ entry contains:
```json
{
  "timestamp": "2025-11-14T15:30:00Z",
  "conn_id": "connection-id",
  "error": "hec send failed after retries",
  "data": "{\"original\":\"log line\"}"
}
```

## Monitoring DLQ

### Check for DLQ Files

Presence of DLQ files indicates forwarding issues:

```bash
# List DLQ files
ls -lh /var/log/relay/dlq/

# Check if DLQ has entries today
if [ -f "/var/log/relay/dlq/dlq-$(date +%Y-%m-%d).ndjson" ]; then
    echo "WARNING: DLQ has entries today"
fi
```

### Count DLQ Entries

```bash
# Count entries in today's DLQ
wc -l /var/log/relay/dlq/dlq-$(date +%Y-%m-%d).ndjson

# Count entries across all DLQ files
wc -l /var/log/relay/dlq/dlq-*.ndjson
```

### Set Up Alerts

Create a monitoring script:

```bash
#!/bin/bash
# monitor-dlq.sh

DLQ_DIR="/var/log/relay/dlq"
ALERT_THRESHOLD=100

TODAY_DLQ="${DLQ_DIR}/dlq-$(date +%Y-%m-%d).ndjson"

if [ -f "$TODAY_DLQ" ]; then
    COUNT=$(wc -l < "$TODAY_DLQ")

    if [ "$COUNT" -gt "$ALERT_THRESHOLD" ]; then
        echo "ALERT: DLQ has $COUNT entries (threshold: $ALERT_THRESHOLD)"
        # Send alert via email, Slack, PagerDuty, etc.
        # mail -s "Relay DLQ Alert" ops@example.com <<< "DLQ has $COUNT failed forwards"
    fi
fi
```

Add to cron:
```bash
# Check DLQ every 5 minutes
*/5 * * * * /path/to/monitor-dlq.sh
```

## Analyzing DLQ Failures

### View Recent Failures

```bash
# Show last 10 DLQ entries
tail -10 /var/log/relay/dlq/dlq-$(date +%Y-%m-%d).ndjson | jq .
```

### Analyze Error Patterns

```bash
# Count errors by type
jq -r '.error' /var/log/relay/dlq/dlq-*.ndjson | sort | uniq -c | sort -rn

# Example output:
#  1247 hec send failed after retries
#   432 circuit breaker open
#    89 Post "https://splunk.example.com:8088": dial tcp: lookup splunk.example.com: no such host
```

### Find Errors by Time

```bash
# Find all errors in a specific hour
jq -r 'select(.timestamp | startswith("2025-11-14T15")) | .error' \
    /var/log/relay/dlq/dlq-2025-11-14.ndjson | sort | uniq -c
```

### Check Specific Connection

```bash
# Find all failures for a specific connection
jq -r 'select(.conn_id == "abc-123") | .' \
    /var/log/relay/dlq/dlq-*.ndjson
```

## Replaying DLQ Messages

### Manual Replay to HEC

Extract and replay failed messages to Splunk HEC:

```bash
#!/bin/bash
# replay-dlq.sh

DLQ_FILE="$1"
HEC_URL="https://splunk.example.com:8088/services/collector/raw"
HEC_TOKEN="your-hec-token"
SOURCETYPE="zpa:user:activity"

if [ ! -f "$DLQ_FILE" ]; then
    echo "Error: DLQ file not found: $DLQ_FILE"
    exit 1
fi

# Extract original data from DLQ entries
jq -r '.data' "$DLQ_FILE" > /tmp/replay-data.ndjson

# Send to HEC
curl -k \
    -H "Authorization: Splunk $HEC_TOKEN" \
    -H "Content-Type: text/plain" \
    --data-binary @/tmp/replay-data.ndjson \
    "${HEC_URL}?sourcetype=${SOURCETYPE}"

echo "Replayed $(wc -l < /tmp/replay-data.ndjson) entries from $DLQ_FILE"
rm /tmp/replay-data.ndjson
```

Usage:
```bash
./replay-dlq.sh /var/log/relay/dlq/dlq-2025-11-14.ndjson
```

### Batch Replay with Rate Limiting

For large DLQ files, replay in batches to avoid overwhelming Splunk:

```bash
#!/bin/bash
# replay-dlq-batched.sh

DLQ_FILE="$1"
HEC_URL="https://splunk.example.com:8088/services/collector/raw"
HEC_TOKEN="your-hec-token"
SOURCETYPE="zpa:user:activity"
BATCH_SIZE=100
DELAY_SECONDS=1

TEMP_DIR=$(mktemp -d)
trap "rm -rf $TEMP_DIR" EXIT

# Extract original data
jq -r '.data' "$DLQ_FILE" > "$TEMP_DIR/all-data.ndjson"

TOTAL_LINES=$(wc -l < "$TEMP_DIR/all-data.ndjson")
echo "Replaying $TOTAL_LINES entries in batches of $BATCH_SIZE"

# Split into batches
split -l "$BATCH_SIZE" "$TEMP_DIR/all-data.ndjson" "$TEMP_DIR/batch-"

# Replay each batch
BATCH_NUM=0
for BATCH_FILE in "$TEMP_DIR"/batch-*; do
    BATCH_NUM=$((BATCH_NUM + 1))
    BATCH_LINES=$(wc -l < "$BATCH_FILE")

    echo "Replaying batch $BATCH_NUM ($BATCH_LINES entries)..."

    curl -k -s \
        -H "Authorization: Splunk $HEC_TOKEN" \
        -H "Content-Type: text/plain" \
        --data-binary @"$BATCH_FILE" \
        "${HEC_URL}?sourcetype=${SOURCETYPE}"

    echo " done"
    sleep "$DELAY_SECONDS"
done

echo "Replay complete: $TOTAL_LINES entries replayed"
```

Usage:
```bash
./replay-dlq-batched.sh /var/log/relay/dlq/dlq-2025-11-14.ndjson
```

### Selective Replay

Replay only specific entries based on criteria:

```bash
#!/bin/bash
# replay-dlq-selective.sh

DLQ_FILE="$1"
ERROR_FILTER="$2"  # e.g., "circuit breaker"
HEC_URL="https://splunk.example.com:8088/services/collector/raw"
HEC_TOKEN="your-hec-token"
SOURCETYPE="zpa:user:activity"

# Extract data for entries matching error filter
jq -r "select(.error | contains(\"$ERROR_FILTER\")) | .data" "$DLQ_FILE" \
    > /tmp/filtered-replay.ndjson

COUNT=$(wc -l < /tmp/filtered-replay.ndjson)

if [ "$COUNT" -eq 0 ]; then
    echo "No entries match filter: $ERROR_FILTER"
    rm /tmp/filtered-replay.ndjson
    exit 0
fi

echo "Replaying $COUNT entries matching '$ERROR_FILTER'"

curl -k \
    -H "Authorization: Splunk $HEC_TOKEN" \
    -H "Content-Type: text/plain" \
    --data-binary @/tmp/filtered-replay.ndjson \
    "${HEC_URL}?sourcetype=${SOURCETYPE}"

echo "Replay complete"
rm /tmp/filtered-replay.ndjson
```

Usage:
```bash
# Replay only circuit breaker failures
./replay-dlq-selective.sh /var/log/relay/dlq/dlq-2025-11-14.ndjson "circuit breaker"
```

## DLQ Maintenance

### Archive Processed DLQ Files

After successfully replaying, archive DLQ files:

```bash
#!/bin/bash
# archive-dlq.sh

DLQ_DIR="/var/log/relay/dlq"
ARCHIVE_DIR="/var/log/relay/dlq-archive"

mkdir -p "$ARCHIVE_DIR"

# Archive DLQ files older than 7 days
find "$DLQ_DIR" -name "dlq-*.ndjson" -mtime +7 -exec mv {} "$ARCHIVE_DIR/" \;

echo "Archived DLQ files older than 7 days"
```

### Clean Up Old DLQ Files

Delete DLQ files after successful replay and retention period:

```bash
#!/bin/bash
# cleanup-dlq.sh

ARCHIVE_DIR="/var/log/relay/dlq-archive"
RETENTION_DAYS=30

# Delete archived DLQ files older than retention period
find "$ARCHIVE_DIR" -name "dlq-*.ndjson" -mtime +$RETENTION_DAYS -delete

echo "Deleted DLQ files older than $RETENTION_DAYS days"
```

### Automated DLQ Processing

Set up automated replay and cleanup:

```bash
# Add to cron
# Process DLQ daily at 2 AM
0 2 * * * /path/to/replay-dlq-batched.sh /var/log/relay/dlq/dlq-$(date -d yesterday +\%Y-\%m-\%d).ndjson && \
          /path/to/archive-dlq.sh && \
          /path/to/cleanup-dlq.sh
```

## Troubleshooting

### DLQ Files Keep Growing

**Symptom**: DLQ accumulates many entries continuously

**Diagnosis**:
```bash
# Check error patterns
jq -r '.error' /var/log/relay/dlq/dlq-$(date +%Y-%m-%d).ndjson | sort | uniq -c
```

**Common Causes**:
1. **HEC endpoint unreachable**: Check network connectivity
   ```bash
   curl -k https://splunk.example.com:8088/services/collector/health
   ```

2. **Invalid HEC token**: Verify token in configuration
   ```bash
   # Test HEC token
   curl -k -H "Authorization: Splunk your-token" \
        https://splunk.example.com:8088/services/collector/raw \
        -d '{"test":"data"}'
   ```

3. **Circuit breaker open**: Check relay logs for circuit breaker state
   ```bash
   grep "circuit breaker" /var/log/relay/relay.log
   ```

4. **HEC endpoint overloaded**: Reduce batch size or increase timeouts

### Replay Fails

**Symptom**: Replay script returns errors

**Diagnosis**:
```bash
# Test HEC connectivity
curl -v -k \
    -H "Authorization: Splunk $HEC_TOKEN" \
    -H "Content-Type: text/plain" \
    -d '{"test":"data"}' \
    "$HEC_URL"
```

**Solutions**:
- Verify HEC URL and token are correct
- Check Splunk HEC is accepting data
- Ensure sourcetype exists in Splunk
- Check replay script has correct permissions

### DLQ Not Creating Files

**Symptom**: No DLQ files despite HEC failures

**Diagnosis**:
```bash
# Check if DLQ is enabled
grep -A 5 "dlq:" /path/to/relay-config.yml

# Check relay logs
grep -i "dlq" /var/log/relay/relay.log
```

**Solutions**:
- Verify `dlq.enabled: true` in configuration
- Check DLQ directory exists and is writable
- Verify relay was restarted after config change (DLQ not reloadable)

## Best Practices

1. **Monitor DLQ daily**: Set up automated alerts for DLQ entries
2. **Investigate root causes**: Don't just replay - fix underlying issues
3. **Replay promptly**: Process DLQ within 24-48 hours to maintain data freshness
4. **Verify replays**: Check Splunk to confirm data arrived after replay
5. **Archive, don't delete**: Keep processed DLQ files for audit trail
6. **Test replay scripts**: Verify replay works before you need it in production
7. **Document procedures**: Maintain runbook for DLQ operations

## See Also

- [Configuration Reference: Dead Letter Queue](../reference/configuration.md#dead-letter-queue-configuration)
- [Configuration Reload](reload-configuration.md)

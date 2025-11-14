# How To: Manage Log Retention

This guide shows you how to manage log file retention to prevent disk space exhaustion.

## Prerequisites

- Relay configured and running
- Understanding of your retention requirements (compliance, disk capacity, etc.)

## Choosing a Retention Approach

Relay supports **two approaches** for log retention:

1. **Built-in retention**: Relay manages cleanup automatically
2. **External tools** (logrotate): System-level log management

### When to Use Built-in Retention

Choose built-in retention when:
- Running in containers (Docker, Kubernetes)
- Prefer self-contained applications
- Simple retention needs (age-based deletion/compression)
- No existing log management infrastructure
- Want unified configuration in single file

### When to Use External Tools

Choose external tools (logrotate) when:
- Managing logs centrally across multiple applications
- Need advanced policies (size-based, pre/post hooks)
- Existing logrotate infrastructure
- Prefer standard Unix tools
- Need flexibility beyond built-in capabilities

## Option 1: Built-in Retention

### Basic Setup

Enable retention with default settings (30 days, hourly checks):

```yaml
# config.yml
retention:
  enabled: true
  max_age_days: 30
  check_interval_seconds: 3600
```

Restart relay:
```bash
sudo systemctl restart relay
# or
./relay --config config.yml
```

### Common Configurations

#### Development/Testing (7 days)

```yaml
retention:
  enabled: true
  max_age_days: 7
  check_interval_seconds: 1800  # Check every 30 minutes
```

#### Production (30 days with compression)

```yaml
retention:
  enabled: true
  max_age_days: 30
  compress_age_days: 7  # Compress after 7 days
  check_interval_seconds: 3600
```

**Disk space timeline**:
- Days 0-7: Uncompressed files
- Days 8-30: Compressed files (70-90% size reduction)
- Day 31+: Deleted

#### Compliance (1 year)

```yaml
retention:
  enabled: true
  max_age_days: 365
  compress_age_days: 90  # Compress after 90 days
  check_interval_seconds: 21600  # Check every 6 hours
```

### Monitoring Built-in Retention

**Check retention activity in logs**:

```bash
# View retention cleanup events
grep "retention cleanup complete" /var/log/relay/relay.log

# Example output:
# {"level":"info","msg":"retention cleanup complete","files_deleted":5,"files_compressed":12,"bytes_freed":524288000}
```

**Verify files are being deleted**:

```bash
# List log files with ages
find /var/log/relay -name "*.ndjson*" -mtime +30 -ls

# Count files by age
find /var/log/relay -name "*.ndjson" -mtime -7 | wc -l   # Last 7 days
find /var/log/relay -name "*.ndjson" -mtime +30 | wc -l  # Older than 30 days
```

**Monitor disk space**:

```bash
# Current disk usage
du -sh /var/log/relay

# Disk usage by age
du -sh /var/log/relay/*.ndjson | head -10
du -sh /var/log/relay/*.ndjson.gz | head -10
```

### Troubleshooting Built-in Retention

#### Files not being deleted

**Check configuration**:
```bash
# Verify retention is enabled
grep -A 5 "retention:" /path/to/config.yml
```

**Check logs for errors**:
```bash
grep -i "retention" /var/log/relay/relay.log
```

**Common issues**:
1. `retention.enabled: false` - Retention disabled
2. `max_age_days` too high - Files not old enough yet
3. File permissions - Relay process can't delete files
4. Wrong file pattern - Files don't match `*-YYYY-MM-DD.ndjson`

**Verify file patterns**:
```bash
# Files should match this pattern
ls /var/log/relay/zpa-2025-*.ndjson
ls /var/log/relay/dlq/dlq-2025-*.ndjson
```

#### Compression not working

**Check compress_age_days setting**:
```yaml
retention:
  compress_age_days: 7  # Must be > 0 and < max_age_days
```

**Check for compressed files**:
```bash
ls -lh /var/log/relay/*.ndjson.gz
```

**Verify compression effectiveness**:
```bash
# Compare original vs compressed
ls -lh /var/log/relay/zpa-2025-11-07.ndjson
ls -lh /var/log/relay/zpa-2025-11-07.ndjson.gz
```

## Option 2: External Tools (logrotate)

### Basic Setup

**Create logrotate configuration** (`/etc/logrotate.d/relay`):

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

**Test configuration**:
```bash
# Dry run to verify configuration
sudo logrotate -d /etc/logrotate.d/relay

# Force rotation to test
sudo logrotate -f /etc/logrotate.d/relay
```

### Advanced logrotate Configurations

#### Size-based rotation

```
/var/log/relay/zpa-logs/*.ndjson {
    size 100M
    rotate 30
    compress
    delaycompress
    notifempty
    missingok
    create 0640 relay relay
}
```

#### Post-rotation hook (upload to S3)

```
/var/log/relay/zpa-logs/*.ndjson {
    daily
    rotate 30
    compress
    notifempty
    missingok
    create 0640 relay relay
    postrotate
        /usr/local/bin/upload-to-s3.sh /var/log/relay/zpa-logs/*.gz
    endscript
}
```

#### Per-log-type retention

```
# Short retention for metrics
/var/log/relay/zpa-logs/zpa-pse-metrics-*.ndjson {
    daily
    rotate 7
    compress
    notifempty
    missingok
}

# Long retention for audit logs
/var/log/relay/zpa-logs/zpa-audit-*.ndjson {
    daily
    rotate 365
    compress
    notifempty
    missingok
}
```

### Monitoring logrotate

**Check logrotate status**:
```bash
# View logrotate logs
cat /var/log/logrotate.log

# Check for errors
grep -i error /var/log/logrotate.log
```

**Verify rotation is working**:
```bash
# Check for rotated files
ls -lth /var/log/relay/zpa-logs/*.ndjson*

# Count rotated files
ls /var/log/relay/zpa-logs/*.ndjson.gz | wc -l
```

**Monitor last rotation time**:
```bash
# Check rotation timestamps
stat /var/log/relay/zpa-logs/*.ndjson.1.gz
```

### Troubleshooting logrotate

#### logrotate not running

**Check cron**:
```bash
# Verify logrotate cron job exists
cat /etc/cron.daily/logrotate
```

**Run manually**:
```bash
sudo /usr/sbin/logrotate /etc/logrotate.conf
```

#### Files not rotating

**Check configuration syntax**:
```bash
sudo logrotate -d /etc/logrotate.d/relay
```

**Common issues**:
1. Wrong file path
2. Missing permissions (relay user can't read/write)
3. File pattern doesn't match
4. State file issues

**Check state file**:
```bash
cat /var/lib/logrotate/status | grep relay
```

## Hybrid Approach

You can use **both** approaches for different purposes:

```yaml
# config.yml - Built-in retention for DLQ only
retention:
  enabled: true
  max_age_days: 90
  compress_age_days: 30
```

```
# /etc/logrotate.d/relay - External rotation for main logs
/var/log/relay/zpa-logs/*.ndjson {
    daily
    rotate 30
    compress
    notifempty
    missingok
}
```

**When to use hybrid**:
- Different retention policies for different log types
- DLQ requires special handling
- Transition period between approaches

## Migrating Between Approaches

### From External to Built-in

1. **Document current policy**:
   ```bash
   cat /etc/logrotate.d/relay
   ```

2. **Disable logrotate**:
   ```bash
   sudo rm /etc/logrotate.d/relay
   ```

3. **Enable built-in retention**:
   ```yaml
   retention:
     enabled: true
     max_age_days: 30  # Match previous logrotate policy
     compress_age_days: 7
   ```

4. **Restart relay**:
   ```bash
   sudo systemctl restart relay
   ```

5. **Monitor for 24 hours** to verify cleanup works

### From Built-in to External

1. **Configure logrotate** (see above)

2. **Test logrotate**:
   ```bash
   sudo logrotate -f /etc/logrotate.d/relay
   ```

3. **Disable built-in retention**:
   ```yaml
   retention:
     enabled: false
   ```

4. **Restart relay**:
   ```bash
   sudo systemctl restart relay
   ```

5. **Clean up manually rotated files** if needed

## Best Practices

1. **Test before production**: Verify retention works in dev/staging first
2. **Monitor disk space**: Set up alerts for disk usage
3. **Document retention policy**: Keep records for compliance
4. **Regular verification**: Periodically check retention is working
5. **Plan for growth**: Account for increasing log volumes over time
6. **Consider compression**: Balance disk space vs CPU usage
7. **Backup before deletion**: For compliance-critical logs
8. **Coordinate with backups**: Ensure backups complete before deletion

## Disk Space Planning

### Calculation Formula

```
Daily volume × Uncompressed days + (Daily volume × Compression ratio × Compressed days)
```

### Example Calculation

**Scenario**:
- 100 MB/day log volume
- 30-day retention
- Compress after 7 days
- 90% compression ratio (typical for logs)

**Calculation**:
```
Days 0-7:   100 MB × 7 = 700 MB (uncompressed)
Days 8-30:  10 MB × 23 = 230 MB (compressed)
Total:      930 MB
```

**Without compression**:
```
100 MB × 30 = 3000 MB
```

**Savings**: 69% with compression

### Monitoring Script

Create `/usr/local/bin/relay-disk-check.sh`:

```bash
#!/bin/bash
WARN_THRESHOLD=80
CRIT_THRESHOLD=90
LOG_DIR="/var/log/relay"

USAGE=$(df -h "$LOG_DIR" | awk 'NR==2 {print $5}' | sed 's/%//')

if [ "$USAGE" -ge "$CRIT_THRESHOLD" ]; then
    echo "CRITICAL: Relay log directory at ${USAGE}% capacity"
    exit 2
elif [ "$USAGE" -ge "$WARN_THRESHOLD" ]; then
    echo "WARNING: Relay log directory at ${USAGE}% capacity"
    exit 1
else
    echo "OK: Relay log directory at ${USAGE}% capacity"
    exit 0
fi
```

Add to monitoring (Nagios, Prometheus, etc.):
```bash
*/15 * * * * /usr/local/bin/relay-disk-check.sh
```

## See Also

- [Configuration Reference: Log Retention Configuration](../reference/configuration.md#log-retention-configuration)
- [ADR-0016: Optional Log Retention](../explanation/adr/0016-optional-log-retention.md)
- [How to Process DLQ Messages](process-dlq-messages.md)

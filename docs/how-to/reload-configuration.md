# How to Reload Configuration Without Restarting Relay

This guide walks you through reloading runtime configuration for the relay service using the SIGHUP signal, allowing you to update certain parameters without interrupting active connections or restarting the service.

## Why Reload Configuration?

Configuration reload via SIGHUP enables operational changes without service interruption:

- **Update HEC tokens** after rotation without dropping connections
- **Modify access control lists** to add or remove allowed IP ranges
- **Change Splunk source types** without interrupting log collection
- **Enable/disable gzip compression** for HEC forwarding
- **Zero downtime** for configuration changes that don't require listener restart

Without this feature, updating these parameters would require a full service restart, potentially causing:
- Loss of in-flight log data
- Connection interruptions from ZPA LSS
- Gaps in log collection during restart

## Prerequisites

- Relay service version that supports SIGHUP (check with `relay --version`)
- Access to the configuration file
- Permission to send signals to the relay process (usually requires same user or root)
- Understanding of which parameters can/cannot be reloaded (see below)

## What Can Be Reloaded

### Reloadable Parameters ✅

These parameters can be changed and reloaded without restarting the service:

| Parameter | Config Key | Scope | Example Use Case |
|-----------|------------|-------|------------------|
| HEC Token | `hec_token` | Per-listener or global | Token rotation for security |
| HEC Sourcetype | `source_type` | Per-listener | Change log categorisation |
| HEC Gzip | `gzip` | Per-listener or global | Optimise network usage |
| ACL CIDRs | `allowed_cidrs` | Per-listener | Add/remove allowed networks |

### Non-Reloadable Parameters ❌

These parameters require a full service restart to change:

| Parameter | Config Key | Why Restart Required |
|-----------|------------|---------------------|
| Listen Address | `listen_addr` | Requires new TCP listener |
| TLS Certificate | `tls.cert_file` | Certificate loaded at startup |
| TLS Key | `tls.key_file` | Private key loaded at startup |
| Output Directory | `output_dir` | May break in-flight writes |
| File Prefix | `file_prefix` | Affects filename generation |
| Log Type | `log_type` | Fundamental listener identity |
| Max Line Bytes | `max_line_bytes` | Affects active connections |
| Batch Config | `batch.*` | Requires forwarder recreation |
| Circuit Breaker | `circuit_breaker.*` | Requires state machine restart |
| Listener Count | Number of listeners | Cannot add/remove listeners |

## Step-by-Step Reload Process

### Step 1: Identify the Relay Process

Find the relay process ID (PID):

```bash
# Using ps
ps aux | grep relay

# Using pgrep
pgrep relay

# Using systemctl (if running as a service)
systemctl status relay
```

Example output:
```
user    12345  0.1  0.2  1234567  45678 ?  Ssl  10:30  0:05 /usr/local/bin/relay --config /etc/relay/config.yml
```

The PID in this example is `12345`.

### Step 2: Edit the Configuration File

Make your desired changes to the configuration file:

```bash
# Edit with your preferred editor
vim /etc/relay/config.yml

# Or
nano /etc/relay/config.yml
```

**Example: Update HEC token**

```yaml
listeners:
  - name: "user-activity"
    listen_addr: ":9015"
    log_type: "user-activity"
    output_dir: "./zpa-logs"
    file_prefix: "zpa-user-activity"
    splunk:
      hec_token: "new-token-after-rotation-abc123"  # Changed
      source_type: "zpa:user:activity"
```

**Example: Update ACL to allow new network**

```yaml
listeners:
  - name: "user-activity"
    listen_addr: ":9015"
    log_type: "user-activity"
    output_dir: "./zpa-logs"
    file_prefix: "zpa-user-activity"
    allowed_cidrs: "10.0.0.0/8, 172.16.0.0/12"  # Added 172.16.0.0/12
    splunk:
      source_type: "zpa:user:activity"
```

### Step 3: Send SIGHUP Signal

Choose one of the following methods to send the SIGHUP signal:

#### Method 1: Using kill with PID

```bash
kill -HUP 12345
```

#### Method 2: Using pkill with process name

```bash
# Send to all processes named 'relay'
pkill -HUP relay

# More specific match
pkill -HUP -f "relay --config"
```

#### Method 3: Using systemctl (systemd service)

```bash
# If relay is configured as a systemd service with reload support
systemctl reload relay
```

Note: This requires your systemd service file to have `ExecReload=/bin/kill -HUP $MAINPID` configured.

### Step 4: Verify the Reload

Check the relay logs to confirm successful reload:

```bash
# If using journald
journalctl -u relay -f

# If logging to a file
tail -f /var/log/relay/relay.log

# If running in foreground
# Check the terminal output
```

**Successful reload log messages:**

```json
{"time":"2025-11-14T10:30:00.000Z","level":"INFO","msg":"received SIGHUP, reloading configuration"}
{"time":"2025-11-14T10:30:00.001Z","level":"INFO","msg":"ACL configuration updated","cidrs":"10.0.0.0/8, 172.16.0.0/12"}
{"time":"2025-11-14T10:30:00.001Z","level":"INFO","msg":"HEC configuration updated","sourcetype":"zpa:user:activity","gzip":true}
{"time":"2025-11-14T10:30:00.001Z","level":"INFO","msg":"reloaded configuration for listener","listener":"user-activity"}
{"time":"2025-11-14T10:30:00.002Z","level":"INFO","msg":"configuration reloaded successfully"}
```

## Common Reload Scenarios

### Scenario 1: HEC Token Rotation

**Situation**: Your security policy requires rotating HEC tokens every 90 days.

**Steps:**

1. Generate new HEC token in Splunk
2. Update `hec_token` in config.yml
3. Send SIGHUP to reload
4. Verify successful reload in logs
5. Test that new logs reach Splunk
6. Revoke old token in Splunk after verification period

```bash
# Edit config
sed -i 's/old-token-abc123/new-token-def456/g' /etc/relay/config.yml

# Reload
pkill -HUP relay

# Verify
journalctl -u relay -n 20
```

### Scenario 2: Emergency ACL Update

**Situation**: You need to immediately block a suspicious IP range.

**Steps:**

1. Update `allowed_cidrs` to remove the problematic range
2. Send SIGHUP to reload immediately
3. Monitor logs to verify blocked connections

```bash
# Edit config to remove 192.168.1.0/24 from allowed_cidrs
vim /etc/relay/config.yml

# Reload immediately
pkill -HUP relay

# Monitor for denied connections
journalctl -u relay -f | grep "denied by ACL"
```

### Scenario 3: Enable Gzip Compression

**Situation**: You want to reduce network bandwidth to Splunk HEC.

**Steps:**

1. Add or set `gzip: true` in Splunk configuration
2. Send SIGHUP to reload
3. Monitor HEC traffic to verify compression is working

```yaml
# Before
splunk:
  hec_token: "abc123"

# After
splunk:
  hec_token: "abc123"
  gzip: true
```

```bash
pkill -HUP relay
```

## Handling Reload Errors

### Error: Non-Reloadable Parameter Changed

```json
{"time":"2025-11-14T10:30:00.000Z","level":"ERROR","msg":"failed to reload configuration","error":"listener user-activity: listen address changed (requires restart)"}
```

**Cause**: You attempted to change a parameter that requires a full restart.

**Solution**:
1. Revert the non-reloadable parameter change
2. Keep only the reloadable parameter changes
3. Retry the reload
4. Schedule a maintenance window for the non-reloadable changes

### Error: Invalid CIDR Format

```json
{"time":"2025-11-14T10:30:00.000Z","level":"ERROR","msg":"failed to reload configuration","error":"listener user-activity: failed to create new ACL: invalid CIDR address: 10.0.0.0/33"}
```

**Cause**: Invalid CIDR notation in `allowed_cidrs`.

**Solution**:
1. Fix the CIDR notation (valid subnet masks are /0 through /32 for IPv4)
2. Retry the reload
3. The old ACL remains active until a successful reload

### Error: Configuration File Not Found

```json
{"time":"2025-11-14T10:30:00.000Z","level":"ERROR","msg":"failed to reload configuration","error":"failed to load configuration: configuration file not found: /etc/relay/config.yml"}
```

**Cause**: Config file was moved or deleted.

**Solution**:
1. Verify the config file exists at the expected path
2. Check file permissions are readable by relay process
3. Restore from backup if necessary

### Error: Invalid YAML Syntax

```json
{"time":"2025-11-14T10:30:00.000Z","level":"ERROR","msg":"failed to reload configuration","error":"failed to parse YAML config: yaml: line 15: mapping values are not allowed in this context"}
```

**Cause**: YAML syntax error introduced during editing.

**Solution**:
1. Validate YAML syntax: `yamllint /etc/relay/config.yml`
2. Check for indentation issues (YAML is whitespace-sensitive)
3. Revert to last known good configuration
4. Fix syntax errors and retry

## Automation and Integration

### Automated Reload Script

Create a script for safe configuration reload with validation:

```bash
#!/bin/bash
# reload-relay.sh - Safely reload relay configuration

set -euo pipefail

CONFIG_FILE="/etc/relay/config.yml"
BACKUP_DIR="/var/backups/relay"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Backup current config
mkdir -p "$BACKUP_DIR"
cp "$CONFIG_FILE" "$BACKUP_DIR/config_${TIMESTAMP}.yml"
echo "Backed up config to $BACKUP_DIR/config_${TIMESTAMP}.yml"

# Validate YAML syntax
if ! yamllint -d relaxed "$CONFIG_FILE" 2>/dev/null; then
    echo "Error: Invalid YAML syntax in $CONFIG_FILE"
    exit 1
fi

# Send SIGHUP
if pkill -HUP relay; then
    echo "Sent SIGHUP signal to relay"
    sleep 2

    # Check logs for success
    if journalctl -u relay --since "30 seconds ago" | grep -q "configuration reloaded successfully"; then
        echo "✓ Configuration reloaded successfully"
        exit 0
    else
        echo "✗ Reload may have failed, check logs:"
        journalctl -u relay --since "30 seconds ago" | grep -E "(ERROR|WARN)"
        exit 1
    fi
else
    echo "Error: Failed to send signal to relay process"
    exit 1
fi
```

Make it executable:

```bash
chmod +x reload-relay.sh
```

### Ansible Playbook

```yaml
---
- name: Reload relay configuration
  hosts: relay_servers
  become: yes
  tasks:
    - name: Update relay configuration
      template:
        src: relay-config.yml.j2
        dest: /etc/relay/config.yml
        owner: relay
        group: relay
        mode: '0640'
        validate: 'yamllint -d relaxed %s'
      register: config_update

    - name: Reload relay via SIGHUP
      command: pkill -HUP relay
      when: config_update.changed

    - name: Wait for reload to complete
      pause:
        seconds: 2
      when: config_update.changed

    - name: Check reload success
      command: journalctl -u relay --since "1 minute ago"
      register: relay_logs
      when: config_update.changed
      failed_when: "'configuration reloaded successfully' not in relay_logs.stdout"
```

### Systemd Service Configuration

To enable `systemctl reload relay`, add this to your systemd service file:

```ini
# /etc/systemd/system/relay.service

[Unit]
Description=Relay Service for ZPA LSS to Splunk HEC
After=network.target

[Service]
Type=simple
User=relay
Group=relay
ExecStart=/usr/local/bin/relay --config /etc/relay/config.yml
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Then reload systemd and restart:

```bash
sudo systemctl daemon-reload
sudo systemctl restart relay
```

Now you can use:

```bash
sudo systemctl reload relay
```

## Testing Configuration Reload

### Test 1: Basic Reload Test

```bash
# 1. Start relay with initial config
./relay --config config.yml &
RELAY_PID=$!

# 2. Verify it's running
ps -p $RELAY_PID

# 3. Make a simple change (e.g., add gzip)
sed -i 's/gzip: false/gzip: true/' config.yml

# 4. Send SIGHUP
kill -HUP $RELAY_PID

# 5. Check logs for success message
# Look for "configuration reloaded successfully"

# 6. Verify service still running
ps -p $RELAY_PID
```

### Test 2: ACL Reload Test

```bash
# Terminal 1: Start relay
./relay --config config.yml

# Terminal 2: Test connection from allowed IP
echo '{"test": "message"}' | nc localhost 9015

# Terminal 3: Update ACL to block all
sed -i 's/allowed_cidrs: "0.0.0.0\/0"/allowed_cidrs: "192.168.99.0\/24"/' config.yml
pkill -HUP relay

# Terminal 2: Try connection again (should be denied)
echo '{"test": "message"}' | nc localhost 9015
# Check logs for "connection denied by ACL"
```

### Test 3: Error Handling Test

```bash
# Intentionally break config and verify error handling
cp config.yml config.yml.backup
echo "invalid: yaml: [syntax" >> config.yml

# Attempt reload
pkill -HUP relay

# Check logs for error message
# Old config should still be active

# Restore config
mv config.yml.backup config.yml
pkill -HUP relay

# Verify successful reload
```

## Best Practices

### Configuration Management

1. **Always backup before changes**:
   ```bash
   cp config.yml config.yml.$(date +%Y%m%d_%H%M%S)
   ```

2. **Validate YAML before reload**:
   ```bash
   yamllint config.yml
   ```

3. **Use version control**:
   ```bash
   git diff config.yml
   git commit -m "Update HEC token after rotation"
   ```

4. **Test in non-production first**: Always test configuration changes in a staging environment before production.

### Monitoring and Alerting

1. **Log reload attempts**: Ensure all SIGHUP events are logged for audit trail
2. **Alert on reload failures**: Configure monitoring to alert if reload fails
3. **Track reload frequency**: Monitor how often config is reloaded
4. **Verify functionality**: After reload, test that logs still flow to Splunk

### Change Management

1. **Document the change**: Record why configuration was changed
2. **Coordinate with team**: Notify team members of configuration changes
3. **Plan rollback**: Have a plan to revert if issues arise
4. **Verify impact**: Confirm no negative impact on log collection after reload

## Security Considerations

### File Permissions

Ensure configuration files have appropriate permissions:

```bash
# Config file should not be world-readable (contains tokens)
chmod 640 /etc/relay/config.yml
chown relay:relay /etc/relay/config.yml

# Verify
ls -l /etc/relay/config.yml
# Should show: -rw-r----- 1 relay relay
```

### Token Rotation

When rotating HEC tokens:

1. Generate new token in Splunk first
2. Update relay config with new token
3. Reload relay configuration
4. Verify logs flow with new token (monitor for 15-30 minutes)
5. Only then revoke the old token in Splunk

This ensures no gap in log collection during token rotation.

### Audit Trail

Log all configuration reloads for security audit:

```bash
# Create audit log entry before reload
echo "$(date -Iseconds) - $(whoami) - Reloading relay config" >> /var/log/relay/config-changes.log

# Perform reload
pkill -HUP relay

# Verify and log result
if journalctl -u relay --since "30 seconds ago" | grep -q "configuration reloaded successfully"; then
    echo "$(date -Iseconds) - SUCCESS" >> /var/log/relay/config-changes.log
else
    echo "$(date -Iseconds) - FAILED" >> /var/log/relay/config-changes.log
fi
```

## Troubleshooting Checklist

If configuration reload isn't working:

- [ ] Verify relay supports SIGHUP (check version)
- [ ] Confirm relay process is running (`ps aux | grep relay`)
- [ ] Check config file exists at expected path
- [ ] Verify config file is readable by relay user
- [ ] Validate YAML syntax (`yamllint config.yml`)
- [ ] Ensure only reloadable parameters were changed
- [ ] Check relay logs for specific error messages
- [ ] Verify signal was sent to correct PID
- [ ] Confirm no file locks preventing config read
- [ ] Check disk space for log file writes

## When to Restart Instead of Reload

Restart the service (instead of reload) when:

- Changing listener addresses or adding/removing listeners
- Updating TLS certificates (or use configuration that supports hot reload for certs)
- Modifying batch or circuit breaker configuration
- Changing output directories or file prefixes
- Upgrading to a new version of relay
- After multiple config reloads to refresh all state
- When troubleshooting unexplained issues

Restart command:

```bash
# Systemd
sudo systemctl restart relay

# Direct process
pkill relay
./relay --config config.yml
```

## Further Reading

- [Main README - Runtime Configuration Reload](../../README.md#runtime-configuration-reload)
- [Architecture Decision Record: Configuration Management](#) _(if exists)_
- [Unix Signals Reference](https://man7.org/linux/man-pages/man7/signal.7.html)
- [YAML Syntax Guide](https://yaml.org/spec/1.2.2/)

## Support

If you encounter issues not covered in this guide:

1. Check relay logs with `--log-level debug` for detailed information
2. Review the [troubleshooting section](../../README.md#troubleshooting) in the main README
3. Search [existing issues](https://github.com/scottbrown/relay/issues) on GitHub
4. Open a new issue with:
   - Relay version (`relay --version`)
   - Configuration file (with secrets redacted)
   - Log output showing the error
   - Steps to reproduce

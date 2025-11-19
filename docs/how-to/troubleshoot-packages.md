# How to Troubleshoot Package Issues

This guide helps you diagnose and resolve common issues with relay RPM and DEB packages.

## Quick Diagnostics

Run these commands first to gather information:

```bash
# Check package is installed
rpm -q relay           # RPM systems
dpkg -l relay          # DEB systems

# Check service status
systemctl status relay.service

# Check service logs
journalctl -u relay.service -n 50

# Verify binary
/usr/local/bin/relay --version

# Check user and group
getent passwd relay
getent group relay
id relay

# Check directory permissions
ls -la /var/spool/relay/
ls -la /var/log/relay/
```

## Installation Issues

### Issue: Package Installation Fails with "Not a compatible architecture"

**Symptom**:
```
Error: package relay-1.0.0-1.x86_64 is intended for a different operating system
```

**Cause**: Architecture mismatch between package and system.

**Solution**:

Check your system architecture:
```bash
uname -m
```

Download the correct package:
- `x86_64` or `amd64` systems → Use `x86_64` RPM or `amd64` DEB
- `aarch64` or `arm64` systems → Use `aarch64` RPM or `arm64` DEB

### Issue: "systemd dependency not satisfied"

**Symptom**:
```
Error: Package: relay-1.0.0-1 requires systemd
```

**Cause**: System doesn't have systemd or package manager can't find it.

**Solution**:

Verify systemd is installed:
```bash
systemctl --version
```

If systemd is not installed:
```bash
# RHEL/CentOS/Amazon Linux
sudo yum install systemd

# Debian/Ubuntu
sudo apt-get install systemd
```

### Issue: "Package is already installed"

**Symptom**:
```
Package relay-1.0.0-1 already installed
```

**Cause**: Trying to install when package is already present.

**Solution**:

Either upgrade or reinstall:

```bash
# Upgrade to newer version
sudo yum upgrade /path/to/relay-1.1.0-1.x86_64.rpm    # RPM
sudo apt install /path/to/relay_1.1.0_amd64.deb       # DEB

# Reinstall same version
sudo yum reinstall relay                               # RPM
sudo apt reinstall relay                               # DEB

# Remove and install fresh
sudo yum remove relay && sudo yum install /path/to/package.rpm   # RPM
sudo apt remove relay && sudo apt install /path/to/package.deb   # DEB
```

### Issue: "File conflicts with file from package"

**Symptom**:
```
Error: file /usr/local/bin/relay from install of relay-1.0.0-1 conflicts with file from package other-pkg
```

**Cause**: Another package provides the same file.

**Solution**:

Identify the conflicting package:
```bash
# RPM
rpm -qf /usr/local/bin/relay

# DEB
dpkg -S /usr/local/bin/relay
```

Remove the conflicting package or manually remove the file (if safe):
```bash
sudo rm /usr/local/bin/relay
sudo yum install /path/to/relay.rpm    # Then try installing again
```

## Service Issues

### Issue: Service Fails to Start After Installation

**Symptom**:
```bash
$ sudo systemctl start relay.service
Job for relay.service failed. See "systemctl status relay.service" and "journalctl -xe" for details.
```

**Diagnosis**:

Check the service status:
```bash
sudo systemctl status relay.service
```

Check the service logs:
```bash
sudo journalctl -u relay.service -n 50 --no-pager
```

**Common Causes**:

#### 1. Missing Configuration File

**Error in logs**:
```
configuration file is required
```

**Solution**:
```bash
sudo mkdir -p /etc/relay
sudo /usr/local/bin/relay template > /tmp/config.yaml
sudo mv /tmp/config.yaml /etc/relay/config.yaml
sudo chown relay:relay /etc/relay/config.yaml
sudo chmod 600 /etc/relay/config.yaml
```

#### 2. Invalid Configuration

**Error in logs**:
```
failed to parse YAML config
invalid HEC URL
```

**Solution**:

Validate your configuration:
```bash
# Check YAML syntax
sudo /usr/local/bin/relay --config /etc/relay/config.yaml 2>&1 | head -20
```

Common configuration errors:
- Missing required fields (`listen_addr`, `log_type`, `output_dir`)
- Invalid HEC URL (must start with `http://` or `https://`)
- Invalid CIDR notation in `allowed_cidrs`
- TLS files don't exist or aren't readable

#### 3. Port Already in Use

**Error in logs**:
```
listen tcp :9015: bind: address already in use
```

**Solution**:

Find what's using the port:
```bash
sudo lsof -i :9015
sudo netstat -tulpn | grep 9015
```

Either:
- Stop the conflicting service
- Change the port in `/etc/relay/config.yaml`

#### 4. Permission Denied on Directories

**Error in logs**:
```
cannot create output directory: permission denied
failed to write to /var/log/relay/: permission denied
```

**Solution**:

Verify directory ownership:
```bash
sudo chown -R relay:relay /var/log/relay
sudo chown -R relay:relay /var/spool/relay
sudo chmod 755 /var/log/relay
sudo chmod 755 /var/spool/relay
```

Create missing directories:
```bash
sudo mkdir -p /var/log/relay
sudo chown relay:relay /var/log/relay
sudo chmod 755 /var/log/relay
```

### Issue: Service Starts But Immediately Stops

**Symptom**:
```bash
$ sudo systemctl status relay.service
Active: failed (Result: exit-code)
Main PID: 12345 (code=exited, status=1/FAILURE)
```

**Diagnosis**:

Check the exit code and last log entries:
```bash
sudo journalctl -u relay.service -n 100 --no-pager
```

**Common Causes**:

1. **Invalid TLS certificates**: Check cert/key files exist and are readable by `relay` user
2. **Network issues**: HEC health check may be failing if configured
3. **Resource exhaustion**: Out of file descriptors or memory

### Issue: Service Not Enabled After Installation

**Symptom**:
```bash
$ systemctl is-enabled relay.service
disabled
```

**Cause**: Post-install script failed to run or systemd not running during installation (containers).

**Solution**:

Manually enable the service:
```bash
sudo systemctl daemon-reload
sudo systemctl enable relay.service
sudo systemctl start relay.service
```

## User and Permission Issues

### Issue: "relay user does not exist"

**Symptom**:
```bash
$ getent passwd relay
(no output)
```

**Cause**: Post-install script failed or didn't run.

**Solution**:

Manually create the user and group:
```bash
# Create group
sudo groupadd --system relay

# Create user
sudo useradd --system --gid relay --no-create-home \
             --shell /usr/sbin/nologin relay

# Verify
id relay
```

### Issue: "Permission denied" when service writes logs

**Symptom in logs**:
```
failed to create file: permission denied
```

**Diagnosis**:

Check directory ownership:
```bash
ls -la /var/log/relay/
ls -la /var/spool/relay/
stat /var/log/relay
```

**Solution**:

Fix ownership and permissions:
```bash
sudo chown -R relay:relay /var/log/relay
sudo chown -R relay:relay /var/spool/relay
sudo chmod 755 /var/log/relay
sudo chmod 755 /var/spool/relay
```

### Issue: DLQ Directories Missing

**Symptom**:
```
failed to write to DLQ: no such file or directory
```

**Diagnosis**:
```bash
ls -la /var/spool/relay/dlq/
```

**Solution**:

Recreate DLQ structure:
```bash
sudo mkdir -p /var/spool/relay/dlq/user-activity
sudo mkdir -p /var/spool/relay/dlq/user-status
sudo mkdir -p /var/spool/relay/dlq/app-connector-status
sudo mkdir -p /var/spool/relay/dlq/audit
sudo chown -R relay:relay /var/spool/relay
sudo chmod 755 /var/spool/relay
```

## Upgrade Issues

### Issue: Service Still Running Old Version After Upgrade

**Symptom**:
```bash
$ /usr/local/bin/relay --version
relay version 1.1.0

$ ps aux | grep relay
relay ... /usr/local/bin/relay --config /etc/relay/config.yaml  # But logs show old behaviour
```

**Cause**: Binary was upgraded but service wasn't restarted.

**Solution**:

Restart the service:
```bash
sudo systemctl restart relay.service
```

Verify new version is running:
```bash
ps aux | grep relay
journalctl -u relay.service -n 10 | grep version
```

### Issue: Configuration Overwritten During Upgrade

**Symptom**: Configuration file has default values after upgrade.

**Cause**: This should **not** happen - packages don't include `/etc/relay/config.yaml`.

**Diagnosis**:

Check if config was accidentally placed in package-managed location:
```bash
# Check what files the package owns
rpm -ql relay    # RPM
dpkg -L relay    # DEB
```

**Solution**:

If `/etc/relay/config.yaml` is listed, this is a packaging bug. Restore from backup:
```bash
sudo cp /etc/relay/config.yaml.backup /etc/relay/config.yaml
sudo systemctl restart relay.service
```

### Issue: Systemd Unit File Not Updated

**Symptom**: New systemd features not working after upgrade.

**Cause**: Systemd daemon not reloaded.

**Solution**:

```bash
sudo systemctl daemon-reload
sudo systemctl restart relay.service
```

## Uninstallation Issues

### Issue: Package Removal Hangs

**Symptom**: `yum remove` or `apt remove` hangs indefinitely.

**Cause**: Pre-remove script waiting for service to stop, but service is stuck.

**Solution**:

Force stop the service in another terminal:
```bash
sudo systemctl kill -s SIGKILL relay.service
```

Then continue with package removal.

### Issue: Directories Not Removed After Uninstall

**Symptom**:
```bash
$ sudo yum remove relay
$ ls -la /var/spool/relay
drwxr-xr-x 3 relay relay ...
```

**Cause**: This is **expected behaviour** - data directories may contain important logs.

**Solution**:

Manually remove if desired:
```bash
sudo rm -rf /var/log/relay
sudo rm -rf /var/spool/relay
sudo rm -rf /etc/relay
```

### Issue: User and Group Remain After Uninstall

**Symptom**:
```bash
$ getent passwd relay
relay:x:998:998::/home/relay:/usr/sbin/nologin
```

**Cause**: This is **expected behaviour** - system policy is to not automatically remove user accounts.

**Solution**:

Manually remove if desired (ensure no other packages use this user):
```bash
sudo userdel relay
sudo groupdel relay
```

## Configuration Issues

### Issue: "HEC URL must use http or https scheme"

**Symptom**:
```
invalid HEC URL: HEC URL must use http or https scheme
```

**Cause**: HEC URL missing protocol prefix.

**Solution**:

Update configuration:
```yaml
# Bad
hec_url: "splunk.example.com:8088/services/collector/raw"

# Good
hec_url: "https://splunk.example.com:8088/services/collector/raw"
```

### Issue: "HEC health check failed with status: 403"

**Symptom in logs**:
```
invalid Splunk HEC token (403 Forbidden)
```

**Cause**: HEC token is incorrect or expired.

**Solution**:

Verify token in Splunk:
1. Log into Splunk
2. Go to Settings → Data Inputs → HTTP Event Collector
3. Verify token exists and is enabled
4. Update `/etc/relay/config.yaml` with correct token
5. Restart service: `sudo systemctl restart relay.service`

### Issue: Cannot Reload Configuration with SIGHUP

**Symptom**: Configuration changes not taking effect after `kill -HUP`.

**Cause**: Some configuration parameters require full restart.

**Solution**:

Reloadable via SIGHUP:
- HEC tokens
- Source types
- Gzip settings

Require restart:
- Listen addresses
- TLS settings
- Batch configuration
- Circuit breaker settings

For non-reloadable changes:
```bash
sudo systemctl restart relay.service
```

## Logging and Debugging

### Enable Debug Logging

Temporarily run relay with debug output:

```bash
# Stop service
sudo systemctl stop relay.service

# Run manually with debug logging
sudo -u relay /usr/local/bin/relay --config /etc/relay/config.yaml

# Or with specific log level if supported
# Check --help for available flags
```

### View Detailed Service Logs

```bash
# Last 100 lines
sudo journalctl -u relay.service -n 100

# Follow logs in real-time
sudo journalctl -u relay.service -f

# Logs from last hour
sudo journalctl -u relay.service --since "1 hour ago"

# Logs with specific priority (error and above)
sudo journalctl -u relay.service -p err

# Export logs to file
sudo journalctl -u relay.service --since "2024-01-01" > relay-logs.txt
```

### Check Systemd Unit File

View the active unit file:
```bash
systemctl cat relay.service
```

Check for override files:
```bash
systemctl status relay.service | grep "Drop-In"
ls -la /etc/systemd/system/relay.service.d/
```

## Getting Help

If you're still stuck after trying these troubleshooting steps:

1. **Gather diagnostic information**:
   ```bash
   # System info
   cat /etc/os-release
   uname -a

   # Package info
   rpm -qi relay || dpkg -s relay

   # Service status
   systemctl status relay.service

   # Recent logs (last 200 lines)
   sudo journalctl -u relay.service -n 200 --no-pager

   # Configuration (redact sensitive tokens!)
   sudo cat /etc/relay/config.yaml
   ```

2. **Check for known issues**: https://github.com/scottbrown/relay/issues

3. **Open an issue**: https://github.com/scottbrown/relay/issues/new

Include:
- Operating system and version
- Package version (`relay --version`)
- Error messages from logs
- Steps to reproduce
- What you've already tried

## Related Documentation

- [Package Reference](../reference/packaging.md) - Technical details about package structure
- [Configuration Reference](../reference/configuration.md) - All configuration options
- [How to Build and Test Packages Locally](build-packages.md) - For package development

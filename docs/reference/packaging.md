# Package Reference

*Technical reference for relay RPM and DEB packages.*

This document provides comprehensive technical details about the structure, behaviour, and components of relay packages.

## Table of Contents

- [Package Formats](#package-formats)
- [Package Metadata](#package-metadata)
- [File Layout](#file-layout)
- [System User and Group](#system-user-and-group)
- [Systemd Service](#systemd-service)
- [Directory Structure](#directory-structure)
- [Installation Scripts](#installation-scripts)
- [Upgrade Behaviour](#upgrade-behaviour)
- [Removal Behaviour](#removal-behaviour)
- [Security Hardening](#security-hardening)
- [Dependencies](#dependencies)

## Package Formats

The relay service is distributed in two native Linux package formats.

### RPM (Red Hat Package Manager)

**Distributions**: RHEL, CentOS, Fedora, Amazon Linux, Rocky Linux, AlmaLinux

**Architecture Support**:
- `x86_64` - Intel/AMD 64-bit processors
- `aarch64` - ARM 64-bit processors (AWS Graviton, etc.)

**Naming Convention**: `relay-{version}-{release}.{arch}.rpm`

**Example**: `relay-1.0.0-1.x86_64.rpm`

### DEB (Debian Package)

**Distributions**: Debian, Ubuntu, Linux Mint

**Architecture Support**:
- `amd64` - Intel/AMD 64-bit processors
- `arm64` - ARM 64-bit processors

**Naming Convention**: `relay_{version}_{arch}.deb`

**Example**: `relay_1.0.0_amd64.deb`

## Package Metadata

### Common Metadata

| Field | Value |
|-------|-------|
| **Name** | `relay` |
| **Summary** | ZPA Log Streaming Relay Binary |
| **Licence** | Proprietary |
| **URL** | https://github.com/scottbrown/relay |
| **Maintainer** | relay@example.com |

### Dependencies

All packages declare a dependency on `systemd`, ensuring the init system is available for service management.

**RPM**: `Requires: systemd`

**DEB**: `Depends: systemd`

## File Layout

Packages install files to the following locations:

| Path | Type | Owner | Permissions | Purpose |
|------|------|-------|-------------|---------|
| `/usr/local/bin/relay` | Binary | `root:root` | `755` | Main relay executable |
| `/usr/lib/systemd/system/relay.service` | File | `root:root` | `644` | Systemd unit file |
| `/var/spool/relay/` | Directory | `relay:relay` | `755` | Data directory (created by post-install) |
| `/var/spool/relay/dlq/` | Directory | `relay:relay` | `755` | Dead letter queue root |
| `/var/spool/relay/dlq/user-activity/` | Directory | `relay:relay` | `755` | DLQ for user-activity logs |
| `/var/spool/relay/dlq/user-status/` | Directory | `relay:relay` | `755` | DLQ for user-status logs |
| `/var/spool/relay/dlq/app-connector-status/` | Directory | `relay:relay` | `755` | DLQ for app-connector-status logs |
| `/var/spool/relay/dlq/audit/` | Directory | `relay:relay` | `755` | DLQ for audit logs |

**Note**: Configuration files (`/etc/relay/config.yaml`) are **not** included in packages. Users must create configuration manually using `relay template`.

## System User and Group

Packages create a dedicated system user and group for running the relay service.

### User Details

| Attribute | Value |
|-----------|-------|
| **Username** | `relay` |
| **User Type** | System user (`--system` flag) |
| **Home Directory** | None (`--no-create-home`) |
| **Shell** | `/usr/sbin/nologin` (RPM) or `/sbin/nologin` (DEB) |
| **UID** | Auto-assigned by system (typically 900-999 range) |
| **Primary Group** | `relay` |

### Group Details

| Attribute | Value |
|-----------|-------|
| **Group Name** | `relay` |
| **Group Type** | System group (`--system` flag) |
| **GID** | Auto-assigned by system (typically 900-999 range) |

### Creation Logic

The post-install script creates the user and group if they don't already exist:

```bash
# Create group
if ! getent group relay > /dev/null 2>&1; then
    groupadd --system relay
fi

# Create user
if ! getent passwd relay > /dev/null 2>&1; then
    useradd --system --gid relay --no-create-home \
            --shell /usr/sbin/nologin relay
fi
```

**Idempotency**: User/group creation is idempotent - safe to run multiple times.

**Upgrade Behaviour**: User and group persist across package upgrades and are not modified.

## Systemd Service

### Service File Location

`/usr/lib/systemd/system/relay.service`

### Service Configuration

```ini
[Unit]
Description=Relay Binary for Log Forwarding
After=network.target
Wants=network.target

[Service]
Type=simple
User=relay
Group=relay
ExecStart=/usr/local/bin/relay --config /etc/relay/config.yaml
Restart=on-failure
RestartSec=10
StandardOutput=journal
StandardError=journal

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/log/relay /var/spool/relay

[Install]
WantedBy=multi-user.target
```

### Service Behaviour

| Attribute | Value | Purpose |
|-----------|-------|---------|
| **Type** | `simple` | Process runs in foreground |
| **User** | `relay` | Run as dedicated system user |
| **Group** | `relay` | Run as dedicated system group |
| **Restart** | `on-failure` | Automatic restart on crashes |
| **RestartSec** | `10` | Wait 10 seconds before restart |
| **StandardOutput** | `journal` | Logs to systemd journal |
| **StandardError** | `journal` | Errors to systemd journal |

### Default State

After package installation:
- ✅ Service is **enabled** (starts automatically on boot)
- ❌ Service is **not started** (allows configuration first)

Users must:
1. Create `/etc/relay/config.yaml`
2. Start service manually: `systemctl start relay.service`

## Directory Structure

### Created by Package

The post-install script creates:

```
/var/spool/relay/          (owner: relay:relay, mode: 755)
├── dlq/                   (owner: relay:relay, mode: 755)
    ├── user-activity/     (owner: relay:relay, mode: 755)
    ├── user-status/       (owner: relay:relay, mode: 755)
    ├── app-connector-status/ (owner: relay:relay, mode: 755)
    └── audit/             (owner: relay:relay, mode: 755)
```

### User-Created Directories

Users must create:

```
/etc/relay/                (recommended: root:root, mode: 755)
└── config.yaml            (recommended: relay:relay, mode: 600)

/var/log/relay/            (recommended: relay:relay, mode: 755)
└── *.ndjson               (created by relay at runtime)
```

## Installation Scripts

Packages include lifecycle scripts that run at specific points during installation, upgrade, and removal.

### Post-Install Script

**Runs**: After files are extracted and installed

**Purpose**: Set up system user, directories, and enable service

**Actions**:
1. Create `relay` system group (if doesn't exist)
2. Create `relay` system user (if doesn't exist)
3. Create DLQ directory structure at `/var/spool/relay/dlq/`
4. Set ownership to `relay:relay` on all created directories
5. Set permissions to `755` on all created directories
6. Reload systemd daemon configuration
7. Enable relay service (but don't start it)

**Idempotency**: Safe to run multiple times

**Failure Handling**: Script uses `set -euo pipefail` - any error aborts installation

### Pre-Remove Script

**Runs**: Before package files are removed

**Purpose**: Stop and disable service on uninstallation

**Actions**:
1. Check if this is an uninstall (not an upgrade)
   - RPM: `$1 = 0` means uninstall
   - DEB: `$1 = "remove"` means uninstall
2. Stop relay service if running
3. Disable relay service

**Upgrade Behaviour**: Script does **not** stop service during upgrades

### Post-Remove Script

**Runs**: After package files are removed

**Purpose**: Clean up data directories on complete removal

**Actions**:
1. Check if this is complete removal (not an upgrade)
2. Remove `/var/log/relay` directory and all contents
3. Remove `/var/spool/relay` directory and all contents
4. Reload systemd daemon configuration

**Upgrade Behaviour**: Directories are **not** removed during upgrades

**User/Group Handling**: System user and group are **not** removed (system policy)

## Upgrade Behaviour

### Package Upgrade Process

When upgrading from one version to another:

1. **Pre-Remove** script runs (detects upgrade, skips stopping service)
2. Old files are replaced with new files
3. **Post-Install** script runs (idempotent operations)
4. Service continues running with old binary until restarted

### Manual Service Restart Required

After package upgrade:

```bash
sudo systemctl restart relay.service
```

**Rationale**: Automatic restart could interrupt log ingestion. Manual restart gives operators control over timing.

### Configuration Persistence

- Configuration files (`/etc/relay/config.yaml`) persist across upgrades
- Log files in `/var/log/relay/` persist across upgrades
- DLQ data in `/var/spool/relay/dlq/` persists across upgrades

### Systemd Unit File Updates

- Unit file updates take effect after `systemctl daemon-reload`
- Running service continues with old unit file until restarted

## Removal Behaviour

### Complete Removal (Uninstall)

```bash
# RPM
sudo yum remove relay

# DEB
sudo apt remove relay
```

**Actions**:
1. Service is stopped
2. Service is disabled
3. Package files are removed (`/usr/local/bin/relay`, service file)
4. Data directories are removed (`/var/log/relay`, `/var/spool/relay`)
5. User configuration remains (`/etc/relay/` - not managed by package)

### What Persists After Removal

- System user `relay` (system policy - user accounts not removed automatically)
- System group `relay` (system policy - groups not removed automatically)
- Configuration directory `/etc/relay/` (user-created, not managed by package)

### Purge (DEB only)

Complete removal including configuration:

```bash
sudo apt purge relay
```

Same behaviour as `apt remove` - package doesn't manage `/etc/relay/`.

## Security Hardening

### Systemd Security Features

The service file includes multiple security restrictions:

| Directive | Effect |
|-----------|--------|
| `NoNewPrivileges=true` | Cannot gain new privileges via setuid/setgid |
| `PrivateTmp=true` | Private `/tmp` and `/var/tmp` namespace |
| `ProtectSystem=strict` | Entire filesystem read-only except explicit paths |
| `ProtectHome=true` | Home directories inaccessible |
| `ReadWritePaths=...` | Only `/var/log/relay` and `/var/spool/relay` writable |

### Filesystem Permissions

- Binary is owned by `root:root` with `755` permissions (world-readable, not writable)
- Service runs as non-privileged `relay` user (UID typically 900-999)
- Data directories owned by `relay:relay` with `755` permissions
- Configuration should be `relay:relay` with `600` permissions (user-managed)

### Network Isolation

- No network restrictions in systemd unit (relay needs network for HEC forwarding)
- TCP listen ports are configured in `/etc/relay/config.yaml` (user-controlled)
- Firewall rules must be configured separately by operators

### Privilege Separation

- Binary runs as dedicated system user, not `root`
- No capability grants required
- No setuid/setgid bits
- No sudo privileges required during runtime

## Dependencies

### Build-Time Dependencies

Not relevant to end users - packages are pre-built.

### Runtime Dependencies

**Required**:
- `systemd` - Init system for service management
- Linux kernel 3.10+ - For modern networking features
- glibc 2.17+ - Standard C library

**Implicit** (provided by base OS):
- `/usr/sbin/nologin` or `/sbin/nologin` - Non-login shell
- `useradd`, `groupadd` - User management commands (shadow-utils)
- `getent` - Query system databases (glibc-common)

### Network Requirements

- Outbound HTTPS/HTTP to Splunk HEC endpoints (configured by user)
- Inbound TCP on configured listener ports (configured by user)

## Version Information

### Binary Version

Check installed binary version:

```bash
/usr/local/bin/relay --version
```

### Package Version

Check installed package version:

```bash
# RPM
rpm -q relay

# DEB
dpkg -l relay
```

### Determining Package Source

Check package metadata:

```bash
# RPM
rpm -qi relay

# DEB
dpkg -s relay
```

## Related Documentation

- [How to Build and Test Packages Locally](../how-to/build-packages.md)
- [ADR-0017: FPM for Package Distribution](../explanation/adr/0017-fpm-packaging.md)
- [Configuration Reference](configuration.md)

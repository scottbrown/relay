#!/usr/bin/env bash
# Post-installation script for relay package
# Creates relay user/group, DLQ directory structure, and enables systemd service

set -euo pipefail

# Create relay system user and group if they don't exist
if ! getent group relay > /dev/null 2>&1; then
    groupadd --system relay
fi

if ! getent passwd relay > /dev/null 2>&1; then
    useradd --system --gid relay --no-create-home --shell /usr/sbin/nologin relay 2>/dev/null || \
    useradd --system --gid relay --no-create-home --shell /sbin/nologin relay
fi

# Create DLQ directory structure
mkdir -p /var/spool/relay/dlq/user-activity
mkdir -p /var/spool/relay/dlq/user-status
mkdir -p /var/spool/relay/dlq/app-connector-status
mkdir -p /var/spool/relay/dlq/audit
chown -R relay:relay /var/spool/relay
chmod 755 /var/spool/relay

# Register and enable systemd service (if systemd is available)
if [ -d /run/systemd/system ]; then
    systemctl daemon-reload
    systemctl enable relay.service
    echo "Relay package installed successfully"
    echo "Configure /etc/relay/config.yaml before starting the service"
    echo "Start the service with: systemctl start relay.service"
else
    echo "Relay package installed successfully"
    echo "Note: systemd not detected - service not enabled"
    echo "Configure /etc/relay/config.yaml before starting the service"
fi

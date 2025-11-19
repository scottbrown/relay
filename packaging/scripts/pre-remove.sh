#!/usr/bin/env bash
# Pre-removal script for relay package
# Stops and disables service on package removal

set -euo pipefail

# Stop and disable service on package removal
# For RPM: $1 = 0 means uninstall, $1 = 1 means upgrade
# For DEB: $1 = "remove" means uninstall, $1 = "upgrade" means upgrade
if [ "${1:-0}" = "remove" ] || [ "${1:-0}" = "0" ]; then
    if systemctl is-active --quiet relay.service; then
        systemctl stop relay.service
    fi
    systemctl disable relay.service
    echo "Relay service stopped and disabled"
fi

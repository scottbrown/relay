#!/usr/bin/env bash
# Post-removal script for relay package
# Cleans up directories on complete removal

set -euo pipefail

# Clean up directories on complete removal
# For RPM: $1 = 0 means uninstall, $1 = 1 means upgrade
# For DEB: $1 = "remove" or "purge" means uninstall
if [ "${1:-0}" = "remove" ] || [ "${1:-0}" = "purge" ] || [ "${1:-0}" = "0" ]; then
    rm -rf /var/log/relay
    rm -rf /var/spool/relay
    echo "Relay directories cleaned up"
fi

systemctl daemon-reload

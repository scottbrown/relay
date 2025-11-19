#!/usr/bin/env bash
# Test script for validating RPM and DEB package installation using Docker

set -euo pipefail

# Colours for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Colour

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DIST_DIR="${SCRIPT_DIR}/.dist"

# Function to print coloured messages
info() {
    echo -e "${GREEN}[INFO]${NC} $*"
}

error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

# Function to test RPM installation on a given distribution
test_rpm() {
    local distro=$1
    local image=$2
    local package_file=$3

    info "Testing RPM installation on ${distro}..."

    docker run --rm \
        -v "${DIST_DIR}:/packages:ro" \
        "${image}" \
        bash -c "
            set -e
            echo '=== Installing package ==='
            yum install -y /packages/${package_file}

            echo '=== Verifying binary installation ==='
            test -f /usr/local/bin/relay || { echo 'Binary not found'; exit 1; }
            test -x /usr/local/bin/relay || { echo 'Binary not executable'; exit 1; }

            echo '=== Verifying systemd service ==='
            test -f /usr/lib/systemd/system/relay.service || { echo 'Service file not found'; exit 1; }

            echo '=== Verifying relay user and group ==='
            getent passwd relay || { echo 'relay user not found'; exit 1; }
            getent group relay || { echo 'relay group not found'; exit 1; }
            id -u relay > /dev/null 2>&1 || { echo 'relay user ID not found'; exit 1; }

            echo '=== Verifying DLQ directories ==='
            test -d /var/spool/relay/dlq/user-activity || { echo 'user-activity DLQ not found'; exit 1; }
            test -d /var/spool/relay/dlq/user-status || { echo 'user-status DLQ not found'; exit 1; }
            test -d /var/spool/relay/dlq/app-connector-status || { echo 'app-connector-status DLQ not found'; exit 1; }
            test -d /var/spool/relay/dlq/audit || { echo 'audit DLQ not found'; exit 1; }

            echo '=== Verifying directory permissions ==='
            stat -c '%U:%G %a' /var/spool/relay | grep -q 'relay:relay 755' || { echo 'Wrong permissions on /var/spool/relay'; exit 1; }

            echo '=== Testing binary execution ==='
            /usr/local/bin/relay --version || { echo 'Binary version check failed'; exit 1; }
            /usr/local/bin/relay template > /tmp/test-config.yaml || { echo 'Template generation failed'; exit 1; }
            test -s /tmp/test-config.yaml || { echo 'Template file is empty'; exit 1; }

            echo '=== Verifying systemd service registration ==='
            if [ -d /run/systemd/system ]; then
                systemctl is-enabled relay.service || { echo 'Service not enabled'; exit 1; }
                echo 'Service properly enabled'
            else
                echo 'Systemd not running (container environment), skipping service check'
            fi

            echo '=== All checks passed for ${distro} ==='
        "

    if [ $? -eq 0 ]; then
        info "✅ ${distro} test PASSED"
        return 0
    else
        error "❌ ${distro} test FAILED"
        return 1
    fi
}

# Function to test DEB installation on a given distribution
test_deb() {
    local distro=$1
    local image=$2
    local package_file=$3

    info "Testing DEB installation on ${distro}..."

    docker run --rm \
        -v "${DIST_DIR}:/packages:ro" \
        "${image}" \
        bash -c "
            set -e
            export DEBIAN_FRONTEND=noninteractive

            echo '=== Installing package ==='
            apt-get update -qq
            apt-get install -y /packages/${package_file}

            echo '=== Verifying binary installation ==='
            test -f /usr/local/bin/relay || { echo 'Binary not found'; exit 1; }
            test -x /usr/local/bin/relay || { echo 'Binary not executable'; exit 1; }

            echo '=== Verifying systemd service ==='
            test -f /usr/lib/systemd/system/relay.service || { echo 'Service file not found'; exit 1; }

            echo '=== Verifying relay user and group ==='
            getent passwd relay || { echo 'relay user not found'; exit 1; }
            getent group relay || { echo 'relay group not found'; exit 1; }
            id -u relay > /dev/null 2>&1 || { echo 'relay user ID not found'; exit 1; }

            echo '=== Verifying DLQ directories ==='
            test -d /var/spool/relay/dlq/user-activity || { echo 'user-activity DLQ not found'; exit 1; }
            test -d /var/spool/relay/dlq/user-status || { echo 'user-status DLQ not found'; exit 1; }
            test -d /var/spool/relay/dlq/app-connector-status || { echo 'app-connector-status DLQ not found'; exit 1; }
            test -d /var/spool/relay/dlq/audit || { echo 'audit DLQ not found'; exit 1; }

            echo '=== Verifying directory permissions ==='
            stat -c '%U:%G %a' /var/spool/relay | grep -q 'relay:relay 755' || { echo 'Wrong permissions on /var/spool/relay'; exit 1; }

            echo '=== Testing binary execution ==='
            /usr/local/bin/relay --version || { echo 'Binary version check failed'; exit 1; }
            /usr/local/bin/relay template > /tmp/test-config.yaml || { echo 'Template generation failed'; exit 1; }
            test -s /tmp/test-config.yaml || { echo 'Template file is empty'; exit 1; }

            echo '=== Verifying systemd service registration ==='
            if [ -d /run/systemd/system ]; then
                systemctl is-enabled relay.service || { echo 'Service not enabled'; exit 1; }
                echo 'Service properly enabled'
            else
                echo 'Systemd not running (container environment), skipping service check'
            fi

            echo '=== All checks passed for ${distro} ==='
        "

    if [ $? -eq 0 ]; then
        info "✅ ${distro} test PASSED"
        return 0
    else
        error "❌ ${distro} test FAILED"
        return 1
    fi
}

# Main test execution
main() {
    info "Starting package installation tests..."

    # Check if packages exist
    if [ ! -d "${DIST_DIR}" ]; then
        error "Distribution directory not found: ${DIST_DIR}"
        error "Please run 'task package-all' first to build the packages"
        exit 1
    fi

    # Detect host architecture to test appropriate packages
    HOST_ARCH=$(uname -m)
    case "${HOST_ARCH}" in
        arm64|aarch64)
            info "Detected ARM64 architecture, testing ARM packages"
            RPM_FILE=$(find "${DIST_DIR}" -name "relay-*.aarch64.rpm" -type f -exec basename {} \; | head -n 1)
            DEB_FILE=$(find "${DIST_DIR}" -name "relay_*_arm64.deb" -type f -exec basename {} \; | head -n 1)
            ;;
        x86_64|amd64)
            info "Detected x86_64 architecture, testing x86_64 packages"
            RPM_FILE=$(find "${DIST_DIR}" -name "relay-*.x86_64.rpm" -type f -exec basename {} \; | head -n 1)
            DEB_FILE=$(find "${DIST_DIR}" -name "relay_*_amd64.deb" -type f -exec basename {} \; | head -n 1)
            ;;
        *)
            error "Unsupported architecture: ${HOST_ARCH}"
            exit 1
            ;;
    esac

    if [ -z "${RPM_FILE}" ] || [ -z "${DEB_FILE}" ]; then
        error "Package files not found in ${DIST_DIR} for architecture ${HOST_ARCH}"
        error "Please run 'task package-all' first to build the packages"
        exit 1
    fi

    info "Found packages:"
    info "  RPM: ${RPM_FILE}"
    info "  DEB: ${DEB_FILE}"
    echo

    # Track test results
    PASSED=0
    FAILED=0

    # Test RPM distributions
    info "=== Testing RPM Distributions ==="
    echo

    if test_rpm "Amazon Linux 2" "amazonlinux:2" "${RPM_FILE}"; then
        ((PASSED++))
    else
        ((FAILED++))
    fi
    echo

    if test_rpm "Rocky Linux 9" "rockylinux:9" "${RPM_FILE}"; then
        ((PASSED++))
    else
        ((FAILED++))
    fi
    echo

    # Test DEB distributions
    info "=== Testing DEB Distributions ==="
    echo

    if test_deb "Ubuntu 22.04" "ubuntu:22.04" "${DEB_FILE}"; then
        ((PASSED++))
    else
        ((FAILED++))
    fi
    echo

    if test_deb "Debian 12" "debian:12" "${DEB_FILE}"; then
        ((PASSED++))
    else
        ((FAILED++))
    fi
    echo

    # Print summary
    echo "======================================"
    info "Test Summary:"
    info "  ✅ Passed: ${PASSED}"
    if [ ${FAILED} -gt 0 ]; then
        error "  ❌ Failed: ${FAILED}"
        exit 1
    else
        info "  ❌ Failed: ${FAILED}"
    fi
    echo "======================================"

    info "🎉 All package tests passed successfully!"
}

# Run main function
main "$@"

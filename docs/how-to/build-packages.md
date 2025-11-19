# How to Build and Test Packages Locally

This guide walks you through building RPM and DEB packages locally and testing them with Docker.

## Why Build Packages Locally?

Building packages locally allows you to:

- Test package installation before creating a release
- Verify systemd service configuration
- Validate installation scripts (post-install, pre-remove, post-remove)
- Debug packaging issues on your development machine

## Prerequisites

### For Building Packages

- Go 1.21 or later
- [Task](https://taskfile.dev/) runner
- Ruby and Ruby development headers
- FPM gem
- RPM build tools (for RPM packages)
- GNU tar (on macOS)

### For Testing Packages

- Docker (for testing package installation)

## Installation

### macOS

```bash
# Install build dependencies
brew install go-task ruby rpm gnu-tar

# Install FPM
gem install fpm

# Add GNU tar to PATH (add to ~/.zshrc for persistence)
export PATH="/opt/homebrew/opt/gnu-tar/libexec/gnubin:$PATH"
```

### Linux (Ubuntu/Debian)

```bash
# Install build dependencies
sudo apt-get update
sudo apt-get install -y ruby ruby-dev build-essential rpm

# Install Task
sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b /usr/local/bin

# Install FPM
sudo gem install fpm
```

### Linux (RHEL/CentOS/Rocky)

```bash
# Install build dependencies
sudo yum install -y ruby ruby-devel gcc make rpm-build

# Install Task
sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b /usr/local/bin

# Install FPM
sudo gem install fpm
```

## Building Packages

### Build All Packages

Build all packages (2 RPM + 2 DEB) for both architectures:

```bash
# Set version (or use git tag)
export GITHUB_REF="refs/tags/v0.1.0"

# Build packages
task package-all
```

This creates:
- `.dist/relay-0.1.0-1.x86_64.rpm`
- `.dist/relay-0.1.0-1.aarch64.rpm`
- `.dist/relay_0.1.0_amd64.deb`
- `.dist/relay_0.1.0_arm64.deb`

### Build Specific Package Types

Build only RPM packages:

```bash
export GITHUB_REF="refs/tags/v0.1.0"
task package-rpm
```

Build only DEB packages:

```bash
export GITHUB_REF="refs/tags/v0.1.0"
task package-deb
```

Build for a specific architecture:

```bash
export GITHUB_REF="refs/tags/v0.1.0"
task package-rpm-amd64   # x86_64 RPM only
task package-deb-arm64   # ARM64 DEB only
```

## Testing Packages with Docker

The project includes a comprehensive test script that validates package installation across multiple distributions.

### Run Full Test Suite

```bash
./test-packages.sh
```

This tests packages on:
- Amazon Linux 2 (RPM)
- Rocky Linux 9 (RPM)
- Ubuntu 22.04 (DEB)
- Debian 12 (DEB)

The script validates:
- Package installs without errors
- Binary is installed and executable
- Systemd service file is present
- `relay` system user and group are created
- DLQ directories are created with correct permissions
- Binary executes and reports version

### Test on Specific Distribution

#### Test on Ubuntu

```bash
docker run -it --rm -v $(pwd)/.dist:/packages ubuntu:22.04 bash

# Inside container:
apt update
apt install -y /packages/relay_0.1.0_amd64.deb
relay --version
systemctl is-enabled relay.service
ls -la /var/spool/relay/
exit
```

#### Test on Amazon Linux 2

```bash
docker run -it --rm -v $(pwd)/.dist:/packages amazonlinux:2 bash

# Inside container:
yum install -y /packages/relay-0.1.0-1.x86_64.rpm
relay --version
systemctl is-enabled relay.service
ls -la /var/spool/relay/
exit
```

## Inspecting Package Contents

### Inspect RPM Package

List files in an RPM package:

```bash
rpm -qlp .dist/relay-0.1.0-1.x86_64.rpm
```

View RPM package metadata:

```bash
rpm -qip .dist/relay-0.1.0-1.x86_64.rpm
```

View installation scripts:

```bash
rpm -qp --scripts .dist/relay-0.1.0-1.x86_64.rpm
```

### Inspect DEB Package

List files in a DEB package:

```bash
dpkg-deb -c .dist/relay_0.1.0_amd64.deb
```

View DEB package metadata:

```bash
dpkg-deb -I .dist/relay_0.1.0_amd64.deb
```

Extract package contents without installing:

```bash
dpkg-deb -x .dist/relay_0.1.0_amd64.deb extracted/
ls -la extracted/
```

## Common Issues

### Issue: "fpm is not installed"

**Solution:** Install FPM:
```bash
gem install fpm
```

### Issue: "Need executable 'rpmbuild' to convert dir to rpm"

**Solution on macOS:**
```bash
brew install rpm
```

**Solution on Linux:**
```bash
# Debian/Ubuntu
sudo apt-get install rpm

# RHEL/CentOS
sudo yum install rpm-build
```

### Issue: "tar failed (exit code 1)" on macOS

**Problem:** macOS uses BSD tar, but FPM needs GNU tar.

**Solution:**
```bash
brew install gnu-tar
export PATH="/opt/homebrew/opt/gnu-tar/libexec/gnubin:$PATH"
```

Add the export to `~/.zshrc` to make it permanent.

### Issue: "package is intended for a different operating system" (RPM)

**Problem:** FPM detected macOS as the build OS.

**Solution:** This is already handled by the `--rpm-os linux` flag in Taskfile.yml. If you see this error, ensure you're using the latest Taskfile.

### Issue: Version contains dashes

**Problem:** DEB packages don't allow dashes in version numbers.

**Solution:** FPM automatically converts dashes to underscores. This is expected behaviour. Use semantic versions like `1.0.0` in git tags.

## Cleaning Up

Remove built packages:

```bash
task clean
```

This removes:
- `.build/` directory (compiled binaries)
- `.dist/` directory (packages)
- `.test/` directory (test artifacts)

## Testing in GitHub Actions

To test the packaging workflow without creating a release:

1. Go to **Actions** tab in GitHub
2. Select **Test Packaging** workflow
3. Click **Run workflow**
4. Specify a test version (e.g., `0.0.0-test`)
5. Download artifacts after workflow completes

This builds all packages in the GitHub Actions environment and uploads them as artifacts for download and testing.

## Next Steps

- Read [Package Installation Guide](../reference/installation.md) for end-user installation instructions
- Review [ADR-0017](../explanation/adr/0017-fpm-packaging.md) for packaging architecture decisions
- See [Release Process](../reference/release-process.md) for creating official releases


# ADR-0017: FPM for Package Distribution

## Status

Accepted

## Context

The relay service needs to be distributed as native OS packages (RPM and DEB) for easier deployment and management on Linux systems. Users need a standardised installation method that:

- Creates system users and groups automatically
- Installs systemd service files
- Sets up required directory structures
- Handles upgrades and uninstalls cleanly
- Works across multiple Linux distributions (RHEL/CentOS, Debian/Ubuntu, Amazon Linux, Rocky Linux)

Several packaging approaches were considered:

1. **Native RPM spec files and Debian control files**: Traditional approach requiring separate maintenance of two packaging systems with different syntaxes and tooling.

2. **Docker containers only**: Modern approach but requires users to manage container orchestration, volume mounts, and doesn't integrate with system package managers.

3. **Tarball with install script**: Simple but bypasses package managers, no dependency management, difficult to upgrade/uninstall cleanly.

4. **FPM (Effing Package Management)**: Tool that generates multiple package formats from a single definition, supports lifecycle scripts, and integrates with existing build systems.

The relay service already produces cross-compiled binaries for multiple platforms. The challenge is wrapping these binaries with appropriate package metadata and installation logic.

## Decision

We will use FPM to generate both RPM and DEB packages from our Go binaries. The packaging will:

- Use FPM's directory mode (`-s dir -t rpm/deb`) to package pre-built binaries
- Include lifecycle scripts (post-install, pre-remove, post-remove) for user creation and cleanup
- Install a systemd service file with security hardening
- Create the `relay` system user and group during installation
- Set up DLQ directory structure at `/var/spool/relay/dlq/`
- Build packages as part of the GitHub Actions release workflow
- Support both x86_64/amd64 and aarch64/arm64 architectures

Package builds are triggered only on git tags (releases), not on every CI run.

## Consequences

### Positive

- **Single source of truth**: One set of FPM commands generates both RPM and DEB packages
- **Cross-platform building**: Can build Linux packages from macOS during development (with caveats)
- **Familiar tooling**: Packages integrate with standard `yum`/`dnf`/`apt` workflows users expect
- **Automatic dependency management**: Package managers handle systemd and other dependencies
- **Clean upgrades**: Package managers handle file replacement and service restarts
- **Security**: Dedicated system user/group with restricted permissions
- **Minimal learning curve**: FPM syntax is simpler than native spec/control files

### Negative

- **Additional build dependency**: Requires Ruby and FPM to be installed in build environments
- **FPM quirks**: Need workarounds like `--rpm-os linux` when building on macOS
- **Less control**: FPM abstracts away some low-level package details
- **Version constraints**: FPM converts version strings (e.g., dashes to underscores in DEB)
- **Build time**: Package creation adds ~30 seconds to release workflow

### Neutral

- Packages are only built on releases (git tags), not on every commit
- Test script (`test-packages.sh`) requires Docker for validation
- Manual packaging test workflow available for pre-release validation
- RPM packages require GNU tar (not BSD tar) when building on macOS


# ADR-0006: Minimal External Dependencies

## Status

Accepted

## Context

Go projects must decide on their dependency management philosophy. Adding external dependencies can speed up development but introduces risks and complexity. For a security-sensitive log relay service, the dependency strategy is particularly important.

Considerations:
- Attack surface and supply chain security
- Maintenance burden
- Auditability
- Binary size
- Deployment simplicity

## Decision

We will minimize external dependencies and prefer the Go standard library wherever possible. External dependencies are only added when they provide significant value that would be costly to implement ourselves.

**Current approved dependencies:**
- `github.com/spf13/cobra` - Industry-standard CLI framework
- `gopkg.in/yaml.v3` - YAML configuration parsing

All other functionality uses the Go standard library.

## Consequences

### Positive

- **Reduced attack surface**: Fewer third-party code paths to secure
- **Easier security audits**: Less code to audit for vulnerabilities
- **No supply chain attacks**: Minimal exposure to compromised dependencies
- **Faster builds**: Fewer dependencies to download and compile
- **Smaller binaries**: Less code to include in final binary
- **Simpler deployment**: Fewer version conflicts and compatibility issues
- **Long-term stability**: Standard library has strong backward compatibility guarantees
- **Self-sufficient**: Can understand and modify all code without external docs

### Negative

- **More code to write**: Must implement functionality that libraries could provide
- **Reinventing wheels**: May implement sub-optimal versions of common patterns
- **Slower feature development**: Building from scratch takes longer
- **Missing optimisations**: Third-party libraries may have performance optimisations we lack

### Neutral

- Can add dependencies later if clear value justifies the cost
- Standard library is comprehensive and well-designed for server applications
- Many "convenience" libraries don't provide substantial value over stdlib

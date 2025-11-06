# ADR-0008: Internal Packages for Business Logic

## Status

Accepted

## Context

Go provides a special `internal/` directory mechanism that restricts package imports. Packages under `internal/` can only be imported by code in the parent tree. We need to decide how to structure our packages and what should be internal vs exported.

Options considered:
1. **Everything internal**: All business logic in `internal/`, only `cmd/` is public
2. **Everything public**: No `internal/`, all packages can be imported
3. **Mixed approach**: Some packages public (intended as library), some internal
4. **Flat structure**: All packages at root level

## Decision

We will place all implementation packages under `internal/` with only the `cmd/` directory and version information at the root level. This makes it clear that relay is an application, not a library.

Structure:
```
relay/
  cmd/relay/          # Application entry point
  internal/           # All implementation
    server/
    storage/
    forwarder/
    config/
    acl/
    processor/
    healthcheck/
  version.go          # Version info (exported for build injection)
```

## Consequences

### Positive

- **Clear intent**: Signals this is an application, not a reusable library
- **API freedom**: Can refactor internal packages without worrying about breaking external users
- **No accidental API**: Prevents unintentional API surface area
- **Encourages good design**: Forces us to think about public vs private APIs
- **Standard pattern**: Common pattern for Go applications
- **Clean root**: Root directory is uncluttered

### Negative

- **No code reuse**: Other projects can't import our packages (but that's intentional)
- **Testing complexity**: Tests must be in same package or use internal import paths
- **Vendoring difficulties**: Can't vendor internal packages easily (not a concern for applications)

### Neutral

- Can expose packages later if we decide to make them reusable libraries
- Internal packages can still import each other freely
- Standard Go tooling understands and respects internal packages

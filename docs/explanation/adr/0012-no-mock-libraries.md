# ADR-0012: No Third-Party Mocking Libraries

## Status

Accepted

## Context

Testing in Go often requires mocking dependencies like network connections, file systems, and external services. The Go ecosystem offers several mocking libraries (`gomock`, `mockery`, `testify/mock`), but hand-written mocks are also common.

This decision aligns with project conventions specified in CLAUDE.md:
> "Never use third-party mocking libraries"

Options considered:
1. **mockgen/gomock**: Code generation-based mocking
2. **testify/mock**: Manual mocking with assertion helpers
3. **mockery**: Another code generation tool
4. **Hand-rolled mocks**: Write mock implementations manually
5. **Interface-based testing**: Use real lightweight implementations

## Decision

We will not use third-party mocking libraries. Instead, we will:
- Hand-write mock implementations of interfaces when needed
- Use simple test doubles (e.g., `mockConn` for `net.Conn`)
- Prefer `httptest.Server` for HTTP client testing
- Use `t.TempDir()` for filesystem testing
- Keep mocks simple and focused on the test's needs

## Consequences

### Positive

- **Zero dependencies**: No additional test dependencies
- **Simple and clear**: Mock behaviour is explicit in test code
- **Full control**: Can customize mocks exactly as needed for each test
- **Easy to understand**: No magic code generation or reflection
- **No build steps**: No need to regenerate mocks after interface changes
- **Debuggable**: Can step through mock code in debugger
- **Follows project conventions**: Aligns with minimal dependency philosophy

### Negative

- **More code to write**: Each mock must be written manually
- **Maintenance burden**: Mocks need updating when interfaces change
- **Repetitive**: Similar mocks may be needed across different test files
- **No verification helpers**: Must manually verify mock calls and arguments
- **Potentially incomplete**: Easy to forget to implement all interface methods

### Neutral

- Go interfaces are typically small, making manual mocks practical
- Test-specific mocks can be simpler than general-purpose mocks
- Mocks can live alongside tests for easy maintenance
- Many successful Go projects use hand-written mocks (Kubernetes, Docker, etc.)

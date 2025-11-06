# ADR-0003: No context.Context in Long-Running Server

## Status

Accepted

## Context

Go's `context.Context` is commonly used for request-scoped operations, cancellation, and passing request-scoped values. The question arose whether the relay server should use context throughout its implementation.

Considerations:
- The server is designed to run indefinitely
- Connections are long-lived (streaming logs)
- Context is typically used for request-scoped operations with timeouts
- Adding context everywhere increases complexity

## Decision

We will not use `context.Context` for the server's primary operation. The server lifecycle is managed through:
- Direct listener control (listener.Accept/Close)
- Channel-based shutdown signalling (if graceful shutdown is implemented)
- Connection tracking with sync.WaitGroup

Context may be used in specific areas where it provides clear value:
- HTTP client requests (HEC forwarding) where stdlib expects it
- Graceful shutdown with timeout
- Future background workers with cancellation needs

## Consequences

### Positive

- **Simpler code**: Fewer function signatures with context parameters
- **Clearer intent**: Shutdown mechanism is explicit rather than through context cancellation
- **Less boilerplate**: Don't need to thread context through every function
- **Appropriate for use case**: Long-running servers don't benefit from request-scoped context

### Negative

- **May need refactoring later**: If we add features requiring context (distributed tracing, advanced cancellation), we'll need to retrofit
- **Inconsistent with some Go patterns**: Some in the Go community advocate for context everywhere
- **Limited cancellation propagation**: Can't easily cancel operations through a context tree

### Neutral

- Can add context later where needed without breaking existing functionality
- Context-free design is common in long-running services
- HTTP client already supports context.Context where the stdlib requires it

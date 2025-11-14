# ADR-0015: Configuration Reload via SIGHUP

## Status

Accepted

## Context

The relay service requires periodic operational changes without service interruption:

1. **Token Rotation**: Security policies require rotating Splunk HEC tokens every 90 days, but restarting relay causes connection drops and potential log loss
2. **ACL Updates**: Network changes require updating allowed IP ranges, but restart interrupts active ZPA LSS connections
3. **Configuration Tuning**: Operational adjustments (gzip, sourcetype) should not require downtime
4. **High Availability Requirements**: Service restarts create gaps in log collection that violate SLA requirements
5. **24/7 Operations**: Production environments cannot schedule downtime for configuration changes

The core challenge: How to update runtime configuration without:
- Dropping active TCP connections from ZPA LSS
- Losing in-flight log data
- Interrupting log forwarding to Splunk HEC
- Requiring maintenance windows

Options considered:

1. **File Watching**: Automatically reload configuration when file changes
   - Pro: No manual intervention required
   - Pro: Works well with configuration management tools
   - Con: Accidental file saves trigger unintended reloads
   - Con: Race conditions with concurrent edits
   - Con: Difficult to validate before applying
   - Con: No explicit operator control

2. **HTTP API Endpoint**: Provide REST API for configuration updates
   - Pro: Structured validation and responses
   - Pro: Programmatic access for automation
   - Pro: Can return detailed error messages
   - Con: Adds attack surface (requires authentication)
   - Con: Requires HTTP server infrastructure
   - Con: Violates minimal dependencies principle (ADR-0006)
   - Con: Configuration drift between file and running config

3. **SIGHUP Signal**: Unix signal-based reload (traditional daemon approach)
   - Pro: Standard Unix practice (nginx, PostgreSQL, etc.)
   - Pro: No additional dependencies
   - Pro: Explicit operator control
   - Pro: Configuration remains in file (single source of truth)
   - Con: Unix-specific (not Windows-native)
   - Con: Requires process management knowledge

4. **Automatic Reload on Connection**: Reload config per-connection
   - Pro: Always uses latest configuration
   - Con: Significant performance overhead
   - Con: Connection-level inconsistency
   - Con: Complex state management

5. **No Reload Support**: Require service restart for all changes
   - Pro: Simplest implementation
   - Con: Violates HA requirements
   - Con: Causes log collection gaps
   - Con: Poor operational experience

## Decision

We implement configuration reload via SIGHUP signal with the following design:

### 1. Signal Handler

Register SIGHUP alongside SIGTERM/SIGINT in the main event loop:

```go
signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

for {
    select {
    case sig := <-sigCh:
        switch sig {
        case syscall.SIGHUP:
            reloadConfig(...)
        case syscall.SIGINT, syscall.SIGTERM:
            shutdown(...)
        }
    }
}
```

This keeps reload logic in the same control flow as shutdown for consistency.

### 2. Reloadable vs Non-Reloadable Parameters

**Reloadable (safe at runtime):**
- HEC token (`hec_token`) - Pure configuration data
- HEC sourcetype (`source_type`) - Forwarding metadata
- HEC gzip (`gzip`) - Compression setting
- ACL CIDRs (`allowed_cidrs`) - Access control rules

**Non-reloadable (requires restart):**
- Listen address (`listen_addr`) - Requires new TCP listener
- TLS certificate/key (`tls.*`) - Loaded during listener setup
- Output directory (`output_dir`) - May break in-flight file writes
- Max line bytes (`max_line_bytes`) - Affects active connection buffers
- Batch configuration (`batch.*`) - Requires forwarder recreation
- Circuit breaker config (`circuit_breaker.*`) - Requires state machine restart
- Listener count/names - Structural change requiring initialization

This distinction is based on:
- Thread safety (can we update without race conditions?)
- State consistency (does it affect active connections?)
- Resource management (does it require resource recreation?)

### 3. Thread-Safe Configuration Updates

Use read-write mutex (RWMutex) for configuration access:

**Forwarder Package:**
```go
type HEC struct {
    config   Config
    configMu sync.RWMutex  // Protects reloadable fields
}

func (h *HEC) Forward(...) {
    h.configMu.RLock()
    token := h.config.Token
    useGzip := h.config.UseGzip
    h.configMu.RUnlock()
    // Use local copies for consistency
}

func (h *HEC) UpdateConfig(cfg ReloadableConfig) {
    h.configMu.Lock()
    defer h.configMu.Unlock()
    h.config.Token = cfg.Token
    h.config.UseGzip = cfg.UseGzip
}
```

**Server Package:**
```go
type Server struct {
    acl   *acl.List
    aclMu sync.RWMutex  // Protects ACL for reloads
}

func (s *Server) UpdateConfig(cfg ReloadableConfig) error {
    if cfg.AllowedCIDRs != "" {
        newACL, err := acl.New(cfg.AllowedCIDRs)
        if err != nil {
            return err
        }
        s.aclMu.Lock()
        s.acl = newACL
        s.aclMu.Unlock()
    }
}
```

RWMutex allows multiple concurrent readers (hot path) while serializing writers (rare path).

### 4. Validation Before Application

Reload process validates new configuration completely before applying:

1. Load configuration file
2. Parse YAML
3. Validate syntax and structure
4. Verify listener count/names match (structural check)
5. Verify non-reloadable parameters unchanged
6. Validate reloadable parameter values (CIDR format, etc.)
7. Only if all validation passes: apply atomically

If validation fails at any step, old configuration remains active and error is logged.

### 5. Atomic Updates

Configuration updates are atomic per-listener:
- All validations pass before any updates
- Updates happen inside mutex-protected critical section
- Each listener updated completely or not at all
- If any listener fails to update, entire reload aborts

This prevents partial/inconsistent configuration states.

### 6. Forwarder Interface Extension

Add `UpdateConfig()` method to Forwarder interface:

```go
type ReloadableConfig struct {
    Token      string
    SourceType string
    UseGzip    bool
}

type Forwarder interface {
    Forward(connID string, data []byte) error
    HealthCheck() error
    Shutdown(ctx context.Context) error
    UpdateConfig(cfg ReloadableConfig)  // New method
}
```

Both HEC and MultiHEC implement this interface consistently.

### 7. Comprehensive Logging

Log all reload events with appropriate detail:

- SIGHUP received
- Configuration validation results
- Per-listener update results
- Final success/failure status
- Specific errors if validation fails

Do not log secrets (tokens) for security.

### 8. Fail-Safe Behaviour

On any error during reload:
- Log detailed error message
- Keep existing configuration active
- Service continues operating normally
- Operator can fix configuration and retry

Reload failures do not crash or disrupt the service.

## Consequences

### Positive

- **Zero-downtime updates**: Configuration changes without service interruption
- **Operational simplicity**: Standard Unix signal pattern familiar to operators
- **No new dependencies**: Uses standard library only (adheres to ADR-0006)
- **Explicit control**: Operator decides when to reload (predictable)
- **Single source of truth**: Configuration file remains authoritative
- **Thread-safe**: Mutex-protected updates prevent race conditions
- **Atomic updates**: All-or-nothing approach prevents inconsistent state
- **Fail-safe**: Errors during reload do not disrupt running service
- **Comprehensive validation**: Catches errors before application
- **Token rotation support**: Enables security best practices
- **Emergency ACL updates**: Quick response to security incidents
- **Backward compatible**: Existing code paths unaffected
- **Testable**: Clear interfaces make testing straightforward
- **Well-documented**: Operators understand what can/cannot be reloaded

### Negative

- **Platform limitation**: SIGHUP is Unix-specific (Windows uses different mechanisms)
- **Operator knowledge**: Requires understanding of signals and process management
- **Limited scope**: Only subset of parameters reloadable
- **Implementation complexity**: Mutex management and validation logic adds code
- **Testing burden**: Must test thread-safety, validation, and error paths
- **Partial solution**: Still requires restart for structural changes
- **Documentation burden**: Must clearly document reloadable vs non-reloadable
- **Error handling**: Must handle edge cases (invalid CIDR, file not found, etc.)
- **State management**: Must carefully manage which state can be updated

### Neutral

- SIGHUP is standard signal number 1 on Unix systems
- Multiple consecutive SIGHUP signals are handled independently
- Reload validation errors are logged but service continues
- Configuration file path is captured at startup (no dynamic path changes)
- Reload operation is synchronous (blocks until complete)
- Non-reloadable parameter changes cause reload to fail with clear error
- ACL updates take effect immediately for new connections
- HEC configuration updates affect new forward operations immediately
- Active connections continue with pre-reload configuration until completion
- Forwarder interface is extended but maintains backward compatibility
- MultiHEC applies updates to all targets uniformly
- Reload frequency is not rate-limited (operator responsibility)

## Related ADRs

- **ADR-0006**: Minimal External Dependencies - SIGHUP requires no dependencies
- **ADR-0010**: Optional HEC Forwarding - Reload works whether HEC enabled or not
- **ADR-0014**: Multi-Target HEC Support - Reload updates all HEC targets

## Implementation Notes

### Testing Strategy

1. **Unit tests**: Test UpdateConfig methods with various inputs
2. **Integration tests**: Test full reload flow with valid/invalid configurations
3. **Concurrency tests**: Verify thread-safety with concurrent reads/writes
4. **Validation tests**: Test all validation error paths
5. **Regression tests**: Ensure non-reloadable changes are rejected

### Migration Path

For operators upgrading to version with reload support:

1. No configuration changes required (backward compatible)
2. Operators can continue restarting for changes (still works)
3. Operators can adopt SIGHUP reload incrementally
4. Documentation explains which parameters are reloadable

### Windows Compatibility

While SIGHUP is Unix-specific, Windows support could be added via:
- Service control commands (SCM)
- Named pipes or event objects
- Console control handlers

This is left for future enhancement if Windows support is required.

### Security Considerations

1. **File permissions**: Configuration file should be readable only by relay process user
2. **Token visibility**: Reload logs do not include token values
3. **Audit trail**: All reload attempts logged for security audit
4. **Validation**: Prevents loading malformed configurations
5. **No privilege escalation**: Reload uses same permissions as running process

### Performance Impact

- **Read path (hot)**: RLock adds minimal overhead (~20ns per acquire)
- **Write path (cold)**: Lock contention rare (reload is infrequent)
- **Validation**: One-time cost during reload (seconds)
- **Memory**: Small increase for mutex overhead per structure

Performance impact is negligible given reload frequency.

## Future Considerations

Potential enhancements not included in this decision:

1. **Extended reloadable parameters**: Could make more parameters reloadable with careful design:
   - Batch configuration (requires forwarder recreation)
   - Circuit breaker settings (requires state machine update)
   - Output directory (requires careful file handle management)

2. **Hot TLS certificate reload**: Could reload TLS certs without restart:
   - Requires listener recreation or certificate hot-swap
   - More complex but enables automated cert renewal (Let's Encrypt)

3. **Configuration validation endpoint**: HTTP endpoint to validate config without applying:
   - Useful for CI/CD pipelines
   - Requires HTTP server (conflicts with minimal dependencies)

4. **Automatic reload on file change**: Watch configuration file:
   - Convenient but loses explicit operator control
   - See rejected option in Context section

5. **Partial reload**: Reload specific listeners instead of all:
   - More granular control
   - More complex interface

6. **Rollback mechanism**: Automatically revert on failed health check:
   - More sophisticated error handling
   - Requires health check after reload

These can be added later if operational experience shows they're valuable, without breaking the current design.

## References

- SIGHUP signal: `man 7 signal` on Unix systems
- Go sync package: https://pkg.go.dev/sync
- Similar implementations: nginx (`nginx -s reload`), PostgreSQL (`pg_ctl reload`)

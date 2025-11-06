# ADR-0009: Separate Healthcheck Port

## Status

Accepted

## Context

Container orchestration platforms (Kubernetes, Docker Swarm, ECS) need to perform health checks to determine if a container is alive and ready to receive traffic. We need to provide a health check mechanism.

Options considered:
1. **HTTP endpoint on data port**: Add HTTP handling to the TCP server
2. **Separate HTTP health port**: Dedicated port (e.g., :9016) for health checks
3. **TCP handshake on data port**: Check if port accepts connections
4. **No health check**: Rely on process liveness only
5. **File-based**: Write a file that orchestrator checks

## Decision

We will provide a separate TCP port (default :9016) dedicated to health checks. The health check server accepts TCP connections and immediately closes them. This simple handshake indicates the service is alive.

## Consequences

### Positive

- **No interference**: Health checks don't interfere with data connections on main port
- **Simple implementation**: Just accept and close connections, no protocol needed
- **Fast checks**: TCP handshake is very fast (microseconds)
- **Firewall friendly**: Can expose health port to orchestrator without exposing data port
- **Clear separation**: Data port and health port have distinct purposes
- **Works with any orchestrator**: TCP checks supported by all platforms

### Negative

- **Extra port**: Requires opening and managing an additional port
- **Not HTTP**: Some tools prefer HTTP health endpoints with status codes
- **Limited information**: Can't return detailed health status or metrics
- **Port management**: Need to ensure health port doesn't conflict with other services

### Neutral

- TCP health checks are lightweight and sufficient for liveness probes
- Can add HTTP health endpoint later if needed for readiness checks
- Most orchestrators support both TCP and HTTP health checks

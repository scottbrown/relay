# ADR-0010: Optional HEC Forwarding

## Status

Accepted

## Context

The relay service is designed to forward logs to Splunk HEC, but developers and some deployment scenarios may not always have a Splunk instance available or may not want forwarding enabled.

Considerations:
- Development and testing without Splunk
- Disaster recovery scenarios where only local storage is needed
- Environments where HEC is temporarily unavailable
- Use as a pure storage/collection service

## Decision

HEC forwarding is optional. If HEC URL or token is not configured, the relay operates in storage-only mode. The service:
- Always stores logs locally (as per ADR-0005)
- Only attempts HEC forwarding if both URL and token are provided
- Logs a message indicating HEC forwarding is disabled
- Continues normal operation without HEC

## Consequences

### Positive

- **Local development**: Developers can test without setting up Splunk
- **Flexible deployment**: Can deploy as pure log collector without HEC
- **Graceful degradation**: Service continues if HEC configuration is missing
- **Disaster recovery**: Can operate in store-only mode during HEC outages
- **Testing**: Easier to test storage functionality independently
- **Simplified setup**: Don't need HEC for initial testing

### Negative

- **Silent misconfiguration**: Typos in config could disable forwarding without obvious errors
- **Split brain**: Service might run successfully but not forward, causing data gaps in Splunk
- **Missing data detection**: Need monitoring to detect when HEC forwarding isn't happening

### Neutral

- Log message clearly indicates when HEC is disabled
- Smoke-test subcommand can verify HEC configuration
- Local files can always be replayed to HEC later
- Aligns with "storage first" philosophy (ADR-0005)

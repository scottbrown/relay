# Architecture Decision Records

This directory contains Architecture Decision Records (ADRs) for the relay project.

An Architecture Decision Record captures an important architectural decision made along with its context and consequences. This helps current and future team members understand why certain choices were made.

## Format

We follow the format described by Michael Nygard in his article [Documenting Architecture Decisions](http://thinkrelevance.com/blog/2011/11/15/documenting-architecture-decisions):

- **Title**: Short noun phrase
- **Status**: Proposed, Accepted, Deprecated, or Superseded
- **Context**: The issue motivating this decision
- **Decision**: The change being proposed or made
- **Consequences**: The results of applying the decision (positive, negative, and neutral)

## Index

| ADR | Title | Status |
|-----|-------|--------|
| [0001](0001-use-go-task.md) | Use Go Task instead of Make | Accepted |
| [0002](0002-daily-utc-rotation.md) | Daily Log Rotation Based on UTC | Accepted |
| [0003](0003-no-context-in-server.md) | No context.Context in Long-Running Server | Accepted |
| [0004](0004-ndjson-storage.md) | NDJSON for Local Storage | Accepted |
| [0005](0005-store-first-forward-second.md) | Store First, Forward Second | Accepted |
| [0006](0006-minimal-dependencies.md) | Minimal External Dependencies | Accepted |
| [0007](0007-cobra-cli.md) | Cobra for CLI Framework | Accepted |
| [0008](0008-internal-packages.md) | Internal Packages for Business Logic | Accepted |
| [0009](0009-separate-healthcheck-port.md) | Separate Healthcheck Port | Accepted |
| [0010](0010-optional-hec.md) | Optional HEC Forwarding | Accepted |
| [0011](0011-utc-timestamps.md) | UTC Timestamps Everywhere | Accepted |
| [0012](0012-no-mock-libraries.md) | No Third-Party Mocking Libraries | Accepted |

## Creating New ADRs

When making a significant architectural decision:

1. Copy `template.md` to `NNNN-short-title.md` (increment the number)
2. Fill in the sections with your decision and reasoning
3. Update this index with the new ADR
4. Submit as part of your pull request

## What Deserves an ADR?

Not every decision needs an ADR. Good candidates include:

- **Technology choices**: Frameworks, libraries, languages
- **Architectural patterns**: How components interact, data flow
- **Data formats**: File formats, protocols, serialisation
- **Operational decisions**: Deployment strategies, monitoring approaches
- **Significant trade-offs**: When multiple viable options exist

## References

- [Michael Nygard's ADR article](http://thinkrelevance.com/blog/2011/11/15/documenting-architecture-decisions)
- [ADR GitHub organisation](https://adr.github.io/)
- [Sustainable Architectural Design Decisions](https://www.infoq.com/articles/sustainable-architectural-design-decisions/)

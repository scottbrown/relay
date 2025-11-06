# ADR-0007: Cobra for CLI Framework

## Status

Accepted

## Context

The relay application needs a command-line interface for configuration, subcommands (like `template` and `smoke-test`), and flag handling. While the Go standard library provides `flag` package, more sophisticated CLI frameworks offer better user experience and maintainability.

Options considered:
1. **Standard library `flag` package**: Simple but limited
2. **Cobra** (`github.com/spf13/cobra`): Full-featured CLI framework
3. **urfave/cli**: Alternative CLI framework
4. **Custom implementation**: Build our own on top of `flag`

## Decision

We will use Cobra for the command-line interface. Despite our minimal dependency philosophy (ADR-0006), Cobra provides sufficient value to justify the dependency.

## Consequences

### Positive

- **Industry standard**: Cobra is used by kubectl, Hugo, GitHub CLI, and many other major Go projects
- **Subcommand support**: Natural hierarchy for commands (`relay`, `relay template`, `relay smoke-test`)
- **Rich flag handling**: Supports persistent flags, flag inheritance, and complex flag types
- **Auto-generated help**: Automatic help text generation and formatting
- **Shell completion**: Built-in support for bash, zsh, fish, and PowerShell completion
- **Well-maintained**: Actively maintained by the Go community
- **Excellent documentation**: Comprehensive docs and examples
- **Testing support**: Easy to test CLI commands

### Negative

- **External dependency**: Violates minimal dependency principle (but justified by value)
- **Learning curve**: Team members need to learn Cobra patterns
- **Overkill for simple CLIs**: More complex than needed for basic flag parsing
- **Binary size**: Adds ~1-2MB to binary size

### Neutral

- Cobra's popularity means most Go developers are already familiar with it
- Aligns with user expectations (similar to kubectl, docker, etc.)
- Can replace with stdlib `flag` later if needed (though unlikely)

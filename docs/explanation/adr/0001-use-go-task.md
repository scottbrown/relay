# ADR-0001: Use Go Task instead of Make

## Status

Accepted

## Context

The project needed a build automation tool to handle common development tasks like building, testing, and releasing the application. The traditional choice for Go projects has been Makefiles, but alternative tools have emerged that are better suited for Go projects and provide cross-platform compatibility.

Key considerations:
- Cross-platform compatibility (Linux, macOS, Windows)
- Ease of use and readability
- Go ecosystem integration
- Maintainability

## Decision

We will use Go Task (go-task.github.io/task) with a `Taskfile.yml` instead of Make with a `Makefile`.

## Consequences

### Positive

- **Cross-platform**: Task works natively on Windows without requiring Make installation
- **YAML-based**: More readable and easier to maintain than Makefile syntax
- **Go-friendly**: Designed specifically for Go projects, understands Go conventions
- **Built-in features**: Includes features like dependency management, file watching, and variable interpolation
- **Better error messages**: More helpful error messages than Make
- **No shell required**: Can run commands without relying on specific shell features

### Negative

- **Additional tool**: Team members need to install Task (`go install github.com/go-task/task/v3/cmd/task@latest`)
- **Less universal**: Make is more widely known and installed by default on most Unix systems
- **Smaller community**: Task has a smaller community compared to Make

### Neutral

- Task and Make can coexist if needed (some projects provide both)
- Migration from Make to Task is straightforward for simple build scripts

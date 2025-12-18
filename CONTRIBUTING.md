# Contributing to warp

Thank you for your interest in contributing to warp. This document provides guidelines and instructions for contributing to the project.

## Code of Conduct

- Be respectful and constructive in all interactions
- Focus on the technical merits of contributions
- Welcome newcomers and help them get started

## Getting Started

### Prerequisites

- Go 1.21 or higher
- Git
- Basic understanding of Go, HTTP, and networking concepts

### Development Setup

1. Fork and clone the repository:
```bash
git clone https://github.com/zulfikawr/warp.git
cd warp
```

2. Install dependencies:
```bash
go mod download
```

3. Build the project:
```bash
go build -o warp ./cmd/warp
```

4. Run tests:
```bash
go test ./...
```

## Development Workflow

### Branch Strategy

- `main` - stable production branch
- Feature branches - `feature/your-feature-name`
- Bug fixes - `fix/issue-description`

### Making Changes

1. Create a new branch from `main`:
```bash
git checkout -b feature/your-feature-name
```

2. Make your changes following the coding standards below

3. Add tests for new functionality

4. Run the test suite:
```bash
go test ./... -v
```

5. Run linters (if available):
```bash
golangci-lint run
```

6. Commit your changes with clear commit messages:
```bash
git commit -m "feat: add new feature description"
```

### Commit Message Format

Follow conventional commits specification:

- `feat:` - new feature
- `fix:` - bug fix
- `docs:` - documentation changes
- `test:` - test additions or modifications
- `refactor:` - code refactoring
- `perf:` - performance improvements
- `chore:` - maintenance tasks

Example:
```
feat: add parallel chunk upload support

- Implement session-based upload management
- Add configurable worker count
- Include progress tracking via WebSocket
```

### Pull Request Process

1. Update documentation for any user-facing changes

2. Ensure all tests pass and code coverage is maintained

3. Push your branch and create a pull request:
```bash
git push origin feature/your-feature-name
```

4. Fill out the pull request template with:
   - Description of changes
   - Related issue numbers
   - Testing performed
   - Screenshots (if UI changes)

5. Address review feedback promptly

6. Once approved, a maintainer will merge your PR

## Coding Standards

### Go Style

Follow standard Go conventions:

- Use `gofmt` for formatting
- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Use meaningful variable and function names
- Keep functions focused and concise
- Add comments for exported functions and types

### Code Organization

```
cmd/warp/          - CLI entry point
internal/          - Internal packages
  client/          - Download client implementation
  server/          - HTTP server and handlers
  crypto/          - Encryption functionality
  discovery/       - mDNS/DNS-SD discovery
  network/         - Network utilities
  protocol/        - Protocol handshake
  ui/              - Terminal UI components
  config/          - Configuration management
  metrics/         - Prometheus metrics
  logging/         - Structured logging
test/              - End-to-end tests
```

### Testing Requirements

- Write unit tests for new functions
- Maintain or improve code coverage
- Include table-driven tests where appropriate
- Add integration tests for new features
- Test error cases and edge conditions

Example test structure:
```go
func TestFunctionName(t *testing.T) {
    tests := []struct {
        name    string
        input   interface{}
        want    interface{}
        wantErr bool
    }{
        // test cases
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test implementation
        })
    }
}
```

### Error Handling

- Return errors explicitly
- Wrap errors with context: `fmt.Errorf("operation failed: %w", err)`
- Handle errors at appropriate levels
- Use structured logging for error context

### Documentation

- Add godoc comments for exported types and functions
- Update README.md for user-facing changes
- Include examples in documentation
- Document configuration options

## Reporting Issues

### Bug Reports

Include the following information:

- warp version (`warp version` or commit hash)
- Operating system and version
- Go version
- Steps to reproduce
- Expected behavior
- Actual behavior
- Relevant logs or error messages

### Feature Requests

Provide:

- Clear description of the feature
- Use case and motivation
- Proposed implementation approach (optional)
- Alternatives considered

## Project Structure

### Key Components

- **Server** - HTTP server with upload/download handlers
- **Client** - Download client with parallel chunk support
- **Discovery** - mDNS/DNS-SD for local network discovery
- **Crypto** - AES-256-GCM encryption with PBKDF2
- **Metrics** - Prometheus metrics export
- **UI** - Terminal progress bars and QR codes

### Adding New Features

1. Discuss major changes in an issue first
2. Implement with minimal dependencies
3. Follow existing patterns and architecture
4. Add comprehensive tests
5. Update documentation
6. Consider backward compatibility

## Performance Considerations

- Profile code for performance bottlenecks
- Use streaming for large file operations
- Implement proper cleanup and resource management
- Consider memory usage for large transfers
- Test with various network conditions

## Security

- Never commit sensitive information (keys, passwords, tokens)
- Follow secure coding practices
- Validate all user inputs
- Use constant-time comparisons for secrets
- Report security issues privately to maintainers

## Getting Help

- Check existing issues and documentation
- Ask questions in issue discussions
- Review closed PRs for similar changes

## License

By contributing, you agree that your contributions will be licensed under the MIT License.

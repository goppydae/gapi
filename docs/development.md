# Development Guide

This document explains how to build, test, and contribute to GAPI.

## Prerequisites

### Recommended: Nix

The easiest way to get started is with [Nix](https://nixos.org/):

```bash
nix develop
```

This provides all dependencies including:
- Go 1.25+
- Python 3.11+
- GCC (for CGO)
- Mage build tool
- Development tools (linters, formatters)

### Alternative: Manual Setup

If not using Nix, install:
- **Go 1.25+**: [golang.org/dl](https://golang.org/dl/)
- **Python 3.11+**: [python.org](https://www.python.org/)
- **GCC**: For CGO compilation
- **Mage**: `go install github.com/magefile/mage@latest`
- **Gopy**: `go build -mod=vendor -o $GOBIN/gopy github.com/go-python/gopy` (Must use vendored dependencies)

## Building

### Using Nix + Mage (Recommended)

```bash
nix develop -c mage build
```

This builds both `bin/gapid` and `bin/gapictl`.

### Using Go Directly

```bash
go build -o bin/gapid ./cmd/gapid
go build -o bin/gapictl ./cmd/gapictl
```

### Development Build

For faster iteration with debug symbols:

```bash
nix develop -c mage dev
```

## Running

### Start the Supervisor

```bash
./bin/gapid
```

By default, `gapid`:
- Scans `./agents/` for agent files
- Listens on `127.0.0.1:4242` (QUIC)
- Loads `config.yaml` from the current directory

### Use the CLI
```bash
# Check status
./bin/gapictl agent status

# Lifecycle control
./bin/gapictl lifecycle start myagent
./bin/gapictl lifecycle stop myagent
./bin/gapictl lifecycle restart myagent

# Interactive TUI
./bin/gapictl tui
```

## Testing

### Run All Tests

```bash
nix develop -c mage test
```

This runs:
- Unit tests
- Integration tests
- Python ADK tests

### Run E2E Tests

```bash
nix develop -c mage testE2E
```

End-to-end tests start a real supervisor and test full workflows.

### Run Specific Tests

```bash
# Go tests
go test ./internal/lifecycle/...

# Python tests
cd adk/python
python -m pytest
```

### Test Coverage

```bash
go test -cover ./...
```

## Code Quality

### Format Code

```bash
nix develop -c mage fmt
```

This runs:
- `gofmt` on Go code
- `black` on Python code

### Lint Code

```bash
nix develop -c mage lint
```

This runs:
- `golangci-lint` for Go
- `pylint` for Python

### Tidy Dependencies

```bash
nix develop -c mage tidy
```

Runs `go mod tidy` to clean up dependencies.

### All-in-One

```bash
nix develop -c mage all
```

Runs: format → tidy → build → test

## Project Structure

```
gapi/
├── cmd/
│   ├── gapid/          # Supervisor daemon
│   │   ├── main.go
│   │   └── config/
│   └── gapictl/        # CLI tool
│       ├── gapictl.go
│       ├── lifecycle.go
│       ├── security.go
│       └── tui/        # Terminal UI
│
├── core/
│   ├── config/         # Configuration loading
│   ├── crypto/         # Ed25519 + BLAKE3
│   └── version/        # Version info
│
├── internal/
│   ├── agentmgr/       # Agent lifecycle management
│   ├── agentreg/       # Agent discovery and registry
│   ├── cgroups/        # Cgroups v2 resource limits
│   ├── eventbus/       # Event-driven communication
│   ├── lifecycle/      # State machine
│   ├── proto/          # Protobuf definitions
│   ├── scheduler/      # Timer scheduling
│   ├── socket/         # Socket activation
│   └── transport/      # QUIC/TCP transport
│
├── adk/
│   ├── go/             # Go ADK
│   │   └── agent/
│   └── python/         # Python ADK (gopy bindings)
│       ├── gapi/
│       └── tests/
│
├── agents/             # Example agents
│   ├── hello.py.service
│   ├── timer.py.timer
│   └── echo.py.socket
│
├── test/
│   ├── adk/            # ADK test framework
│   └── e2e/            # End-to-end tests
│
├── docs/               # Documentation
├── nix/                # Nix build configuration
├── Magefile.go         # Build tasks
├── flake.nix           # Nix flake
└── config.yaml         # Example configuration
```

## Architecture Overview

### Control Plane

- **`gapid`**: Supervisor daemon that manages agent lifecycles
- **`gapictl`**: CLI for controlling the supervisor
- **Event Bus**: Asynchronous communication between components

### Data Plane

- **Agents**: Python or Go programs managed by the supervisor
- **Transport**: QUIC or TCP for event bus communication
- **Cgroups**: Resource isolation and limits

### Key Components

1. **Agent Registry** (`agentreg`): Discovers and tracks agents
2. **Agent Manager** (`agentmgr`): Starts, stops, and monitors agents
3. **Lifecycle** (`lifecycle`): State machine for agent states
4. **Scheduler** (`scheduler`): Timer-based execution
5. **Socket Activation** (`socket`): On-demand agent startup
6. **Cgroups** (`cgroups`): Resource limits enforcement

## Debugging

### Enable Debug Logging

```bash
GAPI_LOG_LEVEL=debug ./bin/gapid
```

### Inspect Cgroups

```bash
# Find agent cgroup
systemd-cgls | grep gapi

# Check resource usage
cat /sys/fs/cgroup/user.slice/.../gapi-myagent/cpu.stat
cat /sys/fs/cgroup/user.slice/.../gapi-myagent/memory.current
```

### Event Bus Tracing

Enable event tracing:

```bash
GAPI_TRACE_EVENTS=true ./bin/gapid
```

This logs all events published and received.

### Attach Debugger

Using Delve:

```bash
dlv debug ./cmd/gapid
```

## Contributing

### Workflow

1. **Fork the repository**
2. **Create a feature branch**: `git checkout -b feature/my-feature`
3. **Make changes**
4. **Run tests**: `nix develop -c mage test`
5. **Format code**: `nix develop -c mage fmt`
6. **Commit**: `git commit -m "Add my feature"`
7. **Push**: `git push origin feature/my-feature`
8. **Open a pull request**

### Commit Messages

Follow conventional commits:

- `feat: Add socket activation support`
- `fix: Resolve memory leak in agent manager`
- `docs: Update configuration guide`
- `test: Add E2E tests for timers`
- `refactor: Simplify lifecycle state machine`

### Code Style

- **Go**: Follow [Effective Go](https://golang.org/doc/effective_go)
- **Python**: Follow [PEP 8](https://www.python.org/dev/peps/pep-0008/)
- **Comments**: Document public APIs and complex logic
- **Tests**: Write tests for new features and bug fixes

### Pull Request Checklist

- [ ] Tests pass (`mage test`)
- [ ] Code is formatted (`mage fmt`)
- [ ] Documentation updated (if applicable)
- [ ] Commit messages follow conventional commits
- [ ] No breaking changes (or documented in PR)

## Release Process

1. **Update version** in `core/version/version.go`
2. **Update CHANGELOG.md**
3. **Tag release**: `git tag v0.x.0`
4. **Push tag**: `git push origin v0.x.0`
5. **Build release binaries**: `nix develop -c mage release`
6. **Create GitHub release** with binaries

## Troubleshooting

### Build Failures

**CGO errors**:
```bash
# Ensure GCC is installed
gcc --version
```

**Nix errors**:
```bash
# Update flake
nix flake update

# Rebuild environment
nix develop --rebuild
```

### Test Failures

**E2E tests fail**:
- Ensure no other `gapid` instance is running
- Check port 4242 is available
- Verify cgroup delegation (for resource limit tests)

**Python tests fail**:
- Ensure Python 3.11+ is installed
- Install Python dependencies: `pip install -r adk/python/requirements.txt`

### Runtime Issues

**Agents not starting**:
- Check `ENABLED = True` in agent metadata
- Verify agent syntax: `python agents/myagent.py.service`
- Check supervisor logs for errors

**Cgroup errors**:
- Verify cgroup v2 is enabled: `mount | grep cgroup2`
- Check delegation: `cat /sys/fs/cgroup/user.slice/user-$(id -u).slice/user@$(id -u).service/cgroup.controllers`
- See [Features - Resource Limits](features.md#rootless-support)

**Transport errors**:
- Verify certificates exist (for QUIC remote connections) or enable anonymous localhost support
- Check firewall rules
- Ensure address is not already in use

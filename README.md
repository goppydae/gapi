# GAPI - Agent Supervision Framework

**GAPI** is a lightweight, event-driven supervision framework for managing distributed daemon (agent) lifecycles. Built with Go and Python, it provides systemd-like lifecycle control, resource limits, socket activation, and cryptographic integrity.

## ✨ Key Features

- 🔒 **Security**: Ed25519 signing + BLAKE3 hashing for agent integrity
- ⏰ **Timer Agents**: Systemd-style scheduling, cron expressions, named schedules
- 📊 **Resource Limits**: CPU and memory constraints via cgroups v2 (rootless supported)
- 🔌 **Socket Activation**: On-demand agent startup for TCP/UDP services
- 🐍 **Python ADK**: Zero-boilerplate agent development with native Go bindings

## 🚀 Quick Start

### Build

```bash
# Using Nix (recommended)
nix develop -c mage build

# Or with Go directly
go build -o bin/gapid ./cmd/gapid
go build -o bin/gapictl ./cmd/gapictl
```

### Run

```bash
# Start supervisor
./bin/gapid

# Check agent status
./bin/gapictl agent-status

# Lifecycle control
./bin/gapictl lifecycle start myagent
./bin/gapictl lifecycle stop myagent
./bin/gapictl lifecycle restart myagent
```

## 📝 Simple Agent Example

```python
# agents/hello.py.service
# ENABLED = True
# TYPE = service

def start():
    print("Hello from GAPI!")
    while True:
        time.sleep(60)
```

That's it! No classes, no inheritance, no boilerplate.

## 📚 Documentation

- **[Features](docs/features.md)** - Deep dive into security, timers, cgroups, socket activation, Python ADK
- **[Configuration](docs/configuration.md)** - YAML config, transport options, agent metadata
- **[Agent Examples](docs/agent-examples.md)** - Service, timer, socket, resource-limited agents
- **[Development](docs/development.md)** - Build, test, project structure, contributing
- **[Design Document](docs/gapi-design-document.md)** - Architecture and design philosophy
- **[Installation](docs/installation.md)** - Deployment and installation guide
- **[Lexicon](docs/lexicon.md)** - Terminology and concepts
- **[Lore](docs/lore.md)** - Project history and evolution

## 🛠️ Development

```bash
# Run all tasks (format, tidy, build, test)
nix develop -c mage all

# Run tests
nix develop -c mage test

# Run E2E tests
nix develop -c mage testE2E
```

See [Development Guide](docs/development.md) for details.

## 📄 License

MIT
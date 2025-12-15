# GAPI - Agent Supervision Framework

**GAPI** is a lightweight, event-driven supervision framework for managing distributed daemon (agent) lifecycles. Built with Go and Python, it provides systemd-like lifecycle control, resource limits, socket activation, and cryptographic integrity.

## ✨ Key Features

- 🔒 **Security**: Ed25519 signing + BLAKE3 hashing + source-to-binary verification
- ⏰ **Timer Agents**: Systemd-style scheduling, cron expressions, named schedules
- 📊 **Resource Limits**: CPU and memory constraints via cgroups v2 (rootless supported)
- 🔌 **Socket Activation**: On-demand agent startup for TCP/UDP services
- 🐍 **Dual ADK**: Write agents in Python or Go with identical behavior
- 🛠️ **Developer Tools**: Agent templates, watch mode, verification chain
- 🔄 **CI/CD Ready**: Automated cross-ADK testing in GitHub Actions

## 🚀 Quick Start

```bash
# Install
nix develop -c go build ./cmd/gapid
nix develop -c go build ./cmd/gapictl

# Create agent
gapictl agent new my_service

# Build
gapictl agent build agents/go/foundational/my_service

# Verify
gapictl agent verify agents/build/go/my_service

# Run
./bin/gapid
```

## 📝 Simple Example

```python
# agents/python/services/hello.py.service
ID = "hello"
TYPE = "service"

def start():
    print("Hello from GAPI!")
    while True:
        time.sleep(60)
```

That's it! No classes, no inheritance, no boilerplate.

## 📚 Documentation

- **[Getting Started](docs/getting-started.md)** - Installation and first agent
- **[Agent Development](agents/README.md)** - Complete guide to writing agents
- **[Python ADK](agents/python/README.md)** - Python agent development
- **[Go ADK](agents/go/README.md)** - Go agent development
- **[Features](docs/features.md)** - Security, timers, cgroups, socket activation
- **[Configuration](docs/configuration.md)** - YAML config, transport options
- **[Design Document](docs/gapi-design-document.md)** - Architecture and philosophy

## 🛠️ Development

```bash
# Run tests
nix develop -c mage test

# Run cross-ADK tests
nix develop -c go test ./test/adk/
```

See [agents/README.md](agents/README.md) for agent development guide.

## 📄 License

Mozilla Public License 2.0 (MPL-2.0)
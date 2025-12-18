# GAPI - Agent Runtime Library

**GAPI** is the core **runtime library** and SDK for the GoPPydae ecosystem. It provides the low-level mechanism for spawning, supervising, and securing agents on a single machine. It is designed to be embedded into larger orchestrators (like [Goblin](../goblin)).

> **Note**: This repository contains the GAPI **library**. For the production daemon, see [Goblin](https://github.com/goppydae/goblin).

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
# Install Reference Tools
nix develop -c mage build

# Create agent
gapictl agent new my_service

# Run with Reference Daemon (gapid)
# (Useful for local dev/testing without a cluster)
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

# GAPI - Agent Supervision Framework

**GAPI** is a lightweight, event-driven supervision framework for managing distributed daemon (agent) lifecycles in both local and clustered environments. Built with Go and Python, it provides zero-config startup, resource limits, and cryptographic integrity.

## Key Features

### 🔒 Security & Integrity
- **Ed25519 Signing**: Cryptographically sign agents to ensure code integrity
- **BLAKE3 Hashing**: Fast, secure content verification
- **Signature Enforcement**: Optional verification of agent authenticity at runtime

### ⏰ Timer Agents
- **Systemd-Style Scheduling**: Use familiar syntax (`OnUnitActiveSec=5s`, `OnBootSec=30s`)
- **Periodic Execution**: Run agents on intervals without external cron
- **Auto-Start**: Timer agents start automatically on discovery

### 📊 Resource Limits (Cgroups v2)
- **CPU Limits**: Restrict agent CPU usage (e.g., `CPU_LIMIT = 0.5` for 50%)
- **Memory Limits**: Enforce hard memory caps (e.g., `MEMORY_LIMIT = 100MB`)
- **Rootless**: Works in rootless environments with proper delegation

### 🔌 Socket Activation
- **Lazy Loading**: Agents start on first connection
- **TCP/UDP Support**: Listen on any address/port
- **Zero Downtime**: Seamless handoff from supervisor to agent

### 🐍 Python ADK
- **Native Bindings**: Direct Go ↔ Python communication via `gopy`
- **Zero Boilerplate**: Just write functions, no classes required
- **Self-Describing**: Agents expose metadata automatically

## Getting Started

### Prerequisites
- **Nix** (recommended) or Go 1.25+ with Python 3
- **GCC** (for CGO)

### Build

Using Nix (recommended):
```bash
nix develop -c mage build
```

Or with Go directly:
```bash
go build -o bin/gapid ./cmd/gapid
go build -o bin/gapictl ./cmd/gapictl
```

### Run

Start the supervisor:
```bash
./bin/gapid
```

List agents:
```bash
./bin/gapictl status
```

### Security Setup

Generate signing keys:
```bash
./bin/gapictl keygen mykey
```

Sign an agent:
```bash
./bin/gapictl sign agents/myagent.py.service mykey.key
```

Enable verification in `config.yaml`:
```yaml
security:
  verifyKey: mykey.pub
```

## Agent Examples

### Service Agent
```python
# agents/hello.py.service
# ENABLED = True
# TYPE = service

def start():
    print("Hello from service agent!")
    while True:
        time.sleep(60)
```

### Timer Agent
```python
# agents/backup.py.timer
# ENABLED = True
# TYPE = timer
# SCHEDULE = OnUnitActiveSec=5m

def start():
    print("Running backup...")
    # Backup logic here
```

Timers execute on a schedule, supporting multiple formats:

**Systemd-style** (interval-based):
```python
# SCHEDULE = OnUnitActiveSec=5s  # Every 5 seconds
# SCHEDULE = OnBootSec=1m        # 1 minute after boot
# SCHEDULE = OnStartupSec=30s    # 30 seconds after startup
```

**Cron expressions**:
```python
# SCHEDULE = */5 * * * *    # Every 5 minutes
# SCHEDULE = 0 */2 * * *    # Every 2 hours
# SCHEDULE = 0 9 * * 1-5    # 9 AM on weekdays
# SCHEDULE = 0 0 * * 0      # Midnight on Sundays
```

**Named schedules**:
```python
# SCHEDULE = @hourly    # Once per hour
# SCHEDULE = @daily     # Once per day at midnight
# SCHEDULE = @weekly    # Once per week on Sunday
# SCHEDULE = @monthly   # Once per month on the 1st
```

**Raw durations**:
```python
# SCHEDULE = 5s    # Every 5 seconds
# SCHEDULE = 1m    # Every minute
# SCHEDULE = 1h    # Every hour
```

### Socket-Activated Agent
```python
# agents/api.py.socket
# ENABLED = True
# TYPE = socket
# LISTEN_STREAM = 0.0.0.0:8080

def start():
    # Handle incoming connections
    pass
```

### Resource-Limited Agent
```python
# agents/worker.py.service
# ENABLED = True
# TYPE = service
# CPU_LIMIT = 0.5
# MEMORY_LIMIT = 512MB

def start():
    # CPU and memory constrained work
    pass
```

## Configuration

Create `config.yaml`:
```yaml
transport:
  type: quic
  address: 127.0.0.1:4242
  certFile: config/certs/server.crt
  keyFile: config/certs/server.key

security:
  verifyKey: path/to/public.key  # Optional
```

## Development

### Available Mage Tasks
```bash
nix develop -c mage -l
```

Common tasks:
- `mage build` - Build binaries
- `mage test` - Run all tests
- `mage testE2E` - Run E2E tests
- `mage fmt` - Format code
- `mage all` - Format, tidy, build, and test

### Project Structure
```
gapi/
├── cmd/
│   ├── gapid/          # Supervisor daemon
│   └── gapictl/        # CLI tool
├── core/
│   ├── config/         # Configuration
│   ├── crypto/         # Ed25519 + BLAKE3
│   └── version/        # Version info
├── internal/
│   ├── agentmgr/       # Agent management
│   ├── agentreg/       # Agent registry
│   ├── cgroups/        # Resource limits
│   ├── eventbus/       # Event system
│   └── lifecycle/      # State machine
├── adk/
│   ├── go/             # Go ADK
│   └── python/         # Python ADK
└── agents/             # Example agents
```

## Testing

Run all tests:
```bash
nix develop -c mage test
```

Run E2E tests:
```bash
nix develop -c mage testE2E
```

## Documentation

See `docs/` for detailed documentation:
- [Design Document](docs/gapi_design_document.md)
- [Lexicon](docs/lexicon.md)
- [Lore](docs/lore.md)

## License

MIT
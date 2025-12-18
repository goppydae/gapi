# GAPI Agents

This directory contains GAPI agents organized by language and type.

## Directory Structure

```
agents/
├── python/          # Python agents (primary ADK)
│   ├── services/    # Long-running services
│   ├── timers/      # Scheduled/periodic tasks
│   └── sockets/     # Socket-activated services
├── go/              # Go agents (foundational/high-performance)
│   ├── foundational/  # System boot, core services
│   └── coordination/  # Cluster coordination
├── plugins/         # Shared-object plugins (experimental)
└── build/           # Build artifacts (gitignored)
    ├── go/          # Compiled Go binaries
    └── plugins/     # Compiled plugin .so files
```

## Agent Naming Convention

All agents follow the **`name.lang.type`** naming pattern:

**Format**: `<name>.<language>.<type>`

**Examples**:

- `heartbeat.py.service` - Python service agent
- `my_timer.py.timer` - Python timer agent
- `api_server.py.socket` - Python socket agent
- `init.go.init` - Go init agent (PID 1)
- `cluster_join.go.service` - Go service agent

**Components**:

- `<name>`: Agent identifier (alphanumeric, underscores, hyphens)
- `<language>`: `py` (Python) or `go` (Go)
- `<type>`: `service`, `timer`, `socket`, `init`

**Why this convention?**

- **Self-documenting**: Filename reveals language and type
- **Discovery**: Easy to scan for specific agent types
- **Tooling**: Simple glob patterns (`*.py.service`, `*.go.timer`)
- **Consistency**: Uniform across all agents

## Agent Discovery

GAPI uses a systemd-style search path for agent discovery:

1. **Development**: `./agents/`, `$GAPI_DEV_AGENTS`
1. **User**: `~/.local/share/gapi/agents/`, `~/.gapi/agents/`
1. **System**: `/usr/lib/gapi/agents/`, `/usr/local/lib/gapi/agents/`

**First match wins**: Agents in higher priority paths override those in lower priority paths.

## Environment Variables

- `GAPI_AGENT_PATH`: Override entire search path (colon-separated)
- `GAPI_DEV_AGENTS`: Add development path (highest priority)
- `GAPI_SKIP_SYSTEM_AGENTS`: Skip system paths (testing)

## Quick Start

### Python Agent

```bash
# Create a service agent
cat > agents/python/services/my_service.py << 'EOF'
ID = "my_service"
TYPE = "service"
VERSION = "1.0.0"

def initialize():
    print("Initializing...")

def start():
    print("Service running...")
    import time
    while True:
        time.sleep(1)

def stop():
    print("Stopping...")
EOF

# Start gapid (discovers automatically)
gapid
```

### Go Agent

```bash
# Create a Go agent
mkdir -p agents/go/foundational/my_agent
cat > agents/go/foundational/my_agent/main.go << 'EOF'
package main

import (
    "encoding/json"
    "flag"
    "os"
)

var describe = flag.Bool("describe", false, "Print agent metadata")

func main() {
    flag.Parse()

    if *describe {
        metadata := map[string]interface{}{
            "describe": map[string]interface{}{
                "id":      "my_agent",
                "type":    "service",
                "version": "1.0.0",
            },
        }
        json.NewEncoder(os.Stdout).Encode(metadata)
        return
    }

    // Agent logic here
}
EOF

# Build the agent
gapictl agent build agents/go/foundational/my_agent/

# Start gapid (discovers from build/)
gapid
```

## See Also

- [Python ADK Guide](python/README.md)
- [Go ADK Guide](go/README.md)
- [Plugin Development](plugins/README.md)
- [GAPI Design Document](../docs/gapi-design-document.md)

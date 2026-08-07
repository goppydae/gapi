# GAPI Agents

This directory contains GAPI agents organized by language and type.

## Directory Structure

```
agents/
|-- python/          # Python agents (primary ADK)
|   |-- services/    # Long-running services
|   |-- timers/      # Scheduled/periodic tasks
|   `-- sockets/     # Socket-activated services
|-- *.go.<type>      # built Go agents (the deploy payload)
|-- *.py.<type>      # Python agents (source IS the artifact)
|   `-- coordination/  # Cluster coordination
|-- plugins/         # Shared-object plugins (experimental)
`-- build/           # Build artifacts (gitignored)
    |-- go/          # Compiled Go binaries
    `-- plugins/     # Compiled plugin .so files
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

The ordering rule, shared by both scopes: **configuration beats runtime
beats data beats vendor**. An operator's edit outranks a package's file.

**System scope** (the daemon an init system starts), highest first:

1. `$GAPI_DEV_AGENTS` - explicit development override
1. `/etc/gapi/agents/` - operator-authored
1. `/run/gapi/agents/` - transient, generated at runtime
1. `/usr/local/lib/gapi/agents/` - locally installed
1. `/usr/lib/gapi/agents/` - package-owned

**User scope** (`--user`; the tier list is defined, the flag is not yet
implemented), highest first:

1. `$GAPI_DEV_AGENTS`
1. `$XDG_CONFIG_HOME/gapi/agents/` (`~/.config/gapi/agents/`)
1. `/etc/gapi/user/agents/` - operator-provided, for all users
1. `$XDG_RUNTIME_DIR/gapi/agents/`
1. `$XDG_DATA_HOME/gapi/agents/` (`~/.local/share/gapi/agents/`)
1. `~/.gapi/agents/` - legacy, lowest user tier
1. `/usr/lib/gapi/user/agents/` - package-owned user agents

**First match wins**: an agent ID found in a higher-priority path masks
the same ID found lower down. That masking is the override mechanism.

**Scope is chosen, never inferred from privilege.** A system daemon
commonly runs as an unprivileged service user, so deriving scope from
uid would silently flip it into user scope. System scope contains no
home-directory path at all, which is a security boundary and not tidiness.

**There is no implicit `./agents` tier.** It made discovery depend on the
directory a daemon happened to be started from. Name a development
directory explicitly with `GAPI_DEV_AGENTS`.

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
gapictl agent new --lang go --type service my_agent
# writes src/agents/my_agent.go.service - a single file, no main:
#
#   package agent
#
#   import "context"
#
#   const (
#       ID   = "my_agent"
#       Type = "service"
#   )
#
#   func Start(ctx context.Context) error {
#       <-ctx.Done()
#       return nil
#   }

gapictl agent build src/agents/my_agent.go.service

# Start gapid (discovers from build/)
gapid
```

## See Also

- [Python ADK Guide](python/README.md)
- [Go ADK Guide](go/README.md)
- [Plugin Development](plugins/README.md)
- GAPI's architecture, in the goppydae-docs repository

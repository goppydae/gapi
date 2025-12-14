# gapi

*GoPPydae Agent and Process Interface*

---

**GAPI** is a high-performance, event-driven supervisor designed to orchestrate agentic workflows with precision and integrity. It combines the robust process management of a systemd-like architecture with the flexibility of modern agent frameworks.

## Key Features

### 🚀 Advanced Lifecycle Management
- **State Machine Driven**: Deterministic transitions (Starting -> Running -> Stopping -> Stopped).
- **Dependency Graph**: Agents can declare `REQUIRES`, `WANTS`, `WANTED_BY`, etc. GAPI ensures correct startup order and shutdown cascades.
- **Lazy Activation**: Support for Socket Activation. Agents can stay stopped until traffic arrives at their socket.

### 🛡️ Resource Isolation
- **Rootless Cgroups v2**: Enforce CPU and Memory limits without needing root privileges.
- **Strict Boundaries**: Processes are isolated in dedicated cgroups (e.g., `gapid-infra/gapid-agent-name`).
- **OOM Protection**: Hard memory limits ensure runaway agents are killed instantly.

### 🔌 Extensible ADK
- **Python Agents**: Native Python ADK for writing agents with minimal boilerplate.
- **Event Bus**: Structured Protobuf messaging for inter-agent and system communication.
- **Metadata Config**: Agents define their own config (Limits, Dependencies, Descriptions) directly in code.

## Getting Started

### Prerequisites
- Linux with Cgroups v2 enabled
- Go 1.23+
- Python 3.12+

### Build
```bash
# Build the daemon and CLI
go run github.com/magefile/mage@latest buildall
```

### Run
```bash
# Start the supervisor
./bin/gapid
```

### CLI
```bash
# Check agent status
./bin/gapictl agent-status

# View dependency tree
./bin/gapictl agent-status --tree

# Control lifecycle
./bin/gapictl lifecycle start <agent_id>
./bin/gapictl lifecycle stop <agent_id>
```

## Agent Example

Agents are simple Python scripts with metadata headers:

```python
# agents/my_agent.py.service

ID = "my_agent"
DESCRIPTION = "Example Agent"
ENABLED = True
REQUIRES = ["database", "auth_service"]
MEMORY_LIMIT = "512MB"
CPU_LIMIT = "0.5"

import time
from gapi.adk import Agent

def start():
    print("Agent starting...")
    while True:
        # Do work
        time.sleep(1)
```

## Project Structure
```bash
gapi
├── adk                - Agent Development Kits (Python)
├── agents             - Example agents
├── cmd                - Binaries (gapid, gapictl)
├── core               - Core libraries (Store, Crypto, Config)
├── internal           - Internal logic (Lifecycle, Cgroups, AgentMgr)
└── proto              - Protobuf definitions
```
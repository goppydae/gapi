# Python ADK for GAPI

Python is the **primary ADK** for GAPI agents. Use Python for business logic, application services, and orchestration.

## Quick Start

### Service Agent

```python
# agents/python/services/my_service.py.service

ID = "my_service"
TYPE = "service"
VERSION = "1.0.0"
DESCRIPTION = "My service agent"

def initialize():
    """Called once at agent startup"""
    print(f"[{ID}] Initializing...")

def start():
    """Main service loop"""
    print(f"[{ID}] Starting...")
    import time
    while True:
        # Service logic here
        time.sleep(1)

def stop():
    """Graceful shutdown"""
    print(f"[{ID}] Stopping...")
```

### Timer Agent

```python
# agents/python/timers/my_timer.py.timer

ID = "my_timer"
TYPE = "timer"
VERSION = "1.0.0"
SCHEDULE = "OnUnitActiveSec=60s"  # Run every 60 seconds

def start():
    """Executed on each timer trigger"""
    print(f"[{ID}] Timer triggered!")
    # Task logic here
```

### Socket Agent

```python
# agents/python/sockets/my_socket.py.socket

ID = "my_socket"
TYPE = "socket"
VERSION = "1.0.0"
LISTEN_STREAM = "127.0.0.1:8080"

def start():
    """Handle incoming connections"""
    print(f"[{ID}] Socket activated!")
    # Connection handling here
```

## Agent Metadata

All Python agents must define these module-level attributes:

- `ID` (required): Unique agent identifier
- `TYPE` (required): Agent type (`service`, `timer`, `socket`)
- `VERSION` (required): Semantic version string
- `DESCRIPTION` (optional): Human-readable description
- `REQUIRES` (optional): List of hard dependencies (must start before this agent)
- `WANTS` (optional): List of soft dependencies (start if available)
- `WANTED_BY` (optional): List of agents that want this agent
- `REQUIRED_BY` (optional): List of agents that require this agent

## Lifecycle Methods

### Required Methods

- `start()`: Main entry point (required for all types)

### Optional Methods

- `initialize()`: One-time setup (called before start)
- `stop()`: Graceful shutdown
- `reload()`: Reload configuration without restart

## Capabilities

Use the `@capability` decorator to expose custom capabilities:

```python
from gapi.native.adk import capability

@capability("custom_action")
def perform_action():
    """Custom capability"""
    print("Performing custom action")
```

## Dependencies

Specify dependencies to control startup order:

```python
ID = "web_server"
TYPE = "service"
VERSION = "1.0.0"

# Hard dependencies (must start first)
REQUIRES = ["database", "cache"]

# Soft dependencies (start if available)
WANTS = ["monitoring"]
```

## Resource Limits

```python
ID = "resource_limited"
TYPE = "service"
VERSION = "1.0.0"

# systemd-style resource limits
CPU_LIMIT = "50%"
MEMORY_LIMIT = "512M"
```

## Timer Schedules

Timer agents support systemd-style schedule syntax:

```python
# Run every 5 minutes
SCHEDULE = "OnUnitActiveSec=5m"

# Run daily at midnight
SCHEDULE = "OnCalendar=daily"

# Run on boot, then every hour
SCHEDULE = "OnBootSec=0 OnUnitActiveSec=1h"
```

## Event Bus

Publish events to the GAPI event bus:

```python
from gapi.native.adk import send_event

def start():
    send_event("my_service", "started", {"timestamp": time.time()})
```

## Logging

Use standard Python logging (captured by GAPI):

```python
import logging

logger = logging.getLogger(__name__)

def start():
    logger.info("Service started")
    logger.warning("This is a warning")
    logger.error("This is an error")
```

## Testing

Test agents locally before deployment:

```bash
# Describe metadata
python adk/python/agent/runner.py --module agents/python/services/my_service.py --describe

# Run agent
python adk/python/agent/runner.py --module agents/python/services/my_service.py --start
```

## Best Practices

1. **Keep it simple**: Agents should do one thing well
2. **Explicit dependencies**: Always declare `REQUIRES` and `WANTS`
3. **Graceful shutdown**: Implement `stop()` for clean exits
4. **Idempotent**: `initialize()` should be safe to call multiple times
5. **Error handling**: Catch exceptions, don't crash the supervisor

## See Also

- [GAPI Design Document](../../docs/gapi-design-document.md)
- [Agent Directory Structure](../README.md)
- [Go ADK Guide](../go/README.md)

# GAPI Features

This document provides detailed information about GAPI's core features and capabilities.

## Security & Integrity

GAPI provides cryptographic signing and verification to ensure agent code integrity.

### Ed25519 Signing

GAPI uses Ed25519 public-key cryptography for signing agents. This provides:
- **Fast signing and verification**: Ed25519 is one of the fastest signature schemes
- **Small signatures**: Only 64 bytes per signature
- **Strong security**: 128-bit security level

### BLAKE3 Hashing

Content verification uses BLAKE3, a cryptographic hash function that is:
- **Extremely fast**: Faster than MD5 while being cryptographically secure
- **Parallelizable**: Takes advantage of multi-core processors
- **Secure**: Resistant to length extension attacks

### Signature Enforcement

Agents can be cryptographically signed to ensure authenticity:

1. **Generate a keypair**:
   ```bash
   gapictl keygen mykey
   # Creates mykey.key (private) and mykey.pub (public)
   ```

2. **Sign an agent**:
   ```bash
   gapictl sign agents/myagent.py.service mykey.key
   # Creates agents/myagent.py.service.sig
   ```

3. **Enable verification** in `config.yaml`:
   ```yaml
   security:
     verifyKey: mykey.pub
   ```

When verification is enabled, `gapid` will only load agents with valid signatures.

## Timer Agents

Timer agents execute on a schedule, similar to systemd timers or cron jobs.

### Systemd-Style Scheduling

GAPI supports systemd-compatible timer syntax:

- **`OnUnitActiveSec=DURATION`**: Run DURATION after the last execution completes
- **`OnBootSec=DURATION`**: Run DURATION after system boot
- **`OnStartupSec=DURATION`**: Run DURATION after supervisor startup

Examples:
```python
# SCHEDULE = OnUnitActiveSec=5s   # Every 5 seconds
# SCHEDULE = OnBootSec=1m         # 1 minute after boot
# SCHEDULE = OnStartupSec=30s     # 30 seconds after startup
```

### Cron Expressions

Standard cron syntax is supported:

```python
# SCHEDULE = */5 * * * *    # Every 5 minutes
# SCHEDULE = 0 */2 * * *    # Every 2 hours
# SCHEDULE = 0 9 * * 1-5    # 9 AM on weekdays
# SCHEDULE = 0 0 * * 0      # Midnight on Sundays
```

### Named Schedules

Convenient aliases for common intervals:

```python
# SCHEDULE = @hourly    # Once per hour (0 * * * *)
# SCHEDULE = @daily     # Once per day at midnight (0 0 * * *)
# SCHEDULE = @weekly    # Once per week on Sunday (0 0 * * 0)
# SCHEDULE = @monthly   # Once per month on the 1st (0 0 1 * *)
```

### Raw Durations

Simple duration strings:

```python
# SCHEDULE = 5s    # Every 5 seconds
# SCHEDULE = 1m    # Every minute
# SCHEDULE = 1h    # Every hour
# SCHEDULE = 24h   # Every day
```

### Auto-Start Behavior

Timer agents automatically start when discovered by the supervisor. They do not need to be manually started.

## Resource Limits (Cgroups v2)

GAPI can enforce CPU and memory limits using Linux cgroups v2.

### CPU Limits

Restrict the CPU usage of an agent:

```python
# CPU_LIMIT = 0.5    # 50% of one CPU core
# CPU_LIMIT = 1.0    # 100% of one CPU core
# CPU_LIMIT = 2.0    # 200% (2 cores)
```

The limit is enforced via the `cpu.max` cgroup controller.

### Memory Limits

Enforce hard memory caps:

```python
# MEMORY_LIMIT = 100MB    # 100 megabytes
# MEMORY_LIMIT = 1GB      # 1 gigabyte
# MEMORY_LIMIT = 512MB    # 512 megabytes
```

When an agent exceeds its memory limit, it will be OOM-killed by the kernel.

### Rootless Support

GAPI works in rootless environments with proper cgroup delegation:

1. **Enable cgroup delegation** for your user:
   ```bash
   sudo mkdir -p /etc/systemd/system/user@.service.d/
   echo -e "[Service]\nDelegate=yes" | sudo tee /etc/systemd/system/user@.service.d/delegate.conf
   sudo systemctl daemon-reload
   ```

2. **Verify delegation**:
   ```bash
   cat /sys/fs/cgroup/user.slice/user-$(id -u).slice/user@$(id -u).service/cgroup.controllers
   # Should show: cpuset cpu io memory pids
   ```

3. **Run gapid** as your user (no root required)

## Socket Activation

Socket activation allows agents to start on-demand when a connection is received.

### Lazy Loading

Agents with `TYPE = socket` do not start immediately. Instead:
1. The supervisor listens on the configured address/port
2. When a connection arrives, the supervisor starts the agent
3. The file descriptor is passed to the agent
4. The agent handles the connection

This reduces resource usage for infrequently-used services.

### TCP/UDP Support

Both TCP and UDP sockets are supported:

```python
# TCP socket
# LISTEN_STREAM = 0.0.0.0:8080

# UDP socket
# LISTEN_DATAGRAM = 0.0.0.0:5353
```

### Zero Downtime Handoff

The supervisor holds the listening socket, so:
- The agent can crash and restart without losing the port
- No "address already in use" errors
- Seamless upgrades and restarts

### Accessing the Socket

In Python agents, the socket file descriptor is available via environment variable:

```python
import os
import socket

def start():
    fd = int(os.environ.get("LISTEN_FDS", "0"))
    if fd > 0:
        sock = socket.fromfd(fd + 3, socket.AF_INET, socket.SOCK_STREAM)
        # Handle connections on sock
```

## Python ADK

The Python Agent Development Kit (ADK) provides native Go ↔ Python communication.

### Native Bindings

GAPI uses `gopy` to generate Python bindings for Go packages. This provides:
- **Direct function calls**: No JSON serialization or subprocess overhead
- **Type safety**: Go types are exposed as Python types
- **Bidirectional communication**: Python can call Go, Go can call Python

### Zero Boilerplate

Agents are simple Python scripts with minimal structure:

```python
# agents/example.py.service
# ENABLED = True
# TYPE = service

def start():
    print("Agent started!")
    # Your code here
```

No classes, no inheritance, no framework-specific decorators.

### Self-Describing Metadata

Agent metadata is defined as comments at the top of the file:

```python
# ENABLED = True
# TYPE = service
# DEPENDENCIES = database, cache
# CPU_LIMIT = 0.5
# MEMORY_LIMIT = 512MB
```

The supervisor parses these directives automatically. No separate configuration files needed.

### Lifecycle Hooks

Agents can define lifecycle functions:

- **`start()`**: Called when the agent starts
- **`stop()`**: Called when the agent is stopping (optional)
- **`reload()`**: Called when the agent receives a reload signal (optional)

Example:
```python
def start():
    print("Starting...")
    # Initialization code

def stop():
    print("Stopping...")
    # Cleanup code

def reload():
    print("Reloading configuration...")
    # Reload logic
```

### Event Bus Access

Agents can publish and subscribe to events via the GAPI event bus:

```python
from gapi import eventbus

def start():
    # Subscribe to events
    eventbus.subscribe("system", "agent.status", on_status)
    
    # Publish events
    eventbus.publish("custom", "my.event", {"data": "value"})

def on_status(event):
    print(f"Received status: {event}")
```

This enables inter-agent communication and coordination.

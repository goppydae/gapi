---
title: "Agent Metadata"
weight: 50
---

# Agent Metadata

How an agent declares what it is, and what the supervisor hands it at
exec. Both are read by code the configuration schema does not describe,
so neither appears in the generated
[configuration reference](../../reference/configuration/) - that page
publishes the daemon's own keys, not an agent's.

Everything here is checked against `adk/python/agent/runner.py` and the
runners in `core/agentmgr`.

## Declaring metadata

Agent metadata is declared as **module-level assignments** in the agent
file. It is read with `getattr` on the imported module, so a commented
directive is silently dropped:

```python
# WRONG - a comment. Read by nothing, silently ignored.
# TYPE = "timer"
# SCHEDULE = "OnUnitActiveSec=30s"
```

```python
# RIGHT - real assignments.
TYPE = "timer"
SCHEDULE = "OnUnitActiveSec=30s"
```

### Recognized keys

Each row lists every accepted spelling.

| Meaning | Accepted names |
| ------- | -------------- |
| Identity | `ID`, `id`, `agent_id` |
| Name | `NAME`, `name` |
| Version | `VERSION`, `version` |
| Type | `TYPE`, `type`, `unit_type`, `kind` |
| Enabled | `ENABLED`, `enabled` |
| Interval | `INTERVAL`, `interval` |
| Hard dependencies | `REQUIRES`, `requires`, `DEPS`, `deps`, `Dependencies` |
| Soft dependencies | `WANTS`, `wants` |
| Reverse (wanted) | `WANTED_BY`, `wanted_by`, `WantedBy` |
| Reverse (required) | `REQUIRED_BY`, `required_by`, `RequiredBy` |
| Socket | `LISTEN_STREAM`, `SOCKET`, `PORT` |
| CPU limit | `CPU_LIMIT`, `CPU` |
| Memory limit | `MEMORY_LIMIT`, `MEM`, `MEMORY` |
| Schedule | `SCHEDULE` |
| Description | `DESCRIPTION`, `description`, `Desc` |

`DEPENDENCIES` - plural, all caps - is **not** an accepted alias.
Declaring it yields an agent with no dependencies and therefore no
ordering guarantee. Use `REQUIRES`.

There is no `LISTEN_DATAGRAM`. Socket activation is stream-only: the
supervisor calls `net.Listen`, which yields TCP or UNIX sockets, never
UDP.

### ENABLED

`ENABLED = False` prevents the supervisor from starting the agent
**automatically**. The agent is still discovered and registered, still
appears in `gapictl agent status`, and can still be started explicitly:

```bash
gapictl lifecycle start my-agent
```

This follows systemd's model. An agent that omits `ENABLED` entirely is
enabled - which is why Go agents, whose `--describe` payload does not
carry the field at all, keep starting normally.

### Resource limits

Limits are strings, not bare literals - `MEMORY_LIMIT = 100MB` is a
Python syntax error.

```python
CPU_LIMIT = "0.5"
MEMORY_LIMIT = "100MB"
```

### Example

```python
ID = "hello"
TYPE = "service"
VERSION = "1.0.0"
DESCRIPTION = "Minimal service agent"
ENABLED = True
REQUIRES = ["database"]
CPU_LIMIT = "0.5"
MEMORY_LIMIT = "100MB"

import time


def start():
    while True:
        time.sleep(60)
```

Worked examples of every agent type are on the
[agent examples](../agent-examples/) page.

## What the supervisor passes to an agent

| Variable | Set for | Meaning |
| -------- | ------- | ------- |
| `ADK_RUN_ID` | Go and Python agents | per-start correlation id |
| `ADK_CONTROL_FD` | Go and Python agents | the control descriptor the agent reports its state on |
| `LISTEN_FDS` | socket-activated agents ONLY | how many **listeners** were passed |
| `LISTEN_PID` | socket-activated agents ONLY | `self` |
| `ADK_REJECT_DUMMY` | Python agents | fail loudly rather than falling back to the stub ADK |

The passed descriptors start at **file descriptor 3**, listeners first and
the control descriptor after them. `LISTEN_FDS` is a *count*, not a
descriptor number:

```python
import socket

# One socket passed: LISTEN_FDS=1, the socket is fd 3,
# and ADK_CONTROL_FD is 4.
server = socket.socket(fileno=3)
```

**`LISTEN_FDS` counts listeners and nothing else.** The control descriptor
is not one, and an agent with no socket is passed **no `LISTEN_FDS` at
all** - an absent variable is how the ADK knows there was no socket
activation. `LISTEN_FDS=0` would be a different claim, and a count that
included the control descriptor would send an agent's `Listener()` at fd
3, which is the control channel itself.

The ADK does this arithmetic for you: call `agent.Listener()` in Go, and
branch on `agent.ErrNoInheritedListener` if you want the agent to run
standalone.

There is no `AGENT_ID`, `AGENT_TYPE` or `GAPI_SOCKET`, and no `ENV_`
prefix mechanism for injecting custom variables.

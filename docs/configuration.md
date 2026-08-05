# Configuration Guide

How to configure the supervisor (`gapid`) and how to declare agent
metadata. Everything here is checked against `core/config/config.go` and
`adk/python/agent/runner.py`; if a key or variable is not listed, it is
not read.

## Supervisor configuration

### Where the config file comes from

A release build of `gapid` reads `config.yaml` from **`/etc/gapi`** and
nowhere else (`core/config/loader_prod.go`). A build tagged `dev` also
searches `config/` and the current directory (`loader_dev.go`).

There is **no `--config` flag**. `gapid` defines only `--runtime-addr`,
`--log-level`, `--pid1` and `--no-early-mounts`, and cobra rejects
anything else. To point at a specific file, set an environment variable:

```bash
GAPI_CONFIG=/path/to/config.yaml gapid
```

GAPI runs with no config file at all - every setting below has a
default.

### Environment overrides

**Every** key can be overridden by an environment variable: prefix
`GAPI_`, uppercase, dots become underscores.

```bash
GAPI_LOGGING_LEVEL=debug gapid
```

```bash
GAPI_SUPERVISOR_PRODUCTIONMODE=true gapid
```

```bash
GAPI_TRANSPORT_TLSCERT=/etc/gapi/server.crt gapid
```

The prefix is `RUNTIME`, not `GAPI`. Precedence is environment over
config file over default.

The one shape with no variable is a map: `logging.loki.labels` is
config-file only.

> This used to cover only the keys that happened to carry a registered
> default - the whole `supervisor` section, `security.verifyKey` and the
> TLS paths were dropped in silence, so
> `GAPI_SUPERVISOR_PRODUCTIONMODE=true` produced a daemon with
> signature enforcement off and no error (GAPI-DIV-038). Bindings are now
> derived from the config struct itself, and a test walks every field to
> keep it that way.

### The complete file

Six top-level sections: `transport`, `security`, `metrics`, `logging`,
`timeouts`, `supervisor`.

```yaml
transport:
  type: quic                     # "quic" or "local" - nothing else
  address: "127.0.0.1:14242"
  tlsCert: /etc/gapi/server.crt
  tlsKey: /etc/gapi/server.key
  tlsCa: /etc/gapi/ca.crt
  insecureSkipVerify: true       # DEFAULT IS TRUE - see the warning below

security:
  verifyKey: /etc/gapi/agent-signing.pub.hex

metrics:
  enabled: false
  addr: 127.0.0.1:19090

logging:
  level: info                    # trace, debug, info, warn, error
  format: json                   # json or console
  file:
    enabled: false
    path: /var/log/gapi/gapi.log
    maxSize: 100                 # MB
    maxBackups: 3
    maxAge: 28                   # days
    compress: true

timeouts:
  quicStream: "10s"
  quicIdle: "60s"
  clientPending: "2s"
  clientTerminal: "20s"
  supervisorStart: "20s"
  supervisorShutdown: "5s"

supervisor:
  productionMode: false
  pid1Mode: false
  noEarlyMounts: false
  watchdog:
    enabled: false
    device: /dev/watchdog
    interval: "10s"
  shutdown:
    gracePeriod: "10s"
```

### Transport

Only two types exist: `quic` and `local` (`core/transport/factory.go`).
Setting anything else - `tcp`, `unix` - is a hard startup error.

The TLS keys are `tlsCert`, `tlsKey` and `tlsCa`. Older documentation
used `certFile`/`keyFile`; viper drops unknown keys **silently**, so
that spelling meant the daemon ignored the configured certificate and
generated a throwaway self-signed one instead - a silent downgrade
rather than an error.

> **Warning**: `transport.insecureSkipVerify` defaults to **true**, for
> every address, not just loopback. GAPI does not verify peer
> certificates unless you set it to `false`. Setting
> `supervisor.productionMode: true` does **not** cover this - production
> mode gates agent signature verification and nothing else, so the two
> settings must be made independently.

### Logging

`loki` exists in the schema but is **not implemented**: setting
`logging.loki.enabled: true` makes `gapid` fail at startup with a loud
error rather than silently dropping logs.

JSON output comes from the stdlib `slog` JSON handler, so records carry
`time`, `level` (uppercase) and `msg`.

### Agent signature verification

`security.verifyKey` points at an Ed25519 public key. Either
`GAPI_SECURITY_VERIFYKEY` (the ordinary override) or
`GAPI_VERIFY_KEY` sets it; the latter is a separate `os.Getenv` in
the supervisor, consulted when the config key is empty, and predates the
override covering every key.

Verification is gated on
**production mode**, not on the key being present: with
`supervisor.productionMode: true`, an agent binary must carry a valid
`.b3` digest and `.sig` or it will not start. In production mode with no
verify key configured, discovery rejects every binary - fail closed.

Production mode does exactly two things: this gate, and setting
`ADK_REJECT_DUMMY` for Python agents. It does not touch TLS, the
listen address, or anything else.

Note that only binary agents are verified. Python agents are described
by running the interpreter, which does not go through the signature
path.

## Agent metadata

Agent metadata is declared as **module-level assignments** in the agent
file. It is read with `getattr` on the imported module
(`adk/python/agent/runner.py`), so a commented directive is silently
dropped:

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

## What the supervisor passes to an agent

| Variable | Set for | Meaning |
| -------- | ------- | ------- |
| `ADK_RUN_ID` | Go and Python agents | per-start correlation id |
| `LISTEN_FDS` | socket-activated agents | how many descriptors were passed |
| `LISTEN_PID` | socket-activated agents | `self` |
| `ADK_REJECT_DUMMY` | Python agents | fail loudly rather than falling back to the stub ADK |

The passed sockets start at **file descriptor 3**. `LISTEN_FDS` is a
*count*, not a descriptor number:

```python
import socket

# One socket passed: LISTEN_FDS=1, and the socket is fd 3.
server = socket.socket(fileno=3)
```

There is no `AGENT_ID`, `AGENT_TYPE` or `GAPI_SOCKET`, and no `ENV_`
prefix mechanism for injecting custom variables.

## Environment variables GAPI reads

The `GAPI_` prefix belongs to the PRODUCT, not to the kernel. The kernel
is a library: `goblind` links the same code and reads `GOBLIN_` names,
searches `/etc/goblin`, and tags dmesg with `goblind:`. Every name in
this table is composed from that one identity at startup, so under
another product each row reads `<PREFIX>_` instead (GAPI-DIV-061). The
table below is `gapid`'s view of it.

| Variable | Purpose |
| -------- | ------- |
| `GAPI_CONFIG` | Path to the config file |
| `GAPI_<SECTION>_<KEY>` | Override any config key |
| `GAPI_AGENT_PATH` | Override the agent search root |
| `GAPI_DEV_AGENTS` | Highest-priority agent directory, in either scope |
| `GAPI_SKIP_SYSTEM_AGENTS` | Drop the package-owned agent tiers (`/usr/lib`, `/usr/local/lib`); `/etc` and `/run` survive |
| `GAPI_VERIFY_KEY` | Agent signing public key (fallback for `security.verifyKey`) |
| `GAPI_PY_RUNNER` | Override the Python runner path |
| `GAPI_GO_ADK` | Override the Go ADK source tree used by `gapictl agent build` |
| `GAPI_CGROUPS_DISABLE` | Disable cgroup setup |
| `GAPI_KMSG_PATH` | Override the kmsg device (PID 1 mode) |

There is no `GAPI_AGENTS_DIR`, `GAPI_LOG_LEVEL` or `GAPI_TRACE_EVENTS`.
Those names appear in older documentation and are read by nothing.
`GAPI_AGENT_PATH` is real and listed above. It carried the old
namespace prefix until GAPI-DIV-059, and this paragraph went on denying
it under the new spelling - see the release notes for the rename.

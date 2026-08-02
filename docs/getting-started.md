# Getting Started with GAPI

Create, build and run your first agent.

## Setup

GAPI builds inside a Nix dev shell, which pins the whole toolchain.

```bash
cd gapi
```

```bash
nix develop
```

```bash
mage build
```

This produces `bin/gapid` (the supervisor) and `bin/gapictl` (the CLI).
`mage build` does not install them, so invoke them by path, or run
`mage install` to place them in `$GOPATH/bin`.

Verify the toolchain matches the pins before anything else:

```bash
mage doctor
```

## Your first agent

`gapictl agent new` scaffolds one. The default language is **Go**:

```bash
./bin/gapictl agent new my_service
```

That writes `src/agents/my_service.go.service` - one file, no main.
Build it with `gapictl agent build` and the artifact lands in
`agents/`. For Python, ask for
it explicitly:

```bash
./bin/gapictl agent new my_service --lang python
```

Python agents land in a type-specific directory instead -
`agents/python/services/` for a service, `timers/` for a timer,
`sockets/` for a socket.

The supported types are `service`, `timer` and `socket`:

```bash
./bin/gapictl agent new my_timer --lang python --type timer
```

## What an agent looks like

Metadata is declared as **module-level assignments**. It is read with
`getattr` on the imported module, so a commented directive is silently
ignored and the agent falls back to defaults.

```python
ID = "my_service"
TYPE = "service"
VERSION = "1.0.0"
DESCRIPTION = "My first agent"

import time


def start():
    print("my_service starting")
    while True:
        time.sleep(60)


def stop():
    print("my_service stopping")
```

## Running the supervisor

```bash
./bin/gapid
```

`gapid` binds `127.0.0.1:14242` and discovers agents from its search
paths. To point it at a specific directory:

```bash
GAPI_AGENT_PATH=./agents ./bin/gapid
```

In another shell:

```bash
./bin/gapictl agent status
```

Note the path: `status` is a subcommand of `agent`. A bare
`gapictl status` is not a command.

## Controlling an agent

```bash
./bin/gapictl lifecycle start my_service
```

```bash
./bin/gapictl lifecycle status my_service
```

```bash
./bin/gapictl lifecycle stop my_service
```

`lifecycle status` requires at least one agent name; `agent status`
lists everything.

## Common commands

| Task | Command |
| ---- | ------- |
| List registered agents | `gapictl agent status` |
| Start / stop / restart | `gapictl lifecycle start\|stop\|restart <agent>` |
| Rebuild Go agents | `gapictl agent build` |
| Verify an agent binary | `gapictl agent verify <path>` |
| Interactive monitor | `gapictl tui` |
| Check the daemon is up | `gapictl ping` |
| Version | `gapictl version` |

## Verifying an agent

```bash
./bin/gapictl agent verify agents/my_service.go.service
```

The command reports two things: whether the binary matches its `.b3`
digest, and whether a `.sig` verifies against a signing key. An unsigned
agent reports the missing signature rather than failing outright -
unless the supervisor runs with `supervisor.productionMode: true`, which
refuses to start anything unverified.

To sign one:

```bash
./bin/gapictl crypto keygen --out signing-key
```

That writes `signing-key.pem` (private) and `signing-key.pub.hex`
(public).

```bash
./bin/gapictl crypto sign agents/my_service.go.service --key signing-key.pem
```

That writes the `.b3` digest and the `.sig` over it. Both are required.

## Troubleshooting

**The agent does not appear in `agent status`.** Check the discovery
root. GAPI searches several paths; `GAPI_AGENT_PATH` overrides them
with one directory of your choosing.

**The agent is listed but never starts.** Check `ENABLED`. An agent with
`ENABLED = False` is registered and visible but not started
automatically; `gapictl lifecycle start` still works on it.

**Metadata seems to be ignored.** Check that it is a real assignment and
not a comment. `# TYPE = "timer"` is read by nothing.

**Python metadata is ignored on a fresh clone.** The Python ADK needs
its native bindings built:

```bash
mage python:build
```

## Next steps

- [Configuration](configuration.md) - every config key and metadata field
- [Agent examples](agent-examples.md) - services, timers, sockets
- [Development](development.md) - working on GAPI itself
- [Installation](installation.md) - deploying beyond a dev shell

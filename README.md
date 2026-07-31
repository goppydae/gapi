# GAPI (GoPPydae Agent Process Infrastructure)

**GAPI** is the single-node supervision kernel of the GoPPydae
ecosystem: the mechanism for spawning, supervising, checkpointing and
verifying agents on one machine. It ships as a library, embedded in
process by orchestrators, plus a reference daemon and CLI.

The distributed orchestrator that embeds it is
[Goblin](https://github.com/goppydae/goblin). Running Goblin does not
require running `gapid` alongside it - the kernel is linked in.

## What it does

- **Agent supervision** - three runner types (Go, Python, timer) behind
  one `lifecycle.Agent` interface, with dependency-ordered start.
- **Provenance** - BLAKE3 `.b3` digests and Ed25519 `.sig` sidecars.
  `supervisor.productionMode` refuses to start anything unverified.
- **Checkpoint/restore** - CRIU dump and restore of a running process
  (`core/checkpoint`), the mechanism Goblin's live migration moves.
- **Signal delivery** - pidfd-based, guarded by a start epoch, so a
  signal aimed at a dead run cannot land on its replacement.
- **Resource limits** - cgroups v2, rootless supported.
- **Socket activation** - systemd-style `LISTEN_FDS` handoff. Stream
  sockets only: TCP and UNIX, never UDP.
- **QUIC transport** - one listener, ALPN-routed, with a registry
  Goblin extends.
- **PID 1** - opt-in init mode: subreaper, early mounts, watchdog,
  ordered shutdown.
- **Dual ADK** - Python and Go agents with identical semantics.

## Quick start

```bash
nix develop -c mage build
```

This produces `bin/gapid` and `bin/gapictl`. They are not on `PATH`;
run them by path, or `mage install` them into `$GOPATH/bin`.

Scaffold an agent. The default language is Go:

```bash
./bin/gapictl agent new my_service
```

```bash
./bin/gapictl agent new my_service --lang python
```

Run the supervisor and talk to it:

```bash
./bin/gapid
```

```bash
./bin/gapictl agent status
```

Cluster and lifecycle verbs are subcommands: it is
`gapictl agent status`, not `gapictl status`.

## A Python agent

```python
# agents/python/services/hello.py.service
ID = "hello"
TYPE = "service"

import time


def start():
    print("Hello from GAPI!")
    while True:
        time.sleep(60)
```

No classes, no inheritance. Metadata is read with `getattr` on the
imported module, so it must be a real assignment - a commented
`# TYPE = "timer"` is silently ignored.

## Documentation

- **[Getting started](docs/getting-started.md)** - first agent, end to end
- **[Installation](docs/installation.md)** - NixOS module, images, systemd
- **[Configuration](docs/configuration.md)** - every key and metadata field
- **[Development](docs/development.md)** - building and testing GAPI itself
- **[Features](docs/features.md)** - security, timers, cgroups, sockets
- **[Design document](docs/gapi-design-document.md)** - architecture and philosophy
- **[Agent development](agents/README.md)** - writing agents
  ([Python](agents/python/README.md), [Go](agents/go/README.md))

## Repository layout

```
gapi/
|-- cmd/gapid/            # supervisor daemon
|-- cmd/gapictl/          # control CLI
|-- core/                 # the kernel, embedded by orchestrators
|   |-- agentmgr/ lifecycle/ supervisor/
|   |-- checkpoint/ procsig/ cgroups/
|   |-- crypto/ transport/ eventbus/ config/
|   `-- pid1/ subreaper/ mounts/ watchdog/ shutdown/
|-- internal/             # not importable by consumers
|-- pkg/cli/              # gapictl commands
|-- adk/go/ adk/python/   # agent development kits
|-- proto/gapi/v1/        # schemas
|-- nix/                  # NixOS module, package, image generators, VM tests
`-- divergence.jsonl      # where design and code currently disagree
```

## Development

```bash
nix develop -c mage doctor
```

```bash
nix develop -c mage test
```

```bash
nix develop -c mage testADK
```

`mage test` runs the Go suite only. The Python ADK needs its generated
bindings, which are not committed:

```bash
nix develop -c mage python:build
```

## License

Mozilla Public License 2.0 (MPL-2.0)

# GAPI Design Document

**Version**: 1.3
**Author**: Enqack

This describes the design as built. Where the design and the code
disagree, `divergence.jsonl` is the record, and it wins over this
document.

______________________________________________________________________

## Overview

GAPI is a single-node agent supervision kernel: an event-driven runtime
that starts, supervises, verifies and checkpoints agent processes. It
ships as a library first and a daemon second. Go and Python agents are
first-class and are meant to behave identically.

______________________________________________________________________

## Mechanism versus policy

The ecosystem splits on one line, and every allocation of
responsibility follows from it.

### GAPI - mechanism, strictly single-machine

"I know how to start a process, capture its output, restart it when it
dies, verify its signature, and dump its memory."

GAPI knows nothing about clusters, peers, leaders or consensus. It
treats the world as though it is the only computer that exists. It is
the library Goblin imports and runs **in process**; a Goblin deployment
is one binary, not two daemons.

### Goblin - policy, across machines

"I know that agent X should be running on node 3, and I know how to move
it to node 4."

Raft consensus, Serf membership, placement, failover, capability token
issuance, migration coordination.

### Separation matrix

| Concern | Owner | Why |
| ------- | ----- | --- |
| Process supervision | GAPI | controlling a pid is a local operation |
| Output capture | GAPI | happens at the process boundary |
| Signal delivery (pidfd + epoch) | GAPI | a pid is local |
| Checkpoint/restore mechanism | GAPI | CRIU acts on a local process |
| AGE encryption | GAPI | decrypting a secret is a local concern |
| Local event bus | GAPI | routing between local agents |
| cgroups v2 limits | GAPI | a kernel-local control |
| PID 1 duties | GAPI | init is per-machine by definition |
| Consensus (Raft) | Goblin | coordinated state needs the network |
| Membership (Serf) | Goblin | finding peers is a cluster concern |
| Placement and failover | Goblin | *where* to start it |
| Live migration policy | Goblin | *when* and *whither* to move it |
| Capability token issuance | Goblin | authority is cluster-scoped |
| Distributed event bus | Goblin | routing between nodes |

Note the pairing: GAPI owns "the act of starting it", Goblin owns "the
decision of where". The same pairing holds for migration - GAPI dumps
and restores, Goblin decides and transports.

______________________________________________________________________

## Core architecture

- **Languages**: Go compiled, Python through gopy-generated native
  bindings.
- **Transport**: QUIC, or an in-process local transport. Those are the
  only two - `core/transport/factory.go` accepts `quic` and `local` and
  errors on anything else. There is no TCP transport and no UNIX-socket
  transport.
- **Wire format**: Protocol Buffers, for control and telemetry both.
- **Logging**: the standard library's `log/slog`, JSON or text. Zerolog
  is not used; one config value (`trace`) survives from the zerolog era
  and maps to debug.
- **Lifecycle**: a controller driving a state machine, with runners
  behind a narrow interface.

______________________________________________________________________

## Identity, security and integrity

Shipped, not planned:

- **BLAKE3** digests over agent binaries, written as a `.b3` sidecar.
- **Ed25519** signatures over that digest, written as a `.sig` sidecar.
  Sign and verify operate on the canonical hex digest, not on the
  sidecar's bytes - conflating the two is GAPI-DIV-032, and it made
  verification impossible for every signature the CLI produced.
- **AGE** for encrypting material at rest.
- **Capability tokens** verified here, issued by the orchestrator.
  Rights are partitioned: GAPI owns bits 0-7 (signal delivery), Goblin
  owns bits 8 and up.

There is no `gapi-crypto` binary. The crypto surface is
`core/crypto` as a library and `gapictl crypto` as a CLI.

Enforcement is gated on `supervisor.productionMode`, not on a key being
configured. In production mode with no verify key, discovery rejects
every binary loudly - fail closed, never open.

Python agents are not signed artifacts; they are described by running
the interpreter over the module.

______________________________________________________________________

## Lifecycle model

### The agent interface

`core/lifecycle.Agent` - what a supervised thing must answer:

```go
Initialize() error
Start() error
Stop() error
Restart() error
Reload() error
Describe() *meta.AgentInfo
ID() string
Type() string
Scope() string
```

`Restart()` is **required**, not optional. Every method above is part of
the contract.

### The runner interface

`core/lifecycle.Runner` - what actually owns a process:

```go
Start(ctx context.Context) error
Stop(ctx context.Context) error
Reload(ctx context.Context) error
Reset()
```

The context bounds the **start operation**, not the process's lifetime.
Binding a child to it hands `os/exec` a watchdog that kills the agent
the moment the start call returns; that was GAPI-DIV-028, and the
interface comment now records it.

### Optional capabilities

Extensions are advertised by implementing an interface, and asserted
once at admission rather than discovered at failure time:

- `RunIDSetter` - accepts a per-start correlation id.
- `Checkpointer` - can be dumped and restored with CRIU. `GoAgent` and
  `PythonAgent` implement it; `TimerAgent` does not, because it has no
  child process to dump.

`Checkpoint` leaves the process **stopped** on success. That is
deliberate: the image is the rollback artifact for a migration, and a
source that keeps executing past the point its image captured has
already diverged from it.

There are no `BeforeStart`, `AfterStop` or `OnSignal` hooks anywhere in
the codebase.

______________________________________________________________________

## SDK design

### Flat functions

An agent is a file of functions, not a class hierarchy. Each function
maps to a lifecycle phase, and the Python runner resolves each phase
against a list of accepted names - `start` or `run`; `stop`, `shutdown`
or `teardown`; `reload`, `rehash` or `reconfigure`.

Only `start` is required. If it takes one parameter, the runner passes a
`threading.Event` set on shutdown, which is the cooperative exit path.

### Self-description

- **Go ADK**: the agent answers `--describe` on stdout with a JSON
  metadata block. Discovery runs it.
- **Python ADK**: the runner imports the module and reads
  module-level assignments with `getattr`, against an alias table. That
  is why comment-style directives are silently dropped - a comment is
  not an attribute.

No manifest files, in either language.

### The bindings are generated

The Python ADK reaches the kernel through gopy bindings under
`adk/python/gapi/native/`, which are built (`mage python:build`), not
committed. Absent them the runner falls back to a stub that writes
events to stdout. `RUNTIME_REJECT_DUMMY_ADK` turns that into a hard
failure, and production mode sets it.

gopy cannot bind Go channels, so the bridge is blocking calls over
simple types: `Initialize`, `StartQUIC`, `SendEvent`, `StartHeartbeat`,
`SetSchemaHash`, `ComputeSchemaHash`.

______________________________________________________________________

## Describe schema

Two shapes exist, deliberately.

`lifecycle.Agent.Describe()` returns a typed `*meta.AgentInfo`:

```go
type AgentInfo struct {
    ID          string
    Name        string
    Version     string
    Type        string
    Description string
    Interval    int
    Enabled     bool
    Implements  []string
}
```

`agentmgr.Agent.Describe()` returns `map[string]string`, which is what
the supervisor consults for `type` and `listen_stream` when deciding how
to auto-start. Being stringly typed is why the enabled flag could not
live there and became a typed accessor instead (GAPI-DIV-034).

A Go agent's `--describe` output nests under a `describe` key:

```json
{
  "describe": {
    "id": "my_service",
    "type": "service",
    "version": "1.0.0",
    "language": "go",
    "capabilities": ["initialize", "start", "stop"]
  }
}
```

______________________________________________________________________

## Interface contracts

Protobuf defines the lifecycle and IPC surface: `LifecycleControl`,
`LifecycleStatus` and `Envelope` are the core messages. Schema evolution
is gated by `buf breaking` - see
[protobuf_compatibility.md](protobuf_compatibility.md), including what
that gate does *not* catch.

______________________________________________________________________

## Ecosystem

- **GAPI** - the kernel: lifecycle, logging, crypto, checkpoint,
  transport.
- **Goblin** - the orchestrator: membership, consensus, placement,
  migration. Embeds GAPI.
- **magelib** - the shared build library both repos use for their gates.
- **GoPPydae** - the ecosystem containing all of the above.

Division of labour inside a deployment: Go carries lifecycle, logging
and IPC; Python carries agent behaviour; Protobuf carries everything
crossing a boundary.

______________________________________________________________________

## Developer experience

- Zero boilerplate - write functions.
- Optional capabilities detected by interface assertion, not
  configuration.
- Flat files, no manifests.
- `gapictl` for the full local lifecycle. Cluster verbs live in
  `goblinctl`, not here.

______________________________________________________________________

## Appendix A - naming and taxonomy

Python agents are `<name>.py.<unit-type>`, for example
`heartbeat.py.timer`. Discovery matches on the `.py.` infix plus one of
exactly **three** suffixes: `.service`, `.timer`, `.socket`. There is no
`.pipe`, `.event` or `.init` unit type.

`TYPE` values the supervisor acts on are `service`, `oneshot`, `timer`
and `socket`. A `service` or `oneshot` with no listen address is started
immediately; a timer is scheduled; anything declaring `LISTEN_STREAM` is
armed for lazy activation instead.

A Go agent is any executable that answers `--describe`; its filename
carries no meaning.

The `dae` suffix designates GoPPydae descendants.

## Appendix B - roadmap

- Schema validation and describe consistency across the two ADKs.
- Persistent timers: a one-shot that elapsed while the supervisor was
  down fires on next start, but nothing is recorded across restarts, so
  a repeating schedule has no catch-up.
- Native gopy channel support - needs upstream work or an architecture
  shift.

## Appendix C - build metadata

Version is stamped at link time into `core/version.GAPIVersion`,
resolved env-over-tag-over-file from the root `VERSION`. Editing
`core/version/version.go` achieves nothing; the linker overwrites it.
Build, hash and release automation live in `Magefile.go` on top of
`magelib`.

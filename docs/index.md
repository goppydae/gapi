# GAPI

The **GoPPydae Agent Process Infrastructure**: a single-node agent
supervision kernel. It starts, supervises, verifies and checkpoints
agent processes on one machine, and knows nothing about clusters.

It ships as a library first and a daemon second. The distributed
orchestrator, Goblin, embeds it in process rather than talking to a
separate `gapid`.

## Core concepts

- **Supervisor** (`gapid`) - discovers agents, orders their start by
  dependency, and owns their lifecycle.
- **Agent** - a unit of work in Go or Python. Services run
  continuously, timers fire on a schedule, socket agents start on
  traffic.
- **Runner** - what actually owns a process. Three of them: `GoAgent`,
  `PythonAgent`, `TimerAgent`.
- **Event bus** - in-process pub/sub carrying lifecycle control and
  status as Protobuf messages.
- **Provenance** - a BLAKE3 digest and an Ed25519 signature per binary;
  production mode refuses to start anything that does not verify.
- **Checkpoint** - CRIU dump and restore of a running process. The
  mechanism; the policy that moves one between machines is Goblin's.

## Start here

- [Getting started](getting-started.md) - build and run your first agent
- [Installation](installation.md) - NixOS module, machine images, systemd
- [Configuration](configuration.md) - every config key and metadata field
- [Configuration examples](config-example.md) - worked setups

## Reference

- [Features](features.md) - what the kernel does, in detail
- [Agent examples](agent-examples.md) - services, timers, sockets, Go agents
- [Design document](gapi-design-document.md) - architecture and philosophy
- [Library usage](library_usage.md) - embedding the kernel in your own binary

## Working on GAPI

Contributor documentation - the development guide, the cursed knowledge, PID 1
testing, the determinism checklist, the protobuf compatibility contract, the
glossary, the lexicon, the lore and the kernel's history - lives in the
goppydae-docs repository rather than here. This site is the operator and
library surface.

## The honesty layer

Design docs describe the target. `divergence.jsonl` in the repo root
records where the code and the design currently disagree, and why.
Check it before trusting any document here as current.

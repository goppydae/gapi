# Development Guide

Working on GAPI itself.

## Prerequisites

Everything comes from the Nix dev shell, which pins the whole toolchain.
Do not satisfy a missing tool with a host install: `mage doctor` fails
closed on anything resolving outside `/nix/store`.

```bash
nix develop
```

```bash
mage doctor
```

The shell pins Go **1.26** (matching the `go` and `toolchain` directives
in `go.mod`), gcc, protobuf and buf, golangci-lint, gosec, govulncheck,
criu and libseccomp, and a Python carrying `pytest`, `jsonschema` and
`pybindgen`.

`gopy` is built on first shell entry from the in-repo `tools/gopy`
module, pinned at v0.4.10, into `$GOBIN` - so a stray host `gopy` cannot
shadow the pin.

## Building

```bash
mage build
```

Produces `bin/gapid` and `bin/gapictl`, each with a `.b3` digest
sidecar, stamped with the version resolved from `VERSION`.

### Building outside the workspace

The repo commits a `go.work` listing `../magelib`. A lone clone without
that sibling must disable the workspace:

```bash
GOWORK=off go build ./cmd/gapid
```

This is the contract CI uses - it has no sibling checkouts and sets
`GOWORK=off` for every job.

### Python bindings

The gopy-generated bindings under `adk/python/gapi/native/` are
generated, not committed, so a fresh clone has none:

```bash
mage python:build
```

Without this the Python ADK degrades to a stub (`DummyAdk`), and the
Python half of the cross-ADK suite tests the fallback rather than the
ADK. CI runs it before every test job for exactly that reason.

## Testing

| Target | What it runs |
| ------ | ------------ |
| `mage test` | `go test -race ./...` - Go tests only |
| `mage testShort` | the fast inner-loop subset |
| `mage testUnit` | `./internal/...` only |
| `mage testADK` | cross-ADK integration; builds the Python bindings first |
| `mage testE2E` | the end-to-end script |
| `mage testPid1` | gapid as PID 1 of a rootless podman container |
| `mage testTimer` | timer agent behaviour |
| `mage testIntegrity` | provenance and signature verification |
| `mage fuzz` | every `Fuzz*` target, bounded |

`mage test` runs **Go tests only** and does not build the Python
bindings, so on a fresh clone it does not exercise the Python ADK. Run
`mage python:build` first, or use `mage testADK`, which depends on it.

A single package:

```bash
go test -race ./core/lifecycle/
```

## Running it

```bash
mage dev
```

Builds the binaries **and** the Python bindings, then runs `./bin/gapid`
in the foreground with output attached. It is not a watch mode and does
not reload on change - stop it and re-run.

## Gates

```bash
mage lint
```

Three things, in order: a `gofmt -l` check that fails on any unformatted
file, `golangci-lint` with the pinned config from
`../magelib/.golangci.yml`, and **gosec**. The gosec step is the one
most likely to fail a first contribution. Two rules are carved out
repo-wide, with the reasons recorded at `Magefile.go`'s `Lint`:

- **G204** - launching operator-registered agent binaries with
  discovered paths is the product. Roots are fenced by
  `GAPI_AGENT_PATH` and binaries are signature-verified before start.
- **G304** - every variable-path open routes through `internal/safeio`,
  so the rule fires only inside that package.

Adding a third carve-out needs a ledger entry, not a `#nosec` comment.
There is no Python linting in this target.

```bash
mage vuln
```

`govulncheck`, and a required CI gate. It skips loudly when offline
rather than passing quietly.

```bash
mage fmt
```

`goimports -w` over the Go sources. It does not touch Python.

```bash
mage envcheck
```

Compares the sibling dev shells' tool inventories. Skew between gapi,
goblin and magelib is red.

## Repository layout

```
gapi/
|-- cmd/
|   |-- gapid/            # supervisor daemon (gapid.go)
|   `-- gapictl/          # CLI shim; the commands live in pkg/cli
|-- core/                 # the kernel - public API, embedded by goblin
|   |-- agentmgr/         # runners (go, python, timer) and discovery
|   |-- lifecycle/        # Agent/Runner interfaces, controller, state machine
|   |-- supervisor/       # boot, wiring, PID 1 sequencing
|   |-- checkpoint/       # CRIU dump/restore (Linux only)
|   |-- procsig/          # pidfd signal delivery guarded by start epoch
|   |-- crypto/           # Ed25519, BLAKE3, AGE, capability tokens
|   |-- transport/        # QUIC and the ALPN registry
|   |-- eventbus/         # in-process bus and topic constants
|   |-- cgroups/          # cgroups v2 resource limits
|   |-- config/           # viper loading and agent search paths
|   |-- pid1/ subreaper/ mounts/ watchdog/ shutdown/
|   `-- schema/ state/ store/ clock/ logging/ metrics/ tui/ version/ client/ adk/
|-- internal/             # not importable by consumers
|   |-- agentreg/ agents/ db/ ident/ logattr/
|   `-- proto/ safeio/ statewatch/ toposort/
|-- pkg/
|   |-- cli/              # every gapictl command, plus templates/
|   `-- proto/            # generated protobuf
|-- adk/
|   |-- go/               # Go ADK
|   `-- python/           # Python ADK (native/ is generated)
|-- proto/gapi/v1/        # schema sources
|-- nix/                  # NixOS module, package, VM tests, generators
|-- tools/gopy/           # pinned gopy toolchain module
|-- test/                 # adk/, e2e, pid1/
|-- config/config.yaml    # example configuration
`-- divergence.jsonl      # where design and code currently disagree
```

`agentmgr`, `lifecycle`, `eventbus`, `cgroups` and `transport` live
under `core/`, not `internal/` - they are the API goblin embeds. There
is no `internal/scheduler` and no `internal/socket`.

## Key components

- **Agent manager** (`core/agentmgr`) - discovery and the three runners.
  `GoAgent` and `PythonAgent` own external processes; `TimerAgent` runs
  in process, which is why it does not implement `Checkpointer`.
- **Lifecycle** (`core/lifecycle`) - the `Agent` and `Runner` interfaces
  plus the optional capabilities `RunIDSetter` and `Checkpointer`.
- **Supervisor** (`core/supervisor`) - boot ordering, dependency-aware
  start, subreaper and PID 1 wiring.
- **Timers** - `core/agentmgr.NewTimerAgent`, dispatched by the
  supervisor. There is no separate scheduler package.
- **Socket activation** - the `LISTEN_FDS`/`LISTEN_PID` path in the two
  process runners. There is no separate socket package.

## Debugging

```bash
./bin/gapid --log-level debug
```

```bash
GAPI_LOGGING_LEVEL=debug ./bin/gapid
```

`GAPI_LOG_LEVEL` and `GAPI_TRACE_EVENTS` are read by nothing. The
environment prefix is `RUNTIME`.

## Versioning

`VERSION` at the repo root is the single source of truth. The build
resolves it env-over-tag-over-file and stamps `core/version.GAPIVersion`
at link time. Editing `core/version/version.go` achieves nothing - the
linker overwrites it.

## Before opening a pull request

```bash
mage fmt
```

```bash
mage lint
```

```bash
mage vuln
```

```bash
mage test
```

If a change alters behaviour a design doc describes, update the doc or
record the divergence in `divergence.jsonl`. The ledger is the honesty
layer, and entries close on artifacts rather than assertions.

# Go ADK for GAPI

A Go agent is **a single file, not a program**. It declares metadata and
lifecycle functions and nothing else - no `main`, no flag parsing, no
hand-written `--describe` JSON, no signal handling. The ADK supplies all
of it, which is what makes Go and Python agents equivalent rather than
merely similar.

## When to use Go agents

Go suits agents that are performance-sensitive, need static linking, or
run early in boot before a Python interpreter is available. Python suits
orchestration and glue. The lifecycle semantics, dependency declarations,
resource limits and event publishing are identical in both - that is the
parity contract, not a coincidence, and the cross-ADK suite enforces it.

## Quick start

```bash
gapictl agent new --lang go --type service my_service
```

That writes `src/agents/my_service.go.service`:

```go
package agent

import "context"

const (
	ID          = "my_service"
	Type        = "service"
	Version     = "1.0.0"
	Description = "My service agent"
	Enabled     = true
)

func Initialize() error { return nil }

// Start blocks until the supervisor cancels. A service that returns
// early is a service that stopped.
func Start(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func Stop() error { return nil }
```

`func Start() error` is accepted too, when the agent has no use for the
context. `Initialize`, `Stop`, `Reload` and `Restart` are optional.

### The filename carries the type

`<name>.go.<type>`, exactly as Python uses `<name>.py.<type>`. Because it
does not end in `.go`, the file is invisible to `go build ./...`, so the
agent tree needs no build or lint exclusion - that is a property of the
naming scheme rather than configuration anyone maintains.

### Source and artifact live apart

A built Go agent keeps its source's exact name, so the two cannot share a
directory. Sources live off the agent search path (`src/agents/` by
default); the build writes the artifact into an agent directory:

```bash
gapictl agent build src/agents/my_service.go.service
```

That produces `agents/my_service.go.service` - an executable, sitting
beside Python agents, every one readable by name.

To have the daemon discover it, name the directory. There is no implicit
`./agents` tier:

```bash
export GAPI_DEV_AGENTS=$PWD/agents
```

## Declarations

Package-level consts and vars, in idiomatic Go spelling. The Python
spellings are accepted as aliases, so porting an agent between languages
does not mean renaming declarations.

| Go | wire | notes |
|----|------|-------|
| `ID` | `id` | required |
| `Type` | `type` | `service`, `timer`, `socket` |
| `Version` | `version` | |
| `Description` | `description` | |
| `Enabled` | `enabled` | absent means enabled |
| `Requires` | `requires` | hard dependency |
| `Wants` | `wants` | soft dependency |
| `WantedBy` | `wanted_by` | reverse edge, systemd's `[Install]` direction |
| `RequiredBy` | `required_by` | reverse edge |
| `Schedule` | `schedule` | timers; cron or `OnUnitActiveSec=` |
| `ListenStream` | `listen_stream` | sockets; the address the SUPERVISOR binds |
| `CPULimit` | `cpu_limit` | `0.5` = half a core |
| `MemoryLimit` | `memory_limit` | `"512MB"` |
| `Capabilities` | `capabilities` | names beyond the lifecycle set |

Lifecycle functions are reported as capabilities automatically;
`Capabilities` declares any extra ones, which is Go's equivalent of the
Python ADK's `@capability` decorator.

## Agent types

### Service

Runs continuously. `Start` blocks until the context is cancelled. The
supervisor treats the agent as RUNNING once the ADK reports ready.

### Timer

Runs once per firing and **returns**. The supervisor waits for the
process to exit before scheduling the next firing, so blocking in a
timer's `Start` holds the timer inside a single run and every later
firing is skipped. One firing is bounded at 30s.

```go
const Type = "timer"

var Schedule = "OnUnitActiveSec=60s"

func Start() error { return nil }
```

### Socket

Socket-activated. `ListenStream` declares the address **the supervisor
binds**, not one the agent binds. The listener arrives as an inherited
descriptor:

```go
func Start(ctx context.Context) error {
	ln, err := adk.Listener()
	if err != nil {
		return err
	}
	defer func() { _ = ln.Close() }()
	// accept on ln
}
```

Never call `net.Listen` on `ListenStream`. The supervisor has already
bound it, so a fresh bind either fails with `EADDRINUSE` or races the
supervisor and wins - which is worse, because connections that queued
before the agent started are then stranded on a socket nobody is serving.

## Build artifacts

`gapictl agent build` emits the binary plus a BLAKE3 hash file, and a
signature when `--sign` is given:

```
agents/my_service.go.service
agents/my_service.go.service.b3
agents/my_service.go.service.sig
```

The hash covers the assembled package - the author's file plus the
generated main - because that is what was compiled.

```bash
# one agent
gapictl agent build src/agents/my_service.go.service

# every agent under a directory
gapictl agent build src/agents/

# signed
gapictl agent build --sign --key=signing.key src/agents/my_service.go.service

# rebuild on change
gapictl agent build --watch src/agents/
```

## Dependencies

The assembled package is built inside the module that provides the
kernel, so an agent can import it and the standard library without a
`go.mod` of its own. Third-party dependencies are not supported today.

## Testing

```bash
# metadata, the way discovery reads it
agents/my_service.go.service --describe

# supervised, the way the daemon runs it
agents/my_service.go.service --start

# integrity
gapictl agent verify agents/my_service.go.service
```

`--start` prints lifecycle events as JSON lines on stdout: `starting`,
then `ready`, then `stopping` and `stopped`. That stream is the control
channel the supervisor reads, which is why the ADK redirects anything an
agent writes to stdout onto stderr - a stray `fmt.Println` would
otherwise displace the protocol.

## See also

- [Agent tree layout](../README.md)
- [Agent examples](../../docs/content/user/agent-examples.md)
- [Getting started](../../docs/content/user/getting-started.md)

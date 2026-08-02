# Features

What the kernel actually does, checked against source.

## Provenance and integrity

### The chain

A built binary carries two sidecars: `<binary>.b3`, a BLAKE3 digest in
hex, and `<binary>.sig`, an Ed25519 signature. Verification is two
independent checks - the binary must hash to the digest, and the
signature must verify over that digest.

Both are signed and verified over the **canonical hex digest**, not over
the sidecar file's bytes. The sidecar is written with a trailing
newline; treating those bytes as the signed message is what made
verification impossible before GAPI-DIV-032.

Ed25519 gives 64-byte signatures and constant-time verification.
BLAKE3 is used for the digest.

### Signing an agent

```bash
gapictl crypto keygen --out signing-key
```

Writes `signing-key.pem` (private) and `signing-key.pub.hex` (public).

```bash
gapictl crypto sign path/to/agent --key signing-key.pem
```

That writes **both** sidecars: `path/to/agent.b3` and `path/to/agent.sig`.
Verification needs both, and it reads the digest first - a `.sig` without
its `.b3` fails before the signature is ever checked.

```bash
gapictl agent verify path/to/agent
```

`agent verify` reports the digest check and the signature check
separately. An unsigned agent reports a missing signature rather than
failing outright.

### Enforcement

```yaml
security:
  verifyKey: /etc/gapi/agent-signing.pub.hex

supervisor:
  productionMode: true
```

Enforcement is gated on **production mode**, not on the key being
present. Without `productionMode: true` an unverifiable agent still
starts; with it, an agent binary must carry a valid digest and
signature or the supervisor refuses to start it.

Only binary agents go through this path. A Python agent is described by
running the interpreter over the module, which is not a signed artifact.

### AGE encryption

`gapictl crypto` also carries an AGE identity and stream cipher, for
material an agent needs at rest:

```bash
gapictl crypto age-keygen
```

```bash
gapictl crypto encrypt < secret.txt > secret.age
```

```bash
gapictl crypto decrypt < secret.age
```

## Timer agents

The *scheduler* runs inside the supervisor; each **fire** spawns a fresh
process that runs the agent to completion and exits - a Python
interpreter through the ADK runner, or the binary directly. That is why
`TimerAgent` is the one runner without `Checkpointer`: between fires
there is no process to dump.

Because the supervisor waits for each fire to exit before scheduling the
next, a timer body must terminate. The Python runner enforces this by
treating `timer` and `oneshot` types as one-shot - no readiness poll, no
supervision loop. A fire is bounded at 30 seconds, and stopping the agent
cancels a fire already in flight.

Timers work in both ADKs. A Go agent declaring `TYPE = "timer"` in its
`--describe` output is scheduled exactly as a Python one is; a fire runs
the binary directly, which is what its `main` does when not asked to
describe itself. That parity is recent - discovery used to route
`TYPE=timer` to the scheduler only for Python paths, so a Go timer ran
once at discovery and its `SCHEDULE` was discarded (GAPI-DIV-037).

### Schedule syntax

`ParseSchedule` (`core/agentmgr/schedule.go`) tries four forms, in this
order:

| Form | Fires | Example |
| ---- | ----- | ------- |
| `OnUnitActiveSec=D` | repeatedly, every D | `OnUnitActiveSec=30s` |
| `OnStartupSec=D` | **once**, D after the timer starts | `OnStartupSec=5s` |
| `OnBootSec=D` | **once**, D after the system booted | `OnBootSec=1m` |
| raw Go duration | repeatedly | `5s`, `1m30s`, `24h` |
| cron, five fields | repeatedly | `*/5 * * * *`, `0 9 * * 1-5` |
| cron descriptor | repeatedly | `@hourly`, `@daily`, `@weekly`, `@monthly` |

Durations are parsed by `time.ParseDuration`, so `24h` works and `1d`
does not.

The two one-shot forms differ in their anchor, and that is the whole
point of having both: `OnStartupSec` measures from the moment the timer
starts, `OnBootSec` from the system's boot. On a host that has been up
longer than the declared duration, an `OnBootSec` timer has missed its
elapse point and fires immediately, once - late rather than cancelled,
which is what systemd does.

The three prefixes used to be aliases: whichever one matched was
stripped and the duration became a repeating interval, so `OnBootSec=1m`
ran every minute forever (GAPI-DIV-036).

A timer with no `SCHEDULE` gets `OnUnitActiveSec=60s`.

### Declaring one

```python
ID = "heartbeat"
TYPE = "timer"
SCHEDULE = "OnUnitActiveSec=30s"


def start():
    print("tick")
```

Real assignments, not comments. Metadata is read with `getattr` on the
imported module, so `# SCHEDULE = "@hourly"` is read by nothing and the
agent silently falls back to the 60-second default.

Timer agents are auto-started at discovery. `ENABLED = False` suppresses
that, as it does for every runner type.

## Resource limits

cgroups v2, applied when the child process starts and torn down when it
stops.

### CPU

```python
CPU_LIMIT = "0.5"
```

A fraction of one core, written to `cpu.max`. Millicores also parse:
`"500m"` is the same as `"0.5"`.

### Memory

```python
MEMORY_LIMIT = "100MB"
```

Written to `memory.max`; exceeding it means an OOM kill by the kernel.
Accepted suffixes are `K`/`KB`, `M`/`MB`, `G`/`GB`, base 1024. A bare
number is bytes.

Both values are **strings**. `MEMORY_LIMIT = 100MB` is a Python syntax
error, and `CPU_LIMIT = 0.5` is a float where a string is expected.

### Rootless

Works without root given cgroup delegation:

```bash
sudo mkdir -p /etc/systemd/system/user@.service.d
```

```bash
printf '[Service]\nDelegate=yes\n' | sudo tee /etc/systemd/system/user@.service.d/delegate.conf
```

```bash
sudo systemctl daemon-reload
```

```bash
cat /sys/fs/cgroup/user.slice/user-$(id -u).slice/user@$(id -u).service/cgroup.controllers
```

`cpu` and `memory` must appear in that list. Without delegation the
supervisor logs the failure and runs the agent unconstrained rather than
refusing to start it.

## Socket activation

### Lazy start

An agent that declares `LISTEN_STREAM` is **armed** rather than started:
the supervisor binds the socket, holds it, and starts the agent on the
first connection. The trigger is a non-empty `listen_stream`, not
`TYPE = "socket"` - a service that declares a listen address is armed
too.

Because the supervisor owns the listener, the agent can crash and
restart without losing the port, and without `EADDRINUSE`.

### Stream sockets only

The bind goes through `net.Listen`, which yields TCP or UNIX sockets.
**There is no UDP socket activation**, and no `LISTEN_DATAGRAM`
metadata key.

```python
LISTEN_STREAM = "0.0.0.0:8080"   # TCP
```

```python
LISTEN_STREAM = "/run/myagent.sock"   # UNIX, path starts with /
```

```python
LISTEN_STREAM = "8080"   # bare port, becomes :8080 TCP
```

`SOCKET` and `PORT` are accepted aliases for `LISTEN_STREAM`.

### Receiving the descriptor

The supervisor passes descriptors as `ExtraFiles` and sets `LISTEN_FDS`
to the **count**, plus `LISTEN_PID=self`. Passed descriptors begin at
**file descriptor 3**.

```python
import os
import socket


def start():
    n = int(os.environ.get("LISTEN_FDS", "0"))
    if n < 1:
        raise RuntimeError("not socket-activated")
    sock = socket.socket(fileno=3)
    conn, _ = sock.accept()
```

`fd + 3` is wrong for any `LISTEN_FDS` other than the accidental case.
With one socket passed, `LISTEN_FDS` is `1` and the socket is fd `3`.

## Python ADK

### Zero boilerplate

```python
ID = "example"
TYPE = "service"
ENABLED = True


def start():
    print("started")
```

No classes, no inheritance, no decorators required.

### Entry point aliases

The runner resolves each hook against a list of names, first match
wins:

| Hook | Accepted names |
| ---- | -------------- |
| init | `initialize`, `init`, `setup` |
| start | `start`, `run` |
| stop | `stop`, `shutdown`, `teardown` |
| reload | `reload`, `rehash`, `reconfigure` |
| restart | `restart` |

Only `start` is required. If `start` accepts one parameter, the runner
passes a `threading.Event` that is set on shutdown - the cooperative way
to exit a loop:

```python
def start(stop_evt):
    while not stop_evt.wait(1.0):
        do_work()
```

### Metadata

See [configuration.md](configuration.md) for the full alias table. The
one trap worth repeating: `DEPENDENCIES` is **not** an accepted alias.
The spelling is `REQUIRES` (or `DEPS`, or `Dependencies`); declaring
`DEPENDENCIES` yields an agent with no dependencies and therefore no
ordering guarantee.

### Native bindings

The Python ADK talks to the kernel through gopy-generated bindings under
`adk/python/gapi/native/`. They are **generated, not committed**:

```bash
mage python:build
```

Without them the runner falls back to `DummyAdk`, which writes events to
stdout instead of the bus, and warns on stderr. Set
`ADK_REJECT_DUMMY=1` to make that a hard failure instead -
`productionMode` sets it for you.

### What the ADK exposes

```python
from gapi import Agent, AgentMetadata, AgentDescribe, capability
```

`gapi` exports the protocol types (`Agent`, `InitializeFn`, `StartFn`,
`StopFn`, `ReloadFn`, `RestartFn`), the metadata schemas, and the
`capability` decorator. **There is no `gapi.eventbus`** - agents do not
publish to the bus directly; the runner emits lifecycle events on their
behalf.

## Checkpoint and restore

`core/checkpoint` dumps and restores a running process with CRIU,
preserving memory. It is the mechanism Goblin's live migration moves
between nodes; the kernel supplies no policy of its own.

- Linux only. The non-Linux build returns unsupported.
- The `criu` binary is resolved with `exec.LookPath`; without it,
  `Available()` reports `ErrNoCriu`. It is in the dev shell.
- Requires `CAP_CHECKPOINT_RESTORE` or `CAP_SYS_ADMIN`, so an
  unprivileged process gets a capability error even with criu present.
  Real coverage lives in NixOS VM tests.
- A runner opts in by implementing the optional
  `lifecycle.Checkpointer` interface. `GoAgent` and `PythonAgent` do;
  `TimerAgent` does not, because it has no child process.

## PID 1

Opt-in, with `--pid1` or `supervisor.pid1Mode: true`. There is no
autodetection - `gapid` does not check its own pid. In PID 1 mode it
takes on subreaper duty, the early mount table, watchdog petting and
ordered shutdown. See [pid1-testing.md](pid1-testing.md).

## Transport

QUIC on one listener, routed by TLS ALPN, with a registry that Goblin
extends with its own protocols. The only two transport types are `quic`
and `local`; anything else is a startup error.

`transport.insecureSkipVerify` defaults to **true** for every address.
See [configuration.md](configuration.md).

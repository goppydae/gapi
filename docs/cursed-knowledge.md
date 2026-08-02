# Cursed Knowledge

Lessons learned the hard way. Read this before debugging an
"impossible" issue.

## Signing

### Signing the sidecar instead of the digest

**Symptom**: every signature `gapictl crypto sign` produced failed
verification, so `productionMode` could not start any agent - while the
test suite stayed green.

**Cause**: `sign` signed the bare hex digest; the verifier checked the
signature against the raw bytes of the `.b3` file, which the Magefile
writes with a trailing newline. The same function trimmed for the digest
comparison and did not trim for the signature check.

**Why the tests missed it**: the safety test signed the sidecar's bytes
- it mirrored the verifier rather than the CLI, so it validated the
implementation against itself.

**Fix**: sign and verify the canonical hex digest. If you write a test
for a two-sided protocol, drive each side from its real entry point.

**Ref**: GAPI-DIV-032, `core/crypto/verifybinary_test.go`.

## Configuration

### AutomaticEnv does not make a key reachable

**Symptom**: `GAPI_SUPERVISOR_PRODUCTIONMODE=true gapid` started a
daemon that was *not* in production mode, silently.

**Cause**: viper's `Unmarshal` builds its result from the keys viper
already knows, and it learns a key only from a config file, a default, or
an explicit `BindEnv`. `AutomaticEnv` does not enumerate anything. Every
key that happened to lack a `SetDefault` was therefore invisible - the
whole `supervisor` section, `security.verifyKey`, the TLS paths.

**Fix**: `Load` now derives bindings from the `Config` struct by
reflection, so a field is reachable because it exists rather than because
someone remembered to list it. If you add a config field, do nothing; if
you add a config *shape* that reflection cannot walk, extend the walk.

**Ref**: GAPI-DIV-038, `core/config/envoverride_test.go`.

### Unknown config keys vanish

**Symptom**: a configured TLS certificate is ignored and the daemon
serves an auto-generated self-signed one.

**Cause**: viper drops keys that do not match a `mapstructure` tag,
without complaint. `certFile`/`keyFile` are not the key names;
`tlsCert`/`tlsKey`/`tlsCa` are.

**Fix**: check the spelling against `core/config/config.go`. A silent
downgrade is the failure mode here, not an error.

### Agent metadata written as comments

**Symptom**: a `.py.timer` agent registers, but with default metadata -
no schedule, wrong type, no dependencies.

**Cause**: the Python runner reads metadata with `getattr` on the
imported module. A comment is not an attribute.

```python
# WRONG - read by nothing
# TYPE = "timer"
# SCHEDULE = "@hourly"
```

```python
# RIGHT
TYPE = "timer"
SCHEDULE = "@hourly"
```

### Agent discovery precedence

**Symptom**: a test harness or custom directory is ignored.

**Cause**: the variable is `GAPI_AGENT_PATH`, and it *replaces* the
search path rather than adding to it. There is no `GAPI_AGENTS_DIR` -
that name appears in older docs and is read by nothing.

**Fix**: `GAPI_AGENT_PATH` to force one directory,
`GAPI_SKIP_SYSTEM_AGENTS=1` to drop the package-owned roots (`/etc` and
`/run` survive it), `GAPI_DEV_AGENTS` to add development ones.

**Also**: there is no implicit `./agents` tier any more. A daemon started
from a checkout does not discover that checkout's agents unless
`GAPI_DEV_AGENTS` names the directory. That is deliberate - the old
behaviour made discovery depend on the working directory.

## Event bus

### The prefix-subscription trap

**Symptom**: `proto: mismatched message type` in the logs, even though
the sender is demonstrably publishing the right message.

**Cause**: `SubscribePrefix(scope, namespace, "agent/lifecycle", fn)`
matches *every* topic under that prefix - `agent/lifecycle.action`,
`.control`, `.status` and `.transition`. Those carry different message
types, so the handler tries to unmarshal a `LifecycleStatus` as a
`LifecycleControl` and fails.

**Fix**: subscribe to the exact topic constant unless every sub-topic
shares a schema. The constants live in `core/eventbus/topics.go`.

## Lifecycle

### `exec.CommandContext` kills the agent instantly

**Symptom**: an agent starts and dies immediately, with no error from
the agent itself.

**Cause**: the context passed to `Runner.Start` bounds the **start
operation**, not the process lifetime. It is cancelled the moment the
start call returns, and `exec.CommandContext` hands `os/exec` a watchdog
that SIGKILLs the child at exactly that instant.

**Fix**: `exec.Command`, and let `Stop` own the process.

**Ref**: GAPI-DIV-028, `core/lifecycle/interface.go`.

### A timer fire is bounded at 30 seconds

Each fire runs the agent to completion; the next is not scheduled until
this one returns. `TimerFireTimeout` caps it at 30 seconds, and stopping
the agent cancels a fire in flight. Work that needs longer belongs in a
service, not a timer.

### The systemd prefixes are not interchangeable

`OnUnitActiveSec=D` repeats. `OnStartupSec=D` and `OnBootSec=D` fire
**once**, anchored to the timer's start and the system's boot
respectively. They were aliases until GAPI-DIV-036 - all three collapsed
to a repeating interval - so any schedule written against the old
behaviour now means something different.

A one-shot whose elapse point has already passed fires immediately,
once, rather than being cancelled.

## Build system

### `-mod=mod` is not the answer

**Symptom**: `gopy gen` or the binding build fails to resolve packages
with a `vendor/` directory present.

**Cause**: `go/packages` gets confused by vendoring.

**Do not** reach for `-mod=mod`, which an earlier revision of this file
recommended. It is **illegal in workspace mode**, and this repo commits a
`go.work`. It is also what made `nix/package.nix` unbuildable for its
entire existence.

**Fix**: `GOWORK=off`. The gopy toolchain lives in its own `tools/gopy`
module for the same reason.

**Ref**: GAPI-DIV-033.

### The version you edit is not the version you get

`core/version.GAPIVersion` is overwritten at link time from the root
`VERSION` file. Editing the Go source changes nothing in a built binary.

### Go fixtures in integration tests

**Symptom**: `go build` fails with "no Go files in ...".

**Cause**: pointing at the directory rather than the file when the
fixture is a `main` package.

**Fix**: `fixtures/go/my_agent/main.go`, not the directory.

## Nix

### `eachDefaultSystem` now throws

**Symptom**: `nix flake show` fails after an input update, with an error
about `x86_64-darwin`.

**Cause**: nixpkgs 26.11 dropped `x86_64-darwin`, and
`flake-utils.lib.eachDefaultSystem` still enumerates it - merely
instantiating `pkgs` for that platform throws.

**Fix**: an explicit system list. gapi uses `x86_64-linux`,
`aarch64-linux` and `aarch64-darwin`.

### A NixOS module must not be keyed by system

Defining `nixosModules` inside `eachSystem` produces
`nixosModules.<system>.default`, which no consumer can import - the
module is evaluated by the *consuming* system's module system, not ours.
Keep it outside.

## quic-go API

`quic.Connection` does not exist. The type is `*quic.Conn`, and streams
are already pointers.

```go
// WRONG - undefined
var conn quic.Connection
```

```go
// CORRECT
var conn *quic.Conn
stream, err := conn.AcceptStream(ctx)  // *quic.Stream
io.ReadFull(stream, buf)               // already an io.Reader
```

## Protobuf

### Same directory, same package

Every `.proto` in one directory must declare identical `package` and
`go_package`, or generation produces two Go packages in one directory:

```
found packages goblinv1 (file1.pb.go) and proto (file2.pb.go)
```

### `buf breaking` does not catch renames

The gate is configured with `FILE`, which detects renumbering and
incompatible type changes but not a field *rename* at a stable number.
A rename is wire-compatible and source-breaking - see
[protobuf_compatibility.md](protobuf_compatibility.md).

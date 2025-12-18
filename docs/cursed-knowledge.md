# Cursed Knowledge: Gapi

This file contains lessons learned the hard way. Read this before debugging "impossible" issues.

## EventBus & Protobuf

### The "Prefix Subscription" Trap

**Symptom:** `proto: mismatched message type` errors in logs, even when you verify you are sending the correct message.
**Cause:** Using `SubscribePrefix("agent/lifecycle")` subscribes to *everything* starting with that string, including `agent/lifecycle.action` AND `agent/lifecycle.status`. If these topics carry different Protobuf message types (e.g., `LifecycleControl` vs `LifecycleStatus`), the subscriber will try to unmarshal `Status` as `Control` and fail.
**Fix:** Be specific. Subscription topics should match exactly (e.g., `agent/lifecycle.action`) unless you are certain all sub-topics share a message schema.
**Ref:** `core/supervisor/supervisor.go`

## Configuration & ADK

### Agent Discovery Precedence

**Symptom:** Test harness or custom config ignores `GAPI_AGENTS_DIR`.
**Cause:** The config loader prioritizes `GAPI_AGENT_PATH` (singular) over `GAPI_AGENTS_DIR` or XDG paths. `GAPI_AGENT_PATH` is intended to *replace* the search path entirely.
**Fix:** When testing, use `GAPI_AGENT_PATH` if you want to force a specific directory and ignore system/user paths.

### Cross-ADK Constants

**Symptom:** Agents fail to start or behavior differs between Go/Python agents.
**Cause:** Implicit constants (like default schedule `OnUnitActiveSec=60s` for timers) duplicated across ADKs.
**Fix:** When changing behavior in one ADK (e.g., `adk/go`), you MUST verify parity in `adk/python`. Use `test/adk/cross_adk_test.go` to enforce this.

## Build System

### Gopy & Vendor Directories

**Symptom:** `gopy gen` fails to locate packages or `go build` fails during binding compilation when a `vendor/` directory is present.
**Cause:** `gopy` (and the underlying `go/packages` loader) can get confused by `vendor` directories when generating bindings for modules, sometimes failing to resolve dependencies correctly.
**Fix:** Force module mode by passing `-mod=mod` to `go build` commands when compiling the C-shared library for Python bindings.
**Ref:** `Magefile.go` (see `Python:Build` task).

## Testing

### Go Fixtures in Integration Tests

**Symptom:** `go build` fails in tests with "no Go files in ..."
**Cause:** `cross_adk_test.go` pointing to the directory (package) rather than `main.go` when the fixture is a `main` package.
**Fix:** Point to `fixtures/go/my_agent/main.go`, not just the directory.

## quic-go API: The Type Confusion Chronicles (Dec 2024)

**TL;DR**: `quic.Connection` doesn't exist. Use `*quic.Conn`. Streams are already pointers.

**Why it's cursed**: Documentation shows `Connection`, but only `*quic.Conn` exists in the API.

```go
// ❌ WRONG
var conn quic.Connection  // undefined!

// ✅ CORRECT  
var conn *quic.Conn
stream, err := conn.AcceptStream(ctx)  // stream is *quic.Stream
io.ReadFull(stream, buf)  // Use directly, already implements io.Reader
```

**Discovery Time**: 30+ iterations, 45+ tool calls.

## Protobuf Package Organization

**The Problem**: Multiple `.proto` files in same directory MUST use identical package names.

```protobuf
// Both files must match:
package goblin.v1.proto;
option go_package = "github.com/org/project/internal/proto;goblinv1";
```

**The Error**: `found packages goblinv1 (file1.pb.go) and proto (file2.pb.go)`\
**The Fix**: Align both `package` and `go_package` declarations across all `.proto` files.

# GAPI as a Library

The kernel is the product; `gapid` is a thin wrapper around it. Embed
the supervisor in your own Go binary and you get local agent
supervision without running a second daemon. This is exactly how
`goblind` works.

## What is importable

Everything under `core/` is public API. The packages you are most
likely to touch:

| Package | What it gives you |
| ------- | ----------------- |
| `core/supervisor` | the coordination kernel: `New`, `Run`, `EnablePid1` |
| `core/config` | the `Config` tree and `Load` |
| `core/eventbus` | in-process pub/sub, topic constants |
| `core/lifecycle` | the `Agent`, `Runner` and optional-capability interfaces |
| `core/agentmgr` | discovery and the three runners |
| `core/crypto` | Ed25519, BLAKE3, AGE, capability tokens |
| `core/checkpoint` | CRIU dump and restore |

Everything under `internal/` is not importable from outside the module.
That includes `internal/logattr`, which the in-repo example uses -
substitute your own `slog` attributes when adapting it.

## Embedding

```go
import (
    "context"
    "log"

    "github.com/goppydae/gapi/core/config"
    "github.com/goppydae/gapi/core/supervisor"
)
```

Configuration either comes from the usual search path:

```go
cfg, err := config.Load()
```

or is constructed directly, when your application owns its own config
format:

```go
cfg := &config.Config{
    Transport: config.TransportConfig{Type: "quic"},
}
```

Building a `Config` by hand skips `Load`, and therefore skips the viper
defaults. Set anything you care about explicitly - in particular
`Transport.Address`, the `Timeouts` block, and
`Supervisor.ProductionMode`, whose zero value is `false`.

Then start it:

```go
sup, err := supervisor.New(cfg)
if err != nil {
    log.Fatal(err)
}

ctx, cancel := context.WithCancel(context.Background())
defer cancel()

if err := sup.Run(ctx); err != nil {
    log.Fatal(err)
}
```

`Run` blocks until the context is cancelled. Cancelling it is the
shutdown path; the supervisor stops its agents in order before
returning.

## Shutdown requests from inside

`gapictl shutdown` publishes on the bus rather than signalling the
process, so an embedder that wants to honour it reads the channel:

```go
go func() {
    <-sup.ShutdownRequests()
    cancel()
}()
```

Ignore that channel and `gapictl shutdown` does nothing to your binary.

## PID 1

If your binary is going to be init, take the PID 1 path instead of the
plain `Run`:

```go
complete, err := sup.EnablePid1(ctx)
if err != nil {
    log.Fatal(err)
}
if err := sup.Run(ctx); err != nil {
    log.Fatal(err)
}
complete(shutdown.PowerOff)
```

`EnablePid1` installs the signal handlers, the subreaper and the early
mount phase; `complete` runs the sync, unmount and reboot sequence. See
[pid1-testing.md](pid1-testing.md).

## Building against it

The repo commits a `go.work` for sibling development. A consumer outside
that layout resolves the published tag from the module proxy as usual,
and needs no special flags. Inside the silo, set `GOWORK=off` when you
want the pinned version rather than the sibling checkout.

## Example

[examples/standalone/main.go](../examples/standalone/main.go) is a
complete, compiling program: programmatic config, `supervisor.New`, and
a `Run` bounded by a five-second context.

```bash
go run ./examples/standalone
```

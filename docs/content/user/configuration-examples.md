---
title: "Configuration Examples"
weight: 40
---

Worked setups, and the things a key table cannot tell you. Every key,
its type, its default and the environment variable that overrides it are
published in the generated
[configuration reference](../../reference/configuration/), which is
produced from the same schema and the same defaults the daemon loads.
This page does not repeat that table; it shows configurations that do
something, and records the traps.

gapi needs no config file: every setting has a default. These examples
change specific behaviour.

## Minimal

Nothing at all. `gapid start` binds the default control address, logs
JSON at info, and generates an ephemeral self-signed certificate. The
default is LOOPBACK: binding every interface is a decision an operator
makes, not one they inherit.

```bash
gapid start
```

The daemon root carries no action of its own, so a bare `gapid` prints
help and exits non-zero. `start` is the verb that runs it.

## Development: console logs, debug level

```yaml
logging:
  level: debug
  format: console
```

Or without a file at all:

```bash
GAPI_LOGGING_LEVEL=debug GAPI_LOGGING_FORMAT=console gapid start
```

Console output comes from the stdlib `slog` text handler:

```
time=2026-07-29T10:14:03.221Z level=INFO msg="supervisor running"
```

JSON output carries `time`, `level` (uppercase) and `msg`:

```json
{"time":"2026-07-29T10:14:03.221Z","level":"INFO","msg":"supervisor running"}
```

## Production: real TLS, verified agents

```yaml
transport:
  type: quic
  address: "0.0.0.0:29979"
  tlsCert: /etc/gapi/server.crt
  tlsKey: /etc/gapi/server.key
  tlsCa: /etc/gapi/ca.crt
  # Without this, gapi does not verify peers - the default is true.
  insecureSkipVerify: false

security:
  verifyKey: /etc/gapi/agent-signing.pub.hex

supervisor:
  # Refuses to start any agent BINARY whose signature does not verify,
  # and makes a missing Python ADK a hard failure. It does NOT enforce
  # TLS - set transport.insecureSkipVerify to false yourself.
  productionMode: true

logging:
  level: info
  format: json
  file:
    enabled: true
    path: /var/log/gapi/gapi.log
    maxSize: 100
    maxBackups: 3
    maxAge: 28
    compress: true
```

## Metrics

Disabled by default.

```yaml
metrics:
  enabled: true
  addr: 127.0.0.1:10973
```

## Timeouts

All strings, parsed as Go durations.

```yaml
timeouts:
  quicStream: "10s"
  quicIdle: "60s"
  clientPending: "2s"
  clientTerminal: "20s"
  supervisorStart: "20s"
  supervisorShutdown: "5s"
```

## PID 1 (running as init)

Off by default; `gapid` behaves as an ordinary supervisor unless told
otherwise. There is no autodetection of `getpid() == 1`.

```yaml
supervisor:
  pid1Mode: true
  noEarlyMounts: false      # true inside a container: the runtime owns mounts
  watchdog:
    enabled: true
    device: /dev/watchdog
    interval: "10s"
  shutdown:
    gracePeriod: "10s"
```

Equivalently, by flag. **`--pid1` and `--no-early-mounts` belong to
`start`, not to the root** - passing either to a bare `gapid` is a cobra
usage error, not a silent no-op:

```bash
gapid start --pid1
```

```bash
gapid start --pid1 --no-early-mounts
```

## Loki

Not implemented. The keys exist in the schema, but enabling them is a
**hard startup failure**, by design - the alternative would be silently
dropping logs an operator believes are being shipped:

```
loki output is enabled but not implemented; disable logging.loki.enabled
or remove the loki configuration
```

Use the file sink and ship the file, until this is built.

## Environment-only configuration

Every key can be set without a file. Prefix `GAPI_`, uppercase, dots
to underscores:

```bash
GAPI_TRANSPORT_ADDRESS=:15000 GAPI_LOGGING_LEVEL=warn GAPI_METRICS_ENABLED=true gapid start
```

That includes the `supervisor` section and the TLS paths:

```bash
GAPI_SUPERVISOR_PRODUCTIONMODE=true GAPI_TRANSPORT_TLSCERT=/etc/gapi/server.crt gapid start
```

The prefix belongs to the PRODUCT and is composed from its identity at
startup, so it is `GAPI_` here and `GOBLIN_` in the orchestrator that
embeds the same kernel. It was the literal `RUNTIME` before
GAPI-DIV-059; that spelling is read by nothing now.

Precedence is environment over config file over default. Only maps have
no environment spelling; `logging.loki.labels` is config-file only.

Bindings are derived from the config struct itself, so every field is
settable by environment variable, and a test walks the struct to keep it
that way.

## Pointing at a specific file

```bash
GAPI_CONFIG=/opt/gapi/prod.yaml gapid start
```

A release build otherwise reads `/etc/gapi/config.yaml` and nothing
else. A build tagged `dev` also searches `config/` and the current
directory. There is no `--config` flag; the daemon's whole flag surface
is published in the [command reference](../../reference/cli/gapid/gapid/).

## Environment variables gapi reads

These are read directly rather than through the config schema, so they
are not in the generated key table. As above, the `GAPI_` prefix is the
product's identity: under another product each row reads `<PREFIX>_`.

| Variable | Purpose |
| -------- | ------- |
| `GAPI_CONFIG` | Path to the config file |
| `GAPI_<SECTION>_<KEY>` | Override any config key |
| `GAPI_AGENT_PATH` | Override the agent search root |
| `GAPI_DEV_AGENTS` | Highest-priority agent directory, in either scope |
| `GAPI_SKIP_SYSTEM_AGENTS` | Drop the package-owned agent tiers (`/usr/lib`, `/usr/local/lib`); `/etc` and `/run` survive |
| `GAPI_VERIFY_KEY` | Agent signing public key (fallback for `security.verifyKey`) |
| `GAPI_PY_ADK` | Override the Python ADK source tree (the directory holding `agent/` and `gapi/`) |
| `GAPI_GO_ADK` | Override the Go ADK source tree used by `gapictl agent build` |
| `CGO_ENABLED` | Honoured by `gapictl agent build`'s staged build, which otherwise sets `0`. `--cgo` beats it |
| `GAPI_CGROUPS_DISABLE` | Disable cgroup setup |
| `GAPI_KMSG_PATH` | Override the kmsg device (PID 1 mode) |

There is no `GAPI_AGENTS_DIR`, `GAPI_LOG_LEVEL` or `GAPI_TRACE_EVENTS`.
Those names appear in older documentation and are read by nothing.
`GAPI_AGENT_PATH` is real and listed above.

## What the key table does not tell you

### Transport

Only two types exist: `quic` and `local` (`core/transport/factory.go`).
Setting anything else - `tcp`, `unix` - is a hard startup error.

The TLS keys are `tlsCert`, `tlsKey` and `tlsCa`. Older documentation
used `certFile`/`keyFile`; viper drops unknown keys **silently**, so
that spelling meant the daemon ignored the configured certificate and
generated a throwaway self-signed one instead - a silent downgrade
rather than an error.

> **Warning**: `transport.insecureSkipVerify` defaults to **true**, for
> every address, not just loopback. gapi does not verify peer
> certificates unless you set it to `false`. Setting
> `supervisor.productionMode: true` does **not** cover this - production
> mode gates agent signature verification and nothing else, so the two
> settings must be made independently.

### Logging

JSON output comes from the stdlib `slog` JSON handler, so records carry
`time`, `level` (uppercase) and `msg`.

### Agent signature verification

`security.verifyKey` points at an Ed25519 public key. Either
`GAPI_SECURITY_VERIFYKEY` (the ordinary override) or `GAPI_VERIFY_KEY`
sets it; the latter is a separate `os.Getenv` in the supervisor,
consulted when the config key is empty, and predates the override
covering every key.

Verification is gated on **production mode**, not on the key being
present: with `supervisor.productionMode: true`, an agent binary must
carry a valid `.b3` digest and `.sig` or it will not start. In
production mode with no verify key configured, discovery rejects every
binary - fail closed.

Production mode does exactly two things: this gate, and setting
`ADK_REJECT_DUMMY` for Python agents. It does not touch TLS, the listen
address, or anything else.

Note that only binary agents are verified. Python agents are described
by running the interpreter, which does not go through the signature
path.

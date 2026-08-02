# GAPI Configuration Examples

Worked examples. For the full key reference see
[configuration.md](configuration.md).

GAPI needs no config file: every setting has a default. These examples
change specific behaviour.

## Minimal

Nothing at all. `gapid` binds `:14242`, logs JSON at info, and generates
an ephemeral self-signed certificate.

```bash
gapid
```

## Development: console logs, debug level

```yaml
logging:
  level: debug
  format: console
```

Or without a file at all:

```bash
GAPI_LOGGING_LEVEL=debug GAPI_LOGGING_FORMAT=console gapid
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
  address: "0.0.0.0:14242"
  tlsCert: /etc/gapi/server.crt
  tlsKey: /etc/gapi/server.key
  tlsCa: /etc/gapi/ca.crt
  # Without this, GAPI does not verify peers - the default is true.
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
  addr: 127.0.0.1:19090
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

Equivalently, by flag:

```bash
gapid --pid1
```

```bash
gapid --pid1 --no-early-mounts
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
GAPI_TRANSPORT_ADDRESS=:15000 GAPI_LOGGING_LEVEL=warn GAPI_METRICS_ENABLED=true gapid
```

The prefix is `RUNTIME`, not `GAPI`. That includes the `supervisor`
section and the TLS paths:

```bash
GAPI_SUPERVISOR_PRODUCTIONMODE=true GAPI_TRANSPORT_TLSCERT=/etc/gapi/server.crt gapid
```

Only maps have no environment spelling; `logging.loki.labels` is
config-file only.

## Pointing at a specific file

```bash
GAPI_CONFIG=/opt/gapi/prod.yaml gapid
```

A release build otherwise reads `/etc/gapi/config.yaml` and nothing
else. There is no `--config` flag.

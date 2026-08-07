---
title: "gapid start"
---

## gapid start

Start the gapid daemon

```
gapid start [flags]
```

### Options

```
  -h, --help                 help for start
      --listen-addr string   Control-plane bind address (default: 127.0.0.1:29979)
      --no-early-mounts      Skip the Phase 0 mount table (container environments)
      --pid1                 Run as PID 1: Phase 0 pre-userspace boot (subreaper, signals, mounts, reaping)
```

### Options inherited from parent commands

```
      --id string             Unique node identifier (default: hostname)
      --log-file string       Additional rotated file sink
      --log-format string     Log format: json or console
      --log-level string      Log level: debug, info, warn, error
      --log-loki-url string   Forward logs to a Loki endpoint
      --metrics-addr string   Prometheus listen address (empty: disabled)
      --tls-ca string         CA certificate for peer verification
      --tls-cert string       This node's certificate
      --tls-key string        This node's private key
  -v, --version               version for gapid
```

### SEE ALSO

* [gapid](./gapid/)	 - Supervision kernel daemon


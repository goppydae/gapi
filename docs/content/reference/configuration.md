---
title: "Configuration"
description: "Every configuration key, its default, and its environment override"
weight: 20
---

Every key below is settable in the configuration file and overridable
from the environment. This page is generated from the same schema and the
same defaults `gapi` itself loads, so it cannot disagree with the binary.

| Key | Type | Default | Environment |
|---|---|---|---|
| `logging.file.compress` | `bool` | `true` | `GAPI_LOGGING_FILE_COMPRESS` |
| `logging.file.enabled` | `bool` | `false` | `GAPI_LOGGING_FILE_ENABLED` |
| `logging.file.maxAge` | `int` | `28` | `GAPI_LOGGING_FILE_MAXAGE` |
| `logging.file.maxBackups` | `int` | `3` | `GAPI_LOGGING_FILE_MAXBACKUPS` |
| `logging.file.maxSize` | `int` | `100` | `GAPI_LOGGING_FILE_MAXSIZE` |
| `logging.file.path` | `string` | `/var/log/gapi/gapi.log` | `GAPI_LOGGING_FILE_PATH` |
| `logging.format` | `string` | `json` | `GAPI_LOGGING_FORMAT` |
| `logging.level` | `string` | `info` | `GAPI_LOGGING_LEVEL` |
| `logging.loki.enabled` | `bool` | `false` | `GAPI_LOGGING_LOKI_ENABLED` |
| `logging.loki.url` | `string` | (none) | `GAPI_LOGGING_LOKI_URL` |
| `metrics.addr` | `string` | `127.0.0.1:10973` | `GAPI_METRICS_ADDR` |
| `metrics.enabled` | `bool` | `false` | `GAPI_METRICS_ENABLED` |
| `security.verifyKey` | `string` | (none) | `GAPI_SECURITY_VERIFYKEY` |
| `supervisor.noEarlyMounts` | `bool` | `false` | `GAPI_SUPERVISOR_NOEARLYMOUNTS` |
| `supervisor.pid1Mode` | `bool` | `false` | `GAPI_SUPERVISOR_PID1MODE` |
| `supervisor.productionMode` | `bool` | `false` | `GAPI_SUPERVISOR_PRODUCTIONMODE` |
| `supervisor.shutdown.gracePeriod` | `string` | (none) | `GAPI_SUPERVISOR_SHUTDOWN_GRACEPERIOD` |
| `supervisor.watchdog.device` | `string` | (none) | `GAPI_SUPERVISOR_WATCHDOG_DEVICE` |
| `supervisor.watchdog.enabled` | `bool` | `false` | `GAPI_SUPERVISOR_WATCHDOG_ENABLED` |
| `supervisor.watchdog.interval` | `string` | (none) | `GAPI_SUPERVISOR_WATCHDOG_INTERVAL` |
| `timeouts.clientPending` | `string` | `2s` | `GAPI_TIMEOUTS_CLIENTPENDING` |
| `timeouts.clientTerminal` | `string` | `20s` | `GAPI_TIMEOUTS_CLIENTTERMINAL` |
| `timeouts.quicIdle` | `string` | `1m0s` | `GAPI_TIMEOUTS_QUICIDLE` |
| `timeouts.quicStream` | `string` | `10s` | `GAPI_TIMEOUTS_QUICSTREAM` |
| `timeouts.supervisorShutdown` | `string` | `5s` | `GAPI_TIMEOUTS_SUPERVISORSHUTDOWN` |
| `timeouts.supervisorStart` | `string` | `20s` | `GAPI_TIMEOUTS_SUPERVISORSTART` |
| `transport.address` | `string` | `127.0.0.1:29979` | `GAPI_TRANSPORT_ADDRESS` |
| `transport.insecureSkipVerify` | `bool` | `true` | `GAPI_TRANSPORT_INSECURESKIPVERIFY` |
| `transport.tlsCa` | `string` | (none) | `GAPI_TRANSPORT_TLSCA` |
| `transport.tlsCert` | `string` | (none) | `GAPI_TRANSPORT_TLSCERT` |
| `transport.tlsKey` | `string` | (none) | `GAPI_TRANSPORT_TLSKEY` |
| `transport.type` | `string` | `quic` | `GAPI_TRANSPORT_TYPE` |

## Notes

**`logging.file.maxAge`**
: Days

**`logging.file.maxBackups`**
: Number of old files to keep

**`logging.file.maxSize`**
: MB

**`logging.format`**
: json, console

**`logging.level`**
: trace, debug, info, warn, error

**`security.verifyKey`**
: Path to public key

**`supervisor.noEarlyMounts`**
: NoEarlyMounts skips the mount phase (the OCI runtime owns mounts in a container).

**`supervisor.pid1Mode`**
: Pid1Mode activates the Phase-0 pre-userspace boot sequence (subreaper, PID-1 signals, kmsg, early mounts). Off by default: gapid runs as an ordinary supervisor unless it IS init.


# GAPI Configuration Example

## Complete Configuration with Logging

```yaml
# Transport Configuration
transport:
  type: quic
  address: ":4242"
  certFile: "/path/to/cert.pem"  # Optional
  keyFile: "/path/to/key.pem"    # Optional

# Security Configuration
security:
  verifyKey: "/path/to/public.key"  # Optional: Enable integrity verification

# Metrics Configuration
metrics:
  enabled: true
  addr: "127.0.0.1:9090"

# Logging Configuration
logging:
  level: "info"      # trace, debug, info, warn, error
  format: "json"     # json or console
  
  # File Output (Optional)
  file:
    enabled: true
    path: "/var/log/gapi/gapi.log"
    maxSize: 100      # MB
    maxBackups: 3     # Number of old files to keep
    maxAge: 28        # Days
    compress: true    # Compress rotated files
  
  # Loki Output (Optional)
  loki:
    enabled: false
    url: "http://loki:3100/loki/api/v1/push"
    labels:
      app: "gapi"
      env: "production"
      host: "server1"
```

______________________________________________________________________

## Minimal Configuration (Defaults)

```yaml
# Everything uses defaults
# Logs to stdout in JSON format at Info level
```

______________________________________________________________________

## Development Configuration

```yaml
logging:
  level: "debug"
  format: "console"  # Human-readable output
```

______________________________________________________________________

## Production Configuration

```yaml
logging:
  level: "info"
  format: "json"
  
  file:
    enabled: true
    path: "/var/log/gapi/gapi.log"
    maxSize: 100
    maxBackups: 5
    maxAge: 30
    compress: true

metrics:
  enabled: true
  addr: "127.0.0.1:9090"
```

______________________________________________________________________

## Kubernetes Configuration

```yaml
# Let K8s handle log collection
logging:
  level: "info"
  format: "json"
  # No file output - logs go to stdout
  # K8s log drivers collect from stdout
```

______________________________________________________________________

## Grafana Stack Configuration

```yaml
logging:
  level: "info"
  format: "json"
  
  loki:
    enabled: true
    url: "http://loki:3100/loki/api/v1/push"
    labels:
      app: "gapi"
      env: "production"

metrics:
  enabled: true
  addr: "127.0.0.1:9090"
```

______________________________________________________________________

## Environment Variable Overrides

```bash
# Override log level
export GAPI_LOGGING_LEVEL=debug

# Override log format
export GAPI_LOGGING_FORMAT=console

# Enable file logging
export GAPI_LOGGING_FILE_ENABLED=true
export GAPI_LOGGING_FILE_PATH=/custom/path/gapi.log

# Enable metrics
export GAPI_METRICS_ENABLED=true
```

______________________________________________________________________

## Log Levels

- **trace**: Very detailed, function entry/exit (future)
- **debug**: Operational details, event flow, configuration
- **info**: Key events, startup, shutdown, state changes (default)
- **warn**: Recoverable errors, performance issues
- **error**: Failures, unrecoverable errors

______________________________________________________________________

## Log Formats

### JSON (Default)

```json
{"level":"info","stream":"runtime","time":"2024-01-15 10:30:45","message":"supervisor running","host":"server1"}
```

### Console (Development)

```
10:30:45 INF supervisor running host=server1 stream=runtime
```

______________________________________________________________________

## File Rotation

When file logging is enabled:

- **maxSize**: Rotate when file reaches this size (MB)
- **maxBackups**: Keep this many old log files
- **maxAge**: Delete files older than this (days)
- **compress**: Compress rotated files with gzip

Example rotation:

```
/var/log/gapi/
├── gapi.log           # Current log
├── gapi-2024-01-14.log.gz
├── gapi-2024-01-13.log.gz
└── gapi-2024-01-12.log.gz
```

______________________________________________________________________

## Multi-Output Behavior

GAPI supports multiple simultaneous outputs:

1. **stdout** (always enabled)
1. **file** (optional, with rotation)
1. **loki** (optional, for Grafana)

All enabled outputs receive the same log messages.

______________________________________________________________________

## Best Practices

### Development

```yaml
logging:
  level: "debug"
  format: "console"
```

### Production (Bare Metal)

```yaml
logging:
  level: "info"
  format: "json"
  file:
    enabled: true
    path: "/var/log/gapi/gapi.log"
    maxSize: 100
    maxBackups: 5
    compress: true
```

### Production (Docker/K8s)

```yaml
logging:
  level: "info"
  format: "json"
  # stdout only - let container runtime handle collection
```

### Production (Grafana Stack)

```yaml
logging:
  level: "info"
  format: "json"
  loki:
    enabled: true
    url: "http://loki:3100"
    labels:
      app: "gapi"
      env: "prod"
```

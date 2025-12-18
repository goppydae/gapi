# Configuration Guide

This document explains how to configure GAPI and define agent metadata.

## Supervisor Configuration

The supervisor (`gapid`) is configured via a `config.yaml` file.

### Configuration File Location

By default, `gapid` looks for `config.yaml` in:

1. Current working directory
1. `$XDG_CONFIG_HOME/gapi/config.yaml`
1. `$HOME/.config/gapi/config.yaml`

You can override this with the `--config` flag:

```bash
gapid --config /path/to/config.yaml
```

### Configuration Structure

```yaml
transport:
  type: quic              # Transport type: "quic" or "tcp"
  address: 127.0.0.1:4242 # Listen address
  certFile: config/certs/server.crt  # TLS certificate (QUIC only)
  keyFile: config/certs/server.key   # TLS private key (QUIC only)

security:
  verifyKey: path/to/public.key  # Optional: Ed25519 public key for signature verification

agents:
  directory: ./agents     # Directory to scan for agents (default: ./agents)
  scanInterval: 30s       # How often to rescan for new agents (default: 30s)
```

### Transport Options

#### QUIC Transport (Recommended)

QUIC provides multiplexed, encrypted communication:

```yaml
transport:
  type: quic
  address: 127.0.0.1:4242
  certFile: config/certs/server.crt
  keyFile: config/certs/server.key
```

Generate self-signed certificates:

```bash
openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt -days 365 -nodes
```

#### TCP Transport

For simpler setups or debugging:

```yaml
transport:
  type: tcp
  address: 127.0.0.1:4242
```

**Note**: TCP transport does not provide encryption. Use only in trusted environments.

### Security Configuration

#### Signature Verification

To enforce agent signature verification:

1. **Generate a keypair**:

   ```bash
   gapictl keygen signing-key
   ```

1. **Configure the public key**:

   ```yaml
   security:
     verifyKey: signing-key.pub
   ```

1. **Sign your agents**:

   ```bash
   gapictl sign agents/myagent.py.service signing-key.key
   ```

When `verifyKey` is set, `gapid` will only load agents with valid signatures.

## Agent Metadata

Agents are configured via metadata directives in comments at the top of the file.

### Required Directives

```python
ENABLED = True
TYPE = "service"
```

- **`ENABLED`**: Must be `True` for the agent to be loaded
- **`TYPE`**: Agent type: `service`, `timer`, or `socket`

### Optional Directives

#### Dependencies

Specify other agents that must start before this one:

```python
DEPENDENCIES = ["database", "cache", "logging"]
```

Dependencies are a list of agent IDs (filename without extension).

#### Resource Limits

Enforce CPU and memory limits:

```python
CPU_LIMIT = 0.5        # 50% of one CPU core
MEMORY_LIMIT = "512MB"   # 512 megabytes
```

Supported units for memory: `KB`, `MB`, `GB`

#### Timer Configuration

For `TYPE = "timer"` agents:

```python
SCHEDULE = "OnUnitActiveSec=5m"
```

See [Features - Timer Agents](features.md#timer-agents) for schedule format details.

#### Socket Configuration

For `TYPE = "socket"` agents:

```python
LISTEN_STREAM = "0.0.0.0:8080"      # TCP socket
# LISTEN_DATAGRAM = "0.0.0.0:5353"    # UDP socket
```

### Complete Example

```python
# agents/api-server.py.service
ENABLED = True
TYPE = "socket"
DEPENDENCIES = ["database", "cache"]
CPU_LIMIT = 1.0
MEMORY_LIMIT = "1GB"
LISTEN_STREAM = "0.0.0.0:8080"

def start():
    import os
    import socket
    
    fd = int(os.environ.get("LISTEN_FDS", "0"))
    if fd > 0:
        sock = socket.fromfd(fd + 3, socket.AF_INET, socket.SOCK_STREAM)
        # Handle connections
```

## Environment Variables

GAPI sets the following environment variables for agents:

### Standard Variables

- **`AGENT_ID`**: The agent's unique identifier (filename without extension)
- **`AGENT_TYPE`**: The agent type (`service`, `timer`, or `socket`)
- **`GAPI_SOCKET`**: Path to the GAPI event bus socket

### Socket Activation Variables

For socket-activated agents:

- **`LISTEN_FDS`**: Number of file descriptors passed to the agent
- **`LISTEN_PID`**: PID of the process (for verification)

File descriptors start at `fd 3` (after stdin=0, stdout=1, stderr=2).

### Custom Variables

You can define custom environment variables in agent metadata:

```python
ENV_DATABASE_URL = "postgresql://localhost/mydb"
ENV_API_KEY = "secret123"
```

Variables prefixed with `ENV_` are set in the agent's environment (with the `ENV_` prefix removed).

## Client Configuration

The `gapictl` client uses the same `config.yaml` to connect to the supervisor.

### Minimal Client Config

```yaml
transport:
  type: quic
  address: 127.0.0.1:4242
  certFile: config/certs/server.crt
  keyFile: config/certs/server.key
```

The client only needs the transport configuration to connect.

### Remote Connections

To connect to a remote supervisor:

```yaml
transport:
  type: quic
  address: remote-host:4242
  certFile: config/certs/server.crt
  keyFile: config/certs/server.key
```

Ensure the certificate is valid for the remote hostname.

> [!NOTE]
> For local development, GAPI supports anonymous QUIC connections (no client certificate required) when connecting to loopback addresses (`127.0.0.1`, `::1`).

## Configuration Validation

Validate your configuration:

```bash
gapid --config config.yaml --validate
```

This checks for:

- Valid YAML syntax
- Required fields present
- Certificate files exist (for QUIC)
- Public key file exists (if signature verification enabled)

---
title: "gapictl agent"
---

## gapictl agent

Manage agents

### Synopsis

Build, sign, and manage agents across Python and Go.

### Options

```
  -h, --help   help for agent
```

### Options inherited from parent commands

```
      --control-addr string   Address of the target daemon's control plane (empty: from config)
      --log-level string      Log level: debug, info, warn, error
      --tls-ca string         CA certificate used to verify the daemon
      --tls-insecure          Skip verification (INSECURE)
  -v, --version               version for gapictl
```

### SEE ALSO

* [gapictl](./gapictl/)	 - Supervision kernel control CLI
* [gapictl agent build](./gapictl_agent_build/)	 - Build Go agents
* [gapictl agent clean](./gapictl_agent_clean/)	 - Clean build artifacts
* [gapictl agent new](./gapictl_agent_new/)	 - Create a new agent from template
* [gapictl agent reload](./gapictl_agent_reload/)	 - Trigger a reload of registered agents
* [gapictl agent status](./gapictl_agent_status/)	 - Show current registered agents
* [gapictl agent verify](./gapictl_agent_verify/)	 - Verify agent binary integrity and authenticity


---
title: "gapictl agent new"
---

## gapictl agent new

Create a new agent from template

### Synopsis

Create a new agent from template with proper structure.

Examples:
  gapictl agent new my_service
  gapictl agent new --type=timer my_timer
  gapictl agent new --lang=python --type=service my_py_service

```
gapictl agent new [name] [flags]
```

### Options

```
  -h, --help            help for new
  -l, --lang string     Agent language (go, python) (default "go")
  -o, --output string   Output directory (default: agents/{lang}/foundational or agents/{lang}/services)
  -t, --type string     Agent type (service, socket, timer) (default "service")
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

* [gapictl agent](./gapictl_agent/)	 - Manage agents


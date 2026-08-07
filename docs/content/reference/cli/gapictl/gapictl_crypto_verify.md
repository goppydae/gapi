---
title: "gapictl crypto verify"
---

## gapictl crypto verify

Verify a file against its .sig

```
gapictl crypto verify <file> --pub <public.hex> [flags]
```

### Options

```
  -h, --help         help for verify
      --pub string   Path to public key (hex)
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

* [gapictl crypto](./gapictl_crypto/)	 - Cryptography utilities


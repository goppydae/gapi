---
title: "gapictl crypto"
---

## gapictl crypto

Cryptography utilities

### Options

```
  -h, --help   help for crypto
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
* [gapictl crypto age-keygen](./gapictl_crypto_age-keygen/)	 - Generate a new AGE identity
* [gapictl crypto decrypt](./gapictl_crypto_decrypt/)	 - Decrypt data from stdin to stdout (AGE)
* [gapictl crypto encrypt](./gapictl_crypto_encrypt/)	 - Encrypt data from stdin to stdout (AGE)
* [gapictl crypto keygen](./gapictl_crypto_keygen/)	 - Generate a new Ed25519 keypair
* [gapictl crypto sign](./gapictl_crypto_sign/)	 - Sign a file and produce a detached .sig
* [gapictl crypto verify](./gapictl_crypto_verify/)	 - Verify a file against its .sig


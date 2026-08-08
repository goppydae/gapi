---
title: "gapi"
---

GoPPydae Agent Process Infrastructure: a single-node agent supervision
kernel. It runs as PID 1, supervises agents through their lifecycle, and
carries the transport, configuration and cryptographic mechanisms the rest
of the ecosystem builds on.

gapi is **mechanism**. Membership, consensus and scheduling are policy and
live in the orchestrator, which embeds this kernel rather than replacing
it. The line between the two is the reason both exist.

## What is here

Everything under [Reference](reference/) is **generated from source** and
gated: `mage docs:check` regenerates it into a temporary tree and
byte-compares, so a page here describes the binary that was built rather
than the one somebody remembered. The command pages come from the cobra
trees the binaries actually run; the configuration reference is a join of
the registered defaults and a reflection walk over the config schema.

- [Command reference](reference/cli/) - every verb and flag of `gapid` and
  `gapictl`.
- [Configuration reference](reference/configuration/) - every key, its
  default, and the environment variable that overrides it.

Everything under [Using gapi](user/) is hand-written prose, and is the
half a generator cannot produce: what the kernel is, how to bring it up,
and how to author an agent against it.

- [Overview](user/overview/) - what the kernel does, in prose.
- [Getting started](user/getting-started/) - build and run your first agent.
- [Installation](user/installation/) - NixOS module, machine images, systemd.
- [Configuration examples](user/configuration-examples/) - worked setups,
  and the traps the key table cannot state.
- [Agent metadata](user/agent-metadata/) - every field an agent declares,
  and what the supervisor hands it at exec.
- [Agent examples](user/agent-examples/) - services, timers, sockets, Go agents.
- [Library usage](user/library-usage/) - embedding the kernel in your own
  binary.

Man pages are generated from the same sources and ship with the package:
`gapid(1)`, `gapictl(1)`, `gapi.conf(5)` and `gapi(7)`.

## What is not here

The design documents, the engineering manifestos and the research notes
live in the ecosystem documentation rather than beside the code, so that no
code repository owns a document governing the whole ecosystem. This site is
the kernel's own reference and its operator-facing prose.

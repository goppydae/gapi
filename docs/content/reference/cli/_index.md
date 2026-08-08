---
title: "Command Reference"
weight: 10
---

Generated from the cobra trees the binaries run, not from a second
declaration of them.

- **[gapid](gapid/gapid/)** - the supervision daemon. Runs as PID 1 or
  under an existing init, and holds the agent lifecycle.
- **[gapictl](gapictl/gapictl/)** - the control client. Speaks to a running
  daemon over the control transport, and is the only cross-platform binary
  of the two.

Two details of how these pages are produced are worth knowing, because
both were defects before they were details.

`gapictl`'s tree is walked from the **populated** root rather than from
its constructor. Five `func init()` blocks register the verbs against a
package-level singleton, so a root built by the constructor carries two
commands where the real one carries 25 - and a reference generated from
it would have omitted every agent, crypto and lifecycle verb while a drift
gate held the omission steady forever.

`gapid start` is documented with a **non-executable action**. A cobra
command whose `RunE` is nil reports itself as unavailable, and the
generator skips unavailable commands in silence - which once dropped the
daemon's central verb and all nine of its flags from both the reference
and the man pages, with no error and no empty output to notice.

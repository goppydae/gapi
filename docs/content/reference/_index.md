---
title: "Reference"
weight: 20
---

Generated reference for the kernel's operator-facing surface. Nothing in
this section is hand-written.

Every page is produced by `mage docs:generate` and held to its source by
`mage docs:check`, which runs inside `mage lint` on every pull request.
The check regenerates into a temporary tree and byte-compares rather than
regenerating in place: a gate that repairs the drift it is measuring
passes on its second run and the defect is gone before anyone reads the
message.

The check reports three conditions, and the third is the one worth having.
*Stale* means the source moved and the page did not. *Missing* means a
page is declared under drift control that generation no longer produces.
*Untracked* means generation produces a file nothing gates - which is the
state this whole section existed in until the generator acquired a caller.

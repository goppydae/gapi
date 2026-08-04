# Shared-object plugins: not implemented

This directory documented a shared-object plugin loader for in-process
agent execution. It was never built, and as of 2026-08-04 it will not be.

Nothing in this repository loads a shared object. There is no plugin
import, no `plugin.Open` call, and no `.so` loading anywhere in the tree.
The document that stood here specified an agent interface, an activation
switch, ABI and crash-isolation limits, and a worked verification command,
none of which had an implementation behind them.

## Why it is retired rather than pending

The operator's decision, 2026-08-04: the available implementation avenues
are flaky to broken. This is recorded as a decision, not as an argument -
it is not an invitation to relitigate the tradeoffs.

## What to use instead

Agents run as supervised processes, discovered through the tiered agent
search path. See `core/config/agent_paths.go` for the search order and
`core/agentmgr/discovery.go` for how a discovered agent is admitted.

## For the record

GAPI-DIV-060 tracked the gap between this document and the code. It closed
by retiring the document. If in-process execution is ever wanted again, it
starts from a new design, not from this file's history.

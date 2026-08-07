---
title: "Overview"
weight: 1
---

# gapi

gapi supervises agents on a single machine. It runs either as a standalone
init system, holding PID 1 and bringing up userspace, or embedded inside a
larger orchestrator that wants per-node supervision without reimplementing
it.

An agent here is a self-contained service, not a static unit definition. It
declares what it is, reports what it is doing, and can be started, stopped,
reloaded and inspected while the machine is running.

## The two binaries

**gapid** is the daemon. It owns the agent lifecycle, the control plane, and
under `--pid1` the pre-userspace boot sequence: subreaping, signal handling,
early mounts and process reaping. It is Linux-only, because those are Linux
facilities.

**gapictl** is the control client. It talks to a running gapid over the
control plane and drives everything an operator does: querying status,
starting and stopping agents, building and signing them, and an interactive
terminal interface. It is cross-platform, and deliberately so - the machine
an operator sits at is not always the machine being supervised.

## How an agent reaches the supervisor

An agent's control channel is an inherited file descriptor, handed to it at
exec. The agent writes typed events; the supervisor marshals them and
publishes them onto its event bus. There is no connection for the agent to
open and no window during which its events have nowhere to go.

Standard output carries logs and nothing else. An agent describes itself
through a separate invocation rather than by printing a descriptor onto the
same stream its logs use.

## Configuration

Configuration is read from a file in the search path, and every key may be
overridden by an environment variable. Precedence is environment, then file,
then the built-in default. The key list, the defaults and the variable names
are generated from the schema the daemon itself loads.

## Agents and the search path

Agents are found by searching a fixed sequence of directories: configuration
beats runtime, which beats data, which beats vendor. The scope is always
explicit and never inferred from the effective user id, and system scope
contains no home-directory path.

Go agents are single files whose built artifacts keep the source's exact
name. Python agents run through a shipped agent development kit. Both are
built by the same command and both are signable.

## Security posture

Production mode refuses an unsigned agent. A node with operator keys
configured refuses a registry that does not match them. Signature
verification uses a configured public key, and a build can stamp a source
hash into the artifact so the running binary can be traced to its source.

## See also

The command reference documents every verb and flag of both binaries, and
the configuration reference documents every key. Both are generated from the
source, so neither can disagree with the binary you are running.

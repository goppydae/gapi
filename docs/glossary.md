# GoPPydae Glossary & Invariants

This glossary defines normative meanings for core terms used across GoPPydae.
If code or docs disagree with this file, the disagreement must be resolved.

## Deterministic
**Definition:** For a fixed input stream and configuration, the system produces the same observable outputs.

**Observables include:**
- lifecycle transitions and their ordering
- emitted events (type + payload)
- exit codes and error classes
- on-disk state (if any), modulo timestamps explicitly declared nondeterministic

**Invariants:**
- no iteration over maps without ordering
- no time-based branching unless time is injected
- randomness must be seeded/injected and recorded

**Non-goals:**
- bit-identical logs (timestamps are allowed to differ)
- identical wall-clock performance

**Evidence hooks:**
- replay test: same inputs ⇒ same outputs hash
- golden trace comparison with stable ordering

## Agent
**Definition:** A unit of software managed by GAPI, defined by a `main` entrypoint and a set of Capabilities.

**Invariants:**
- Must explicitly report its schema hash on startup.
- Must respond to SIGTERM with a graceful shutdown within the configured timeout.
- Must communicate exclusively via the defined ADK protocol (Protobuf).

## Capability
**Definition:** A typed contract that an Agent provides or consumes.

**Invariants:**
- Must be defined in Protobuf.
- Must be statically declared in the Agent's introspection data.
- Cannot change during the runtime of a single process instance.

## Schema Hash
**Definition:** A BLAKE3 hash of the Agent's interface definition (Protobuf schemas).

**Invariants:**
- If the Schema Hash changes, the Agent is effectively a different version.
- Used for compatibility checks during upgrades.

## Mechanism vs Policy
**Definition:** Separation of concerns between capability provision (Mechanism) and decision logic (Policy).

**Invariants:**
- **GAPI** is pure Mechanism (how to start, stop, monitor).
- **Goblin** is Policy (when to start, where to schedule).
- Mechanism layers MUST NOT import Policy layers.

## Security Boundary
**Definition:** The trust perimeter between the GAPI kernel and the Agents it manages.

**Invariants:**
- Agents are untrusted.
- Inputs from Agents are hostile.
- The GAPI kernel memory space is the Trusted Computing Base (TCB).

## Lifecycle Hook
**Definition:** A named phase in the Agent's execution loop (Initialize, Start, Stop).

**Invariants:**
- Hooks are invoked sequentially.
- Failure of a critical hook (Start) results in Agent failure.
- Hooks have strict timeouts.

## Reconciliation
**Definition:** The process of moving the Actual State towards the Desired State.

**Invariants:**
- Level-triggered, not edge-triggered.
- Idempotent: running it twice on the same state produces the same result.
- Convergent: eventually reaches Desired State if inputs are stable.

## Fail Closed
**Definition:** Security and safety default where system failure denies access or halts rather than allowing unsafe operation.

**Invariants:**
- If signature verification fails, the artifact is rejected.
- If configuration is ambiguous, the process exits.
- If a permission check errors, access is denied.

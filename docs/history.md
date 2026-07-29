# GAPI Project History

## Origins and Problem Space

GAPI emerged from an early need within the broader GoPPy ecosystem: a Go-led supervisor capable of managing long-lived microdaemons while delegating domain-specific logic to Python. From the outset, the system emphasized strict boundaries, typed IPC via Protocol Buffers, and deterministic lifecycle management. The earliest conceptual roots trace back to the TactStratX0.d trading daemon work, where Go handled lifecycle and ingestion while Python focused on analytical strategy logic.

The philosophical throughline was clear early on: systemd-like supervision without being systemd, event-driven rather than imperative, and suitable for high-performance or clustered environments rather than desktops.

## Early GoPPy Era (May 2, 2025)

By early May 2025, the architecture had acquired structure under the GoPPy name. Core ideas solidified:

- Go as the supervisory and control-plane language
- Python as a dynamic, interpreted daemon language
- Protobuf as the sole IPC contract
- Explicit lifecycle phases (init, start, stop, reload)
- Event-bus-driven coordination
- Declarative dependency semantics similar to systemd

Operational concerns such as structured logging, PAM-authenticated control interfaces, YAML configuration, and XDG-style paths were already part of the design vocabulary. A systemctl-like CLI experience was envisioned from the start.

## Supervisor Becomes a Platform (May 3, 2025)

On May 3, the project scope expanded decisively. What began as a specialized trading supervisor was explicitly framed as a general-purpose supervision platform with versioned ambitions:

- V1: a functional but scoped supervisor
- V2: a hardened, general-purpose supervisor with stronger security and extensibility

This is also when the daemon SDK concept sharpened: daemons would be declaratively described, versioned, and lifecycle-aware, with SDKs for both Go and Python. Around this time, the name "Goblin" began to circulate as the eventual distributed supervisor built atop the core runtime.

## Birth of GAPI as a Layer (May 4, 2025)

May 4, 2025 marks the clearest inflection point. GAPI was explicitly separated as a foundational layer: a runtime and SDK boundary, no longer merely an internal pattern.

Key structural decisions landed:

- A formal repository layout separating core, SDK, internals, and CLI
- Version stamping and schema hashing
- Naming conventions and daemon identity taxonomy
- Clear layering: GAPI as the kernel, Goblin as the orchestrator

From this point forward, GAPI was treated as a platform primitive rather than an application.

## Cryptography and Canonical Core Doctrine (May 5-7, 2025)

In early May, deterministic security primitives were locked in:

- BLAKE3 for internal hashing and identity fingerprints
- SHA-256 for compatibility
- ED25519 for signatures and identity
- AGE for encrypted configuration and secrets

Equally important was a governance decision: **Go is the canonical implementation layer**. All other SDKs and bindings must interface with Go rather than reimplementing core logic. This ensured long-term consistency across languages and prevented ecosystem drift.

## First Audit Snapshot (May 12, 2025)

By mid-May, GAPI existed as real, auditable code. Tooling and build discipline were strong, Protobuf schemas were present, and lifecycle concepts were defined. Gaps were also clear: incomplete transport wiring, partial lifecycle orchestration, and early-stage introspection.

At this stage, GAPI had a skeleton and organs, but not yet full musculature.

## Design Crystallization (November 11, 2025)

The GAPI design document finalized the project's constitution:

- Event-driven supervision framework
- Local-first with cluster extensibility
- TCP/QUIC transport with future UNIX socket support
- Strong self-description and introspection contracts
- No default reliance on static manifest files
- Explicit positioning within the ecosystem

GAPI was now clearly defined as the foundation, Goblin as the distributed supervisor, and downstream systems as consumers of the runtime.

## One-Sentence Arc

GAPI began as an implied kernel for a Go-Python trading daemon, became a distinct runtime boundary in May 2025, hardened around deterministic identity and canonical Go control, and emerged by late 2025 as a formally specified supervision framework.

## Key Dates

- May 4, 2025: GAPI formally separated as its own layer
- November 11, 2025: GAPI design constitution finalized

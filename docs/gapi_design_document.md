# GAPI Design Document

**Version**: 1.1  
**Date**: Nov 11, 2025  
**Author**: Enqack  

---

## 🧭 Project Overview

**GAPI** is a lightweight, event-driven supervision framework designed for managing distributed daemon (agent) lifecycles in both local and clustered environments. It supports Go and Python natively and is built around principles of clarity, zero-config startup, and scalable coordination.

---

## 🧱 Core Architecture

- **Language Targets**: Go (compiled) and Python (interpreted)
- **Transport**: Initially supports TCP/QUIC and later UNIX sockets
- **Messaging**: Protobuf-encoded control and telemetry messages
- **Lifecycle Model**: Lifecycle-aware agents with structured phases and optional hooks
- **Agent Development Kits (ADKs)**: Provide a zero-boilerplate experience

---

## 🔒 Identity, Security, and Integrity

- **BLAKE3** for schema and identity hashing.
- **ED25519** keys for signing manifests and agent identities.
- **AGE** for encrypting sealed configuration and message payloads.
- Unified cryptographic workflow via `gapi-crypto`, ensuring deterministic and verifiable builds.
- `--describe` includes version, hash, and signer fingerprints.
- Manifests and schema hashes verified at runtime for integrity.

---

## 📦 Logging and IPC

- Structured logging via **Zerolog**
- IPC separated from logs for clarity
- Planned support for stream multiplexing (e.g., over QUIC)

---

## 🔁 Lifecycle Model

### Core Methods
- `Initialize()`
- `Start()`
- `Stop()`
- `Reload()`

### Optional Methods
- `Restart()`

### Optional Hooks
- `BeforeStart()`
- `AfterStop()`
- `OnSignal(sig)`

Lifecycle methods enable flexible agent control while preserving a minimal interface contract.

---

## 🧩 SDK Design

### Functional Layout
- Agents are defined via **flat function files**—no classes or heavy struct requirements.
- Each function maps directly to a lifecycle phase.

### Zero-Config Self-Description
- **Python ADK**: Uses reflection to discover functions at runtime.
- **Go ADK**: Introspects registered functions and exposes `--describe` metadata.

This design eliminates manifest files and supports fully self-describing agents.

---

## 🧾 Describe Schema

Defines standardized metadata exposed by all agents:

```yaml
describe:
  id: "agent-uuid"
  version: "1.0.0"
  type: "service"
  language: "go"
  hash: "b3f2a4..."
  signer: "ed25519:aa44..."
  state: "running"
  capabilities: ["reload", "restart"]
```

Future iterations will formalize schema validation and introspection contracts for consistent behavior across ADKs.

---

## 🔗 Interface Contracts

- Interface contracts defined in Protobuf for lifecycle and IPC.
- `LifecycleControl`, `LifecycleStatus`, and `Envelope` schemas form the core message types.
- ADKs must implement or consume these through bindings (`gopy`, CLI shims, etc.).
- Versioning and schema compatibility will be enforced across SDKs.

---

## 🪸 Ecosystem Relationship and Context

**GAPI** forms the foundational layer of the **GoPPydae ecosystem**, serving as the core runtime and SDK for supervised daemon management. Within this hierarchy:

- **GAPI** — Core runtime and SDK responsible for lifecycle management, structured logging, cryptographic integrity, and process introspection.
- **Goblin** — Distributed supervisor extending GAPI into multi-node systems with gossip-based discovery, Raft coordination, and cluster event routing.
- **GoPPydae** — The broader ecosystem encompassing GAPI, Goblin, and downstream systems (e.g., *TactStratX0.d*).

### Division of Responsibilities
- **Go (GAPI Core):** Lifecycle, logging, secure IPC.
- **Python (Logic Layer):** High-level behaviors and algorithms communicating via Protobuf.
- **Protobuf:** Unified protocol for command and telemetry exchange.

---

## 🌐 Network-Aware Supervision (Goblin)

Goblin extends GAPI into multi-node systems with:

- **Serf** for node discovery
- **Raft** for leader election
- **Event Bus** for cluster-wide messaging with topic filtering
- **Namespacing & Tagging** for daemon grouping and ACL control

---

## 🧠 Developer Experience Philosophy

- **Zero boilerplate** — just write functions
- **Optional features auto-detected**
- **Flat files, no manifests**
- **SDK handles wiring and introspection**
- **CLI tools (`gapictl`) manage full lifecycle and cluster control**

---

## 📘 Appendices

### Appendix A — Naming and Taxonomy
- Agents follow `<name>.<language>.<unit-type>` format (e.g., `heartbeat.py.service`).
- Unit types include `.service`, `.timer`, `.pipe`, `.event`, `.init`.
- `dae` suffix designates GoPPydae descendants.

### Appendix B — Roadmap and Future Work
- Schema validation and describe consistency
- QUIC multiplexing and secure tunnels
- Cross-ADK testing framework
- Goblin: HA clusters and automatic agent failover

### Appendix C — Build Metadata
- Version stamping via `go build -ldflags`.
- Schema hash integration from `.schema_hash`.
- Magefile tasks for build, hash, and describe automation.

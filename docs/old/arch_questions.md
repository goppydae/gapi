# Big-Picture Questions: GoPPydae Architecture Overview

This document collects key architectural questions that arise from the GoPPydae design, particularly the centralization of core logic in Go and the use of language bindings via Daemon Developer Kits (DDKs).

---

## 🧩 Interface Contracts: How Granular Are They?

* Will interface contracts be defined using a format like Protobuf, or something custom?
* Are interface versions explicitly tracked and enforced?
* Will DDKs be expected to implement hooks or just consume Go APIs?

## 🔗 Binding Tooling: How Deep Is the Integration?

* Is `gopy` the long-term strategy for Python, or are other binding tools under consideration?
* How will incompatibilities or breakages in binding tools be handled?
* For less binding-friendly languages (e.g. Rust, Lua), is there a fallback strategy (e.g. CLI/JSON contracts)?

## 🛠 Lifecycle and Describe Contracts: How Rich Are They?

* Do lifecycle hooks support side effects and state persistence?
* How expressive will the `describe` metadata be (e.g., health checks, status, nested dependencies)?
* Will composite daemons or nested lifecycle units be supported?

## 🌐 DDK Interoperability: Cross-DDK Messaging?

* Will all DDKs share the same envelope/message format for IPC and event transport?
* Is a unified message bus (e.g. domain-local QUIC or Unix socket router) part of the plan?
* Are daemons encouraged to communicate across DDKs natively or via message passing only?

## 📦 Distribution and Packaging: Delivery Strategy?

* Will Python/Shell DDKs be available via PyPI, Nix flakes, or some other mechanism?
* Are Go modules versioned in a way that aligns with DDK versions?
* Is there a strategy for ensuring version compatibility across DDKs and the Go core?

## 🧪 Testing Strategy: Integration and Coverage?

* Will there be an integration test suite validating DDK-to-core behavior?
* Are test fixtures planned for simulating lifecycle, IPC, and `describe` calls?
* Will coverage reports include binding code and cross-language test paths?

---

These questions serve as scaffolding for continued refinement of GoPPydae as it matures. They aim to support clear expectations, reduce ambiguity, and strengthen long-term maintainability and extensibility.

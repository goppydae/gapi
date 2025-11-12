# ARC Clarification and Answer Document

This document addresses architectural questions raised in `arch_questions.md`, referencing existing decisions and notes across GoPPydae design documentation, meeting minutes, and the lore.

---

## 1. Interface Contracts via Protobuf

**Question:** Will interface contracts be defined using Protobuf, or something custom?

**Answer:** Yes, Protobuf is the canonical schema format for all inter-agent and controller communication. Contracts for `describe`, `ping`, and `pong` are Protobuf-defined, and the entire messaging layer is standardized using Protobuf. Custom schema languages are explicitly avoided to maintain clarity and toolchain support.

_Ref: `GOPPY_Design_Document.md`, `lore.md`_

---

## 2. Interface Versioning and Enforcement

**Question:** Will versions of interfaces be explicitly tracked and enforced?

**Answer:** Yes. Versioning is enforced by embedding schema hashes and version metadata into each compiled binary. This ensures schema mismatches are caught early and facilitates runtime introspection of compatibility. Linker flags are used during the build process to embed commit hashes and schema hashes into both `gapid` and `gapictl`.

_Ref: `GOPPY_Design_Document.md`, `Magefile.go`_

---

## 3. DDK Roles: Hooks vs API Consumption

**Question:** Do DDKs implement hooks, or do they consume APIs exposed by the system?

**Answer:** DDKs are consumers of Go-implemented APIs. All complex or critical logic lives in the Go core and is accessed via bindings (`gopy` for Python) or command-line helpers (for Shell). DDKs do not reimplement lifecycle logic or cryptographic routines.

_Ref: `arch.md`_

---

## 4. Binding Tools and Language Support

**Question:** Will `gopy` be the primary method of supporting Python?

**Answer:** Yes. For Python, `gopy` is used to generate native bindings for Go packages. The Python DDK is designed to wrap those bindings with idiomatic decorators and interfaces. For Shell, a suite of small Go binaries provides equivalent functionality.

_Ref: `arch.md`_

---

## 5. Fallback for Languages Without Native Bindings

**Question:** How will languages without clean Go bindings (e.g. Rust, Lua) be supported?

**Answer:** These languages will use CLI-based helpers and a consistent IPC format (based on Protobuf-over-QUIC or JSON fallbacks). The Shell DDK is the main reference for this model: a suite of Go helpers is used to handle logic the language cannot easily support natively.

_Ref: `arch.md`_

---

## 6. Lifecycle Hooks and State

**Question:** Will lifecycle support include persistent state, hooks, and side-effect behavior?

**Answer:** Yes. Lifecycle hooks include `init`, `start`, `stop`, and planned additions like `reload`, `beforeStart`, and `afterStop`. These hooks are used to control daemon behavior and can optionally trigger side-effectful operations or updates to stored state.

_Ref: `GOPPY_Design_Document.md`_

---

## 7. Describe Contract Richness

**Question:** How expressive is the `describe` metadata model?

**Answer:** The `describe` schema includes fields for capabilities, version information, lifecycle state, and supported commands. Richer metadata like dependency declarations, health checks, or introspection flags are planned but not yet fully implemented. The structure supports extension via Protobuf without breaking compatibility.

_Ref: `GOPPY_Design_Document.md`_

---

**End of Clarification Pass #1**

This document reflects the current architectural understanding and decisions made as of May 2025. It will evolve as the system matures and remaining questions are resolved.
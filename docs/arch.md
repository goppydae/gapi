# GoPPydae Architecture Stub: Centralized Go Core with DDK Bindings

**Date:** 2025-05-07  
**Topic:** Centering Go as the Canonical Runtime for Logic and Bridging Other DDKs via Bindings

---

## Overview

This stub outlines a pivotal architectural milestone in the GoPPydae ecosystem: the decision to consolidate core logic—including cryptographic primitives, describe metadata, and lifecycle support—within a centralized Go package. Other DDKs (Python, Shell, and future languages) will interface with this logic through well-defined bindings or CLI delegates.

---

## Motivation

- Reduce code duplication across SDKs/DDKs.
- Maintain a single, testable, auditable implementation of foundational logic.
- Ensure uniform behavior across daemons regardless of implementation language.
- Allow Python and Shell agents to "feel native" while sharing core behavior.

---

## Model

### Go as the Canonical Layer

- Hosts: cryptographic logic, schema hashing, signing, event model helpers.
- Used by: `gapid`, `gapictl`, `gapi-crypto`, and all official DDKs.
- Provides versioned APIs for DDK interaction.

### Binding Requirement for DDKs

- DDKs must either:
    - Use language-native bindings (e.g. `gopy` for Python),
    - Or use CLI/IPC shims (e.g. helper binaries for Shell).
- No critical logic may be reimplemented outside of Go.

---

## DDK Integration Examples

### Python

- Uses `gopy`-generated bindings or equivalent.
- Python DDK decorates daemons; underlying logic calls into Go.

### Shell

- Uses a suite of Go helper binaries (e.g. `goppy-hash`, `goppy-describe`).
- Configuration and identity handled via IPC or env file exchange.

---

## Benefits

- **Cryptographic integrity**: one source for schema hashes, key signatures, and version metadata.
- **Consistency**: every daemon speaks the same dialect of `describe`, lifecycle, and config.
- **Extensibility**: new languages can join the ecosystem if they can bind to Go.
- **Maintainability**: fixes and enhancements happen in one place, not many.

---

## Future Considerations

- Create `gapi/crypto`, `gapi/lifecycle`, and `gapi/describe` Go modules.
- Establish interface contracts for language bindings.
- Version and document public APIs for DDK consumption.
- Evaluate `gopy` tooling and Go module compatibility for releases.

---

*Stub entry created from a lounge milestone.*

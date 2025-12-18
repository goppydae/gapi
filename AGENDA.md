# GAPI Agenda

<!-- This file tracks the immediate development plan for the GAPI project. -->

## Active Hypotheses

- [x] **GAPI-HYP-01**: If `NewQUICClient` is updated to accept a `tls.Config`, then the client can be configured to verify the server certificate, preventing man-in-the-middle attacks.

## Blockers

- None.

## Deferred Risks

- **SEC-01**: Current transport implementation uses `InsecureSkipVerify: true` by default.

## Phase 0: Core Library Refactor (Priority)

**Goal**: Export stable core APIs for Goblin integration.

### 1. Package Structure Reorganization

- **Context**: Enable Goblin to import GAPI components without embedding full daemon.
- [x] **Action**: Move `internal/agentmgr` → `core/agentmgr` (public API)
- [x] **Action**: Move `internal/lifecycle` → `core/lifecycle` (public API)
- [x] **Action**: Define stable interfaces with semantic versioning
- [x] **Action**: Update `internal/supervisor` to use exported `core/*` packages
- [x] **Action**: Verify `gapid` daemon continues to work (backward compatibility)
- **Benefit**: Goblin can use GAPI components in-process (single executable architecture)
- **Status**: ✅ Complete (Dec 16, 2025)

______________________________________________________________________

## Phase 1: Security and Stability

## 1. Security: Enforce TLS in Transport

- **Location**: `internal/transport/quic.go`
- **Context**: `NewQUICClient` has hardcoded `InsecureSkipVerify: true`.
- [x] **Action**: Update constructor to accept `TLSConfig` (CA, Insecure flag) similar to Goblin.
- **Benefit**: Allows ADKs to verify the Supervisor's identity in production.

## 2. Feature: Loki Log Output

- **Context**: Supervisor should ship logs to Loki.
- **Priority**: High
- [ ] **Action**: Implement Loki log shipping.

## 3. Documentation Audit

- [ ] **Action**: verify correctness of docs.

## Phase 2: CLI Harmonization and Port Refactoring

### 1. Port Configuration Refactoring

- [x] **Action**: Update default ports to non-standard values.
- [x] **Transport**: `14242`
- [x] **Metrics**: `19090`
- **Status**: ✅ Complete (Dec 17, 2025)

### 2. CLI Command Harmonization

- [x] **Action**: Refactor `gapictl` into namespaces (`agent`, `lifecycle`, `crypto`).
- [x] **Action**: Standardize naming (hyphens to subcommands).
- [x] **Action**: Implement `agent status` default tree view.
- [x] **Action**: Implement `agent reload` deep scan semantics.
- **Status**: ✅ Complete (Dec 17, 2025)

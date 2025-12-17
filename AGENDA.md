# GAPI Agenda

<!-- This file tracks the immediate development plan for the GAPI project. -->

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

---

## Phase 1: Security and Stability

## 1. Security: Enforce TLS in Transport
- **Location**: `internal/transport/quic.go`
- **Context**: `NewQUICClient` has hardcoded `InsecureSkipVerify: true`.
- [ ] **Action**: Update constructor to accept `TLSConfig` (CA, Insecure flag) similar to Goblin.
- **Benefit**: Allows ADKs to verify the Supervisor's identity in production.

## 2. Feature: Loki Log Output
- **Context**: Supervisor should ship logs to Loki.
- **Priority**: High
- [ ] **Action**: Implement Loki log shipping.

## 3. Documentation Audit
- [ ] **Action**: verify correctness of docs.

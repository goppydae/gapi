# GAPI Development Checklist – May 13th 2025

## 🔁 Lifecycle Management
- [ ] Implement `INIT` and `RELOAD` into `LifecycleControl` enum
- [ ] Create lifecycle controller and state machine for agents
- [ ] Add support for optional hooks: `preStart`, `postStop`, etc.
- [ ] Wire lifecycle execution into agent manager
- [ ] Add lifecycle logging events (`lifecycle.init`, `lifecycle.start`, ...)

## 📬 Describe Metadata & Introspection
- [ ] Finalize `describe` schema format (Go + Python agents)
- [ ] Implement `--describe` for Go agents
- [ ] Bind `describe()` logic into Python ADK via `gopy`
- [ ] Include `id`, `version`, `hash`, `capabilities`, and lifecycle fields in output

## 🔐 Cryptography Integration
- [ ] Integrate BLAKE3 hashing from centralized `gapi/crypto`
- [X] Embed schema hash and Git commit hash in `--version` output
- [ ] Add support for ED25519 signing of describe metadata and schema files
- [ ] Begin exposing `gapi-crypto` CLI interface as helper tooling
- [ ] Prototype AGE encryption for optional sealed config fields

## 🔧 Agent Runtime & ADK Expansion
- [ ] Finalize Go agent manager registration and discovery
- [ ] Wire discovered agents into database registry
- [ ] Expand Python ADK to support lifecycle interface calls
- [ ] Begin Shell DDK stub with helper binaries (e.g. `goppy-describe`)
- [ ] Prototype sample agent (e.g., Pi calculator or heartbeat logger)

## 🌐 IPC + Event Bus
- [X] Fix naming inconsistency in `.pb.go` file (`envolope.pb.go` → `envelope.pb.go`)
- [ ] Finalize QUIC transport and handshake setup
- [ ] Add timestamp field to `ping`/`pong` for latency tracking
- [ ] Enable proper topic routing via event bus (e.g., `status.*`, `lifecycle.*`)

## 🛠 Tooling & Testing
- [X] Add schema hash verification task to Magefile
- [X] Include `.proto` files in source tree
- [ ] Establish snapshot CI checks for `.proto` vs `.pb.go` drift
- [ ] Begin basic integration test for `gapid + gapictl + agent` lifecycle
- [ ] Stub test coverage for bindings (Python/Gopy)

## 📦 Packaging and Delivery
- [X] Ensure versioning metadata embedded via linker flags
- [X] Consider early Nix flake structure for GAPI, Goblin, and TactStratX0.d
- [ ] Explore bundling `gapi-crypto` with ADKs
- [ ] Prepare `sdk/go`, `sdk/python`, and future `sdk/shell` as public modules

## 🧠 Documentation & Design
- [ ] Add GAPI lifecycle overview to `docs/`
- [ ] Expand the Event-Driven Architecture doc to include agent participation lifecycle
- [ ] Document architecture for crypto + agent registration + describe
- [ ] Begin public README and modular doc site (MkDocs or Hugo)


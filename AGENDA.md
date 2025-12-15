# Project Agenda

---

## 🎯 Mission Statement

**Build a zero-boilerplate, production-ready daemon supervisor** that scales from single-node embedded systems to distributed clusters.

### Core Objectives
1. **GAPI (Kernel)**: Local agent lifecycle management with strict contracts
2. **Zero Boilerplate**: Self-describing agents, no manifest files
3. **Production Ready**: Cryptographic security, reproducible builds, comprehensive testing

### Success Criteria
- ✅ Agents self-describe via introspection (`--describe` for Go, module attributes for Python)
- ✅ BLAKE3 hashing for build integrity
- ✅ ED25519 signing for agent verification
- ✅ Cross-ADK parity (Python and Go agents behave identically)
- ✅ systemd-style search paths for agent discovery
- ✅ Watch mode for rapid Go agent iteration
- ✅ Agent templates for scaffolding
- ✅ Source-to-binary verification chain
- ✅ Comprehensive test coverage (unit, integration, E2E, cross-ADK)
- ✅ CI/CD integration (GitHub Actions)


---

## Immediate Priorities
### Immediate: ED25519 Signing Protocol
- [x] Implement crypto/ed25519 wrappers in `core`.
- [x] Add signature verification to `internal/agentreg`.
- [x] Add `keygen`, `sign`, `verify` to `gapictl`.
- [x] STRICT enforcement in `gapid` startup loop.

### Immediate: AGE Encryption
- [x] Implement wrappers in `core/crypto`.
- [x] Add `age-keygen`, `encrypt`, `decrypt` commands to `gapictl`.
- [x] Verify workflow.

### Short Term: Agent Capabilities
- [x] Add `@capability` decorator to Python ADK.
- [x] Expose capabilities in `gapid` discovery.
- [x] Update `gapictl agent-status` to show capabilities.
- [x] **Daemon Supervisor Improvements**: Refine the `gapid` supervisor to handle more complex lifecycle states and dependency resolution (Implemented).

## Core Polish (In Progress)
### Build Security & Determinism
- [x] **BLAKE3 Build Hashing**: Integrated BLAKE3 hash generation into `Magefile.go` build pipeline.
  - Binaries now generate `.b3` checksum files automatically
  - Foundation for reproducible builds and integrity verification
- [x] **Source-to-Binary Chain**: Implement full verification chain (source hash → sign → verify binary)
  - `HashDirectory()` function for source hashing
  - Build process embeds source hash via `-ldflags`
  - `gapictl agent verify` command for verification
  - Verifies binary hash, signature, and source hash
  - No manifest files (embedded in binary)
- [x] **Hermetic Builds**: Explore Nix integration for reproducible Go builds

### Cross-ADK Testing
- [x] **Test Framework**: Created comprehensive cross-ADK test suite (`test/adk/cross_adk_test.go`)
  - Tests for describe metadata parity
  - Lifecycle transition verification
  - Capability detection consistency
  - Schema hashing validation
- [x] **Test Fixtures**: Added Python test agents (simple_service, lifecycle_agent, capabilities_agent)
- [x] **Go Test Fixtures**: Create matching Go agent fixtures
  - `simple_service.go` - Minimal service agent
  - `lifecycle_agent.go` - Full lifecycle (initialize, start, stop, reload)
  - `capabilities_agent.go` - Capability detection with custom actions
  - `hash_agent.go` - Schema hashing support with BLAKE3
  - All fixtures compile successfully
  - **Cross-ADK Tests**: 100% passing (4/4 tests)
    - ✅ DescribeMetadata - Metadata structure parity
    - ✅ LifecycleTransitions - Lifecycle behavior parity
    - ✅ CapabilityDetection - Capability introspection parity
    - ✅ SchemaHashing - BLAKE3 hash computation parity
- [x] **CI Integration**: Add cross-ADK tests to continuous integration pipeline
  - GitHub Actions workflow (`.github/workflows/ci.yml`)
  - Build job: Compile `gapictl` and `gapid`
  - Test job: Run cross-ADK parity tests
  - Nix caching for faster builds
  - Test result artifacts uploaded

### Go Agent Execution Strategy
- [x] **Design Document**: Created execution strategy proposal (`go_agent_execution_strategy.md`)
  - Recommends compile-first with optional dev mode
  - Hybrid approach balancing performance and developer experience
  - **Refined**: Three-tier architecture (Python/Compiled Go/Plugins)
- [x] **Build Helper Prototype**: Created `gapictl agent build` command (`cmd/gapictl/agent.go`)
  - Compiles Go agents with BLAKE3 hashing
  - Optional ED25519 signing support
  - Directory scanning for batch builds
- [x] **Watch Mode**: Implement `--watch` flag with fsnotify
  - Auto-rebuilds on `.go` file changes
  - 300ms debouncing to avoid excessive rebuilds
  - Recursive directory watching
  - Skips hidden files and build artifacts
- [x] **Agent Templates**: Add `gapictl agent new --lang=go <name>` scaffolding
  - Embedded templates using Go's `embed` package
  - Supports Go (service, timer, socket) and Python (service)
  - Auto-generates proper directory structure
  - Includes `--describe`, lifecycle methods, signal handling
  - Follows `name.lang.type` naming convention

### Agent Directory Organization
- [x] **Migration Plan**: Created comprehensive reorganization strategy (`agent_directory_migration.md`)
  - Language-specific subdirectories (python/, go/, plugins/)
  - Type-based organization (services/, timers/, sockets/)
  - Build artifact separation (build/ directory)
  - Phased migration with backward compatibility
  - **Corrected**: Zero-boilerplate (no metadata.yaml files)
- [x] **Search Path Specification**: Created systemd-style agent discovery (`agent_search_paths.md`)
  - Development paths: `./agents/`, `$GAPI_DEV_AGENTS`
  - User paths: `~/.local/share/gapi/agents/`, `~/.gapi/agents/`
  - System paths: `/usr/lib/gapi/agents/`, `/usr/local/lib/gapi/agents/`
  - Environment overrides: `GAPI_AGENT_PATH`, `GAPI_SKIP_SYSTEM_AGENTS`
- [x] **Search Path Implementation**: Created `core/config/agent_paths.go`
  - `AgentSearchPaths()` function with priority ordering
  - `ClassifyPath()` for path type detection (DEV/USER/SYSTEM)
  - XDG Base Directory compliance
- [x] **Phase 1**: Create new directory structure
  - Created `python/`, `go/`, `plugins/`, `build/` subdirectories
  - Added `.gitignore` for build artifacts
  - Created comprehensive README files for each directory
  - Python ADK guide with examples and best practices
  - Go ADK guide with build process and self-describing pattern
  - Plugin guide with experimental warnings
- [x] **Phase 2**: Migrate Python agents with symlinks
  - Migrated 3 services: heartbeat, simple_service, hog
  - Migrated 5 timers: cron_5min, cron_hourly, my_strategy, simple_timer, tick
  - Migrated 1 socket: simple_socket
  - Created symlinks with `.new` suffix for testing
  - Original files preserved for backward compatibility
- [x] **Phase 3**: Update discovery logic to use search paths
  - Implemented `DiscoverFromPaths()` in `internal/agentmgr/discovery.go`
  - Updated `core/supervisor/supervisor.go` to use new method
  - First-match-wins priority ordering (Development → User → System)
  - Path type logging (DEV/USER/SYSTEM)
- [x] **Phase 4**: Remove legacy support
  - ~~Activated symlinks (removed `.new` suffix)~~ **Removed entirely**
  - Removed all backward compatibility symlinks
  - Removed `__pycache__` from root
  - Restored `name.lang.type` naming convention
  - All agents now use new structure exclusively:
    - `python/services/heartbeat.py.service`
    - `python/timers/cron_5min.py.timer`
    - `python/sockets/simple_socket.py.socket`
  - Legacy paths no longer supported
  - Migration complete ✅

## Long-Term Vision & Architecture
- [x] **Architecture Enforcement (GAPI as Library)**:
    - [x] **API Boundary Check**: Ensure no network/cluster logic leaks into `core/`
    - [x] **Library-First Design**: Verify `cmd/gapid` is just a thin wrapper around `gapi` package calls
    - [x] **Standalone Usability**: Verify GAPI usage in simple CLI tools without distributed overhead
- [ ] **ADK Evolution**:
    - [x] **Schema Validation**: Formalize describe schema contracts
    - [x] **Native Channels**: Explore native `gopy` channel support (PoC successful)

---

### Documentation & Developer Experience
- [x] **Documentation System**: Implemented dual-mode generation (`Magefile.go`)
  - **HTML Site**: MkDocs with Material theme (`docs:html`)
  - **Man Pages**: Pandoc generation for CLI manuals (`docs:man`)


---

## 🎉 Session Accomplishments (2025-12-14)

### GAPI (8 Features)
1. ✅ **Cross-ADK Tests** - 100% parity (4/4 passing)
2. ✅ **Watch Mode** - Auto-rebuild on changes
3. ✅ **Agent Templates** - `gapictl agent new` scaffolding
4. ✅ **CI Integration** - GitHub Actions workflow
5. ✅ **Source-to-Binary Chain** - Complete verification
6. ✅ **Documentation Polish** - README, getting-started guide
7. ✅ **Documentation System** - HTML & Man page generation
8. ✅ **Architecture Enforcement** - Library-first verification
9. ✅ **Hermetic Builds** - Nix & Mage enforcement



**Total**: 9 major features completed in one session! 🚀

## 🎉 Session Accomplishments (2025-12-15)

### GAPI (3 Features)
1. ✅ **Client Refactor** - Extracted RPC logic to `core/client`
2. ✅ **Client Lifecycle** - Added `Start()/Stop()/Restart()` methods
3. ✅ **CI/CD Fixes** - Fixed artifact uploads & dispatch triggers

### Breakdown
- **Refactoring**: Aligned with "GAPI as Library" goal by moving client logic out of `cmd/gapictl`.
- **Integration**: Unblocked Goblin by implementing missing lifecycle methods.
- **CI/CD**: Fully automated pipeline with cross-repo triggering.

---

## Proposed Enhancements

### TUI Completeness
- [ ] **Log Streaming**: Implement WebSocket or TTY-based log streaming in `gapi`/`gapictl` (replacing TODOs).
- [ ] **Real Metrics**: Hook up `cgroups` stats to `FetchStatus` to populate CPU/Memory/Uptime fields.

### Plugin Architecture
- [ ] **Go Plugin System**: formalized support for loading agents as `.so` plugins for hot-reloading without binary recompilation.

### Observability
- [ ] **Metrics**: Expose Prometheus metrics for agent resource usage.
- [ ] **Logs**: Integrated log shipping or structured logging adapter.

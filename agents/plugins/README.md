# GAPI Plugins (Experimental)

⚠️ **EXPERIMENTAL FEATURE** - Requires `GAPI_ENABLE_PLUGINS=1`

Shared-object plugins provide **ultra-low-latency** in-process agent execution. Use plugins only for performance-critical coordination tasks where microsecond latency matters.

## ⚠️ Limitations

- **Linux-only**: Go plugin system doesn't support Windows/macOS
- **ABI fragility**: Plugin must be compiled with exact same Go version as gapid
- **Crash isolation**: Plugin crash = supervisor crash (no process boundary)
- **Experimental**: API may change without notice

## When to Use Plugins

✅ **Use plugins for**:
- Raft consensus voting (microsecond latency)
- In-process event routing
- High-frequency coordination tasks

❌ **Use compiled Go agents for**:
- System boot, foundational services
- Anything requiring crash isolation
- Cross-platform support

❌ **Use Python agents for**:
- Everything else

## Quick Start

### Basic Plugin

```go
// agents/plugins/event_router/main.go
package main

import "fmt"

// Agent is the exported symbol that gapid will load
type EventRouter struct{}

func (e *EventRouter) Initialize() error {
    fmt.Println("EventRouter initialized")
    return nil
}

func (e *EventRouter) Start() error {
    fmt.Println("EventRouter started")
    return nil
}

func (e *EventRouter) Stop() error {
    fmt.Println("EventRouter stopped")
    return nil
}

func (e *EventRouter) Describe() map[string]string {
    return map[string]string{
        "id":      "event_router",
        "type":    "plugin",
        "version": "1.0.0",
    }
}

// Exported symbol (required)
var Agent EventRouter
```

### Build Plugin

```bash
# Build as shared library
go build -buildmode=plugin -o agents/build/plugins/event_router.so agents/plugins/event_router/

# Generate hash
gapictl agent build agents/plugins/event_router/
```

### Enable Plugins

```bash
# Enable plugin support
export GAPI_ENABLE_PLUGINS=1

# Start gapid
gapid
```

## Plugin Interface

All plugins must implement the `Agent` interface:

```go
type Agent interface {
    Initialize() error
    Start() error
    Stop() error
    Describe() map[string]string
}
```

## Build Artifacts

```
agents/build/plugins/
├── event_router.so      # Shared library
└── event_router.so.b3   # BLAKE3 hash
```

## Version Compatibility

**Critical**: Plugin must be compiled with the **exact same Go version** as `gapid`.

```bash
# Check gapid Go version
go version bin/gapid

# Build plugin with same version
go build -buildmode=plugin ...
```

## Risks and Mitigation

| Risk | Mitigation |
|------|------------|
| **Crash Isolation** | Use plugins only for non-critical tasks |
| **ABI Fragility** | Enforce Go version matching in CI |
| **Platform Lock-in** | Document Linux-only requirement |
| **Debugging** | Use extensive logging, avoid plugins in production |

## Testing

```bash
# Test plugin loading (requires gapid with plugin support)
GAPI_ENABLE_PLUGINS=1 gapid

# Verify plugin is loaded
gapictl agent status | grep event_router
```

## Best Practices

1. **Minimize plugin usage**: Only for ultra-low-latency use cases
2. **Version lock**: Pin Go version in CI/CD
3. **Extensive testing**: Plugins are harder to debug
4. **Fallback**: Have a compiled Go agent alternative
5. **Documentation**: Clearly mark as experimental

## Alternative: Compiled Go Agents

For most use cases, use compiled Go agents instead:

```bash
# Compiled agent (recommended)
gapictl agent build agents/go/coordination/cluster_join/

# Plugin (experimental, high-risk)
go build -buildmode=plugin agents/plugins/event_router/
```

## See Also

- [Go Plugin Documentation](https://pkg.go.dev/plugin)
- [Go ADK Guide](../go/README.md)
- [GAPI Design Document](../../docs/gapi-design-document.md)

---

**Status**: Experimental  
**Supported Platforms**: Linux only  
**Recommended**: Use compiled Go agents instead

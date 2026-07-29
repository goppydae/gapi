# Go ADK for GAPI

Go agents are used for **foundational** and **high-performance** use cases. Use Go for system boot agents, cluster coordination, and high-throughput services.

## When to Use Go Agents

**Use Go for**:

- System boot agents (PID 1 init)
- Cluster coordination (Serf/Raft join)
- High-throughput services (log aggregation, metrics collection)
- Low-latency requirements (\<10ms startup)
- Self-contained binaries (no runtime dependencies)

**Use Python for**:

- Business logic and application services
- Rapid iteration and development
- Integration with Python ecosystem (NumPy, pandas, etc.)

## Quick Start

### Basic Service Agent

```go
// agents/go/foundational/my_agent/main.go
package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "os"
    "os/signal"
    "syscall"
)

var describe = flag.Bool("describe", false, "Print agent metadata")

func main() {
    flag.Parse()

    // Self-describing via --describe flag
    if *describe {
        metadata := map[string]interface{}{
            "describe": map[string]interface{}{
                "id":           "my_agent",
                "type":         "service",
                "version":      "1.0.0",
                "language":     "go",
                "description":  "My Go service agent",
                "capabilities": []string{"initialize", "start", "stop"},
                "requires":     []string{},
                "wants":        []string{},
            },
        }
        json.NewEncoder(os.Stdout).Encode(metadata)
        return
    }

    // Normal agent logic
    fmt.Println("[my_agent] Starting...")

    // Handle graceful shutdown
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

    <-sigChan
    fmt.Println("[my_agent] Shutting down...")
}
```

### Build and Run

```bash
# Build the agent
gapictl agent build agents/go/foundational/my_agent/

# Verify metadata
./agents/build/go/my_agent --describe

# Run manually
./agents/build/go/my_agent

# Or let gapid discover and run it
gapid
```

## Agent Metadata (--describe)

All Go agents **must** implement the `--describe` flag to return JSON metadata:

```go
{
  "describe": {
    "id": "my_agent",           // Required: unique identifier
    "type": "service",           // Required: service, timer, socket, init
    "version": "1.0.0",          // Required: semantic version
    "language": "go",            // Required: "go"
    "description": "...",        // Optional: human-readable description
    "capabilities": [...],       // Optional: list of capabilities
    "requires": [...],           // Optional: hard dependencies
    "wants": [...],              // Optional: soft dependencies
    "wanted_by": [...],          // Optional: reverse dependencies
    "required_by": [...]         // Optional: reverse dependencies
  }
}
```

## Agent Types

### Service Agent

Long-running service with graceful shutdown:

```go
func main() {
    if *describe { /* ... */ }

    // Service logic
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan
}
```

### Init Agent (PID 1)

System bootstrap agent:

```go
{
  "describe": {
    "id": "init",
    "type": "init",  // Special type for PID 1
    "version": "1.0.0"
  }
}
```

### Timer Agent

Periodic task (run and exit):

```go
func main() {
    if *describe { /* ... */ }

    // Execute task
    fmt.Println("Task executed")
    // Exit (supervisor will restart based on schedule)
}
```

## Build Process

### Manual Build

```bash
go build -o agents/build/go/my_agent agents/go/foundational/my_agent/
```

### Using gapictl

```bash
# Build single agent
gapictl agent build agents/go/foundational/my_agent/

# Build all Go agents
gapictl agent build agents/go/

# Build with signing
gapictl agent build --sign --key=~/.gapi/signing.key agents/go/foundational/my_agent/

# Watch mode (auto-rebuild on changes)
gapictl agent build --watch agents/go/foundational/my_agent/
```

## Build Artifacts

After building, you'll have:

```
agents/build/go/
|-- my_agent        # Binary
|-- my_agent.b3     # BLAKE3 hash
`-- my_agent.sig    # ED25519 signature (if --sign used)
```

## Dependencies

Specify dependencies in the `--describe` output:

```go
"requires": ["database", "cache"],  // Must start before this agent
"wants": ["monitoring"]              // Start if available
```

## Best Practices

1. **Always implement --describe**: Required for discovery
1. **Graceful shutdown**: Handle SIGINT/SIGTERM
1. **Minimal dependencies**: Keep binaries small and self-contained
1. **Error handling**: Return non-zero exit codes on failure
1. **Logging**: Use structured logging (JSON) for easy parsing
1. **Idempotent**: Safe to restart at any time

## Example: Cluster Join Agent

```go
// agents/go/coordination/cluster_join/main.go
package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "os"

    "github.com/hashicorp/serf/serf"
)

var describe = flag.Bool("describe", false, "Print agent metadata")

func main() {
    flag.Parse()

    if *describe {
        metadata := map[string]interface{}{
            "describe": map[string]interface{}{
                "id":      "cluster_join",
                "type":    "service",
                "version": "1.0.0",
                "language": "go",
                "description": "Serf cluster join coordinator",
            },
        }
        json.NewEncoder(os.Stdout).Encode(metadata)
        return
    }

    // Serf cluster join logic
    config := serf.DefaultConfig()
    config.NodeName = os.Getenv("NODE_NAME")

    cluster, err := serf.Create(config)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to create cluster: %v\n", err)
        os.Exit(1)
    }
    defer cluster.Shutdown()

    // Join existing cluster
    if seeds := os.Getenv("CLUSTER_SEEDS"); seeds != "" {
        cluster.Join([]string{seeds}, true)
    }

    // Wait for shutdown signal
    select {}
}
```

## Testing

```bash
# Test describe output
./agents/build/go/my_agent --describe | jq .

# Test execution
./agents/build/go/my_agent

# Verify hash
cat agents/build/go/my_agent.b3
```

## See Also

- [GAPI Design Document](../../docs/gapi-design-document.md)
- [Agent Directory Structure](../README.md)
- [Python ADK Guide](../python/README.md)
- [gapictl agent build](../../cmd/gapictl/agent.go)

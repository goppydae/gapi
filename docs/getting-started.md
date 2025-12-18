# Getting Started with GAPI

This guide will help you create, build, and run your first GAPI agent.

## Installation

### Using Nix (Recommended)

```bash
cd gapi
nix develop -c mage build
```

### Using Go Directly

```bash
go build -o bin/gapid ./cmd/gapid
go build -o bin/gapictl ./cmd/gapictl
```

## Your First Agent

### Create a Go Service Agent

```bash
gapictl agent new my_first_service
```

This creates `agents/go/foundational/my_first_service/main.go` with a complete template.

### Build the Agent

```bash
gapictl agent build agents/go/foundational/my_first_service
```

The build process:

- Computes source hash (BLAKE3)
- Embeds hash in binary via `-ldflags`
- Generates binary hash (`.b3` file)
- Creates `agents/build/go/my_first_service`

### Verify the Agent

```bash
gapictl agent verify agents/build/go/my_first_service
```

Output:

```
✅ Binary hash: VERIFIED
⚠️  No .sig file found (not signed)
```

### Run the Supervisor

```bash
./bin/gapid
```

## Next Steps

- **[Agent Development](../agents/README.md)** - Learn to write custom agents
- **[Python ADK](../agents/python/README.md)** - Write Python agents
- **[Go ADK](../agents/go/README.md)** - Write Go agents
- **[Security](features.md#security)** - Sign and verify binaries
- **[Watch Mode](../agents/go/README.md#watch-mode)** - Auto-rebuild on changes

## Common Commands

```bash
# Create agents
gapictl agent new my_service                    # Go service
gapictl agent new --type=timer my_timer         # Go timer
gapictl agent new --lang=python my_py_service   # Python service

# Build
gapictl agent build agents/go/my_service        # Single build
gapictl agent build --watch agents/go/my_service # Watch mode

# Verify
gapictl agent verify agents/build/go/my_service

# Sign and verify
gapictl agent build --sign --key=key.pem agents/go/my_service
gapictl agent verify agents/build/go/my_service --pubkey=key.pub
```

## Troubleshooting

### Build Fails

Make sure you're in the correct directory:

```bash
cd agents/go/foundational/my_service
go build .  # Test build directly
```

### Verification Fails

Check that the binary hasn't been modified:

```bash
# Rebuild
gapictl agent build agents/go/my_service

# Verify again
gapictl agent verify agents/build/go/my_service
```

## What's Next?

Now that you have a working agent, explore:

- **Timer agents** for scheduled tasks
- **Socket agents** for network services
- **Resource limits** with cgroups
- **Cross-ADK testing** for parity verification

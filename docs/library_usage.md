# GAPI as a Library

GAPI is designed with a "kernel implementation" philosophy. The core logic resides in reusable packages, allowing you to embed the GAPI supervisor into your own Go applications without running the `gapid` daemon directly.

## Architecture

The `github.com/goppydae/gapi` module provides the following core packages:

- `core/supervisor`: The main coordination kernel.
- `core/config`: Configuration structures.
- `core/eventbus`: The internal message bus.

The `cmd/gapid` binary is simply a thin wrapper around these packages.

## Embedding GAPI

To use GAPI in your application:

1. Import the core packages:

   ```go
   import (
       "github.com/goppydae/gapi/core/config"
       "github.com/goppydae/gapi/core/supervisor"
   )
   ```

1. Initialize a configuration and supervisor:

   ```go
   // Load config from file or defaults
   cfg, err := config.Load() 

   // Or create programmatically
   cfg = &config.Config{
       Transport: config.TransportConfig{Type: "quic"},
   }

   sup, err := supervisor.New(cfg)
   if err != nil {
       panic(err)
   }
   ```

1. Run the supervisor with a context:

   ```go
   ctx := context.Background()
   if err := sup.Run(ctx); err != nil {
       log.Fatal(err)
   }
   ```

## Example

See [examples/standalone](../examples/standalone/main.go) for a complete, runnable example.

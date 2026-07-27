package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/core/supervisor"
)

func main() {
	log.Println("📚 standalone GAPI library example starting...")

	// 1. Programmatic Config
	// In a real embed, you might read this from your own app's config
	cfg := &config.Config{
		Transport: config.TransportConfig{
			Type: "quic", // Use default transport
		},
	}

	// Create agents dir if it doesn't exist to avoid errors
	_ = os.MkdirAll("agents", 0750)

	// 2. Initialize Supervisor as library
	log.Println("🔧 Initializing Supervisor...")
	sup, err := supervisor.New(cfg)
	if err != nil {
		log.Fatalf("❌ Failed to init supervisor: %v", err)
	}

	// 3. Run with a timeout context to demonstrate lifecycle control
	// This shows we can start/stop the supervisor programmatically
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log.Println("🚀 Running Supervisor (5s duration)...")
	if err := sup.Run(ctx); err != nil {
		log.Printf("⚠️ Supervisor exited with error: %v", err)
	}

	log.Println("✅ Standalone example finished.")
}

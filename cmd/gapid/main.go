package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/core/daemonmgr"
	"github.com/goppydae/gapi/core/eventbus"
	"github.com/goppydae/gapi/internal/transport"
)

func main() {
	log.Println("[gapid] initializing...")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	t, err := transport.NewServerFromConfig(cfg.Transport)
	if err != nil {
		log.Fatalf("failed to initialize transport: %v", err)
	}

	bus := eventbus.NewEventBus(t)
	manager := daemonmgr.NewDaemonManager(bus)

	bus.SubscribePrefix("user", "system/ping", func(e eventbus.Event) {
		response := eventbus.NewEvent("user", "system/pong", "gapid", map[string]string{"status": "pong"}, false)
		_ = bus.Publish(response)
	})

	bus.SubscribePrefix("user", "example/", func(e eventbus.Event) {
		fmt.Printf("[event] %s: %v\n", e.Topic, e.Payload)
	})

	go handleSignals(manager)

	log.Println("[gapid] supervisor running.")
	select {}
}

func handleSignals(dm *daemonmgr.DaemonManager) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
	log.Println("[gapid] shutdown signal received")
	dm.StopAll()
	os.Exit(0)
}

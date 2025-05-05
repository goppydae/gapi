package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/core/version"
	"github.com/goppydae/gapi/internal/daemonmgr"
	"github.com/goppydae/gapi/internal/eventbus"
	"github.com/goppydae/gapi/internal/transport"
)

var rootCmd = &cobra.Command{
	Use:   "gapid",
	Short: "GAPI Supervisor Daemon",
	Run: func(cmd *cobra.Command, args []string) {
		runSupervisor()
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version info",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(version.Summary())
	},
}

func runSupervisor() {
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

	log.Println("[gapid] supervisor running.")

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigs
	log.Printf("[gapid] received signal: %s", sig)
	manager.StopAll()
	log.Println("[gapid] exited cleanly")
}

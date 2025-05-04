package main

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/core/eventbus"
	"github.com/goppydae/gapi/internal/transport"
)

var rootCmd = &cobra.Command{
	Use:   "gapictl",
	Short: "GAPI Control CLI",
	Long:  "Base CLI for controlling GAPI-based systems and agents.",
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version info",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("gapictl v0.1.0")
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check daemon status over transport",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			log.Fatalf("failed to load config: %v", err)
		}

		t, err := transport.NewClientFromConfig(cfg.Transport)
		if err != nil {
			log.Fatalf("failed to init transport: %v", err)
		}

		bus := eventbus.NewEventBus(t)

		done := make(chan struct{})
		bus.SubscribePrefix("user", "system/pong", func(e eventbus.Event) {
			fmt.Printf("Received response: %s\n", e.Payload["status"])
			close(done)
		})

		err = bus.Publish(eventbus.NewEvent("user", "system/ping", "gapictl", map[string]string{"status": "ping"}, false))
		if err != nil {
			log.Fatalf("failed to send ping: %v", err)
		}

		<-done
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func RootCmd() *cobra.Command {
	return rootCmd
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(statusCmd)
}

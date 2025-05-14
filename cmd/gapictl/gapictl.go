package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/core/version"
	"github.com/goppydae/gapi/internal/eventbus"
	protopkg "github.com/goppydae/gapi/internal/proto"
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
		fmt.Print(version.Summary())
	},
}

var pingCmd = &cobra.Command{
	Use:   "ping",
	Short: "Ping daemon over transport",
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
			var pong protopkg.PingStatus
			if err := e.Payload.UnmarshalTo(&pong); err != nil {
				log.Printf("failed to unmarshal pong: %v", err)
				return
			}
			fmt.Printf("Received response: %s\n", pong.Status)
			close(done)
		})

		msg := &protopkg.PingStatus{Status: "ping"}
		anyMsg, err := anypb.New(msg)
		if err != nil {
			log.Fatalf("failed to pack ping: %v", err)
		}

		err = bus.Publish(eventbus.NewEvent("user", "system/ping", "gapictl", anyMsg, true))

		<-done
	},
}

var agentReloadCmd = &cobra.Command{
	Use:   "agent-reload",
	Short: "Reload agent manager configuration",
	Args:  cobra.MaximumNArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(version.Summary())
	},
}

var statusCmd = &cobra.Command{
	Use:   "status [PATTERN...]",
	Short: "Show runtime status of one or more agents",
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
		bus.SubscribePrefix("user", "system/agents.reply", func(e eventbus.Event) {
			var res protopkg.AgentStatusResponse
			if err := e.Payload.UnmarshalTo(&res); err != nil {
				log.Printf("failed to unmarshal agent status: %v", err)
				close(done)
				return
			}
			fmt.Println("Discovered Agents:")
			for _, agent := range res.Agents {
				if len(args) == 0 {
					fmt.Printf(" - %s (%s)\n", agent.Id, agent.Type)
					continue
				}
				for _, pattern := range args {
					if strings.Contains(agent.Id, pattern) {
						fmt.Printf(" - %s (%s)\n", agent.Id, agent.Type)
						break
					}
				}
			}
			close(done)
		})

		msg := &protopkg.AgentStatusRequest{}
		anyMsg, err := anypb.New(msg)
		if err != nil {
			log.Fatalf("failed to pack agent status request: %v", err)
		}

		err = bus.Publish(eventbus.NewEvent("user", "system/agents", "gapictl", anyMsg, true))
		if err != nil {
			log.Fatalf("failed to send status request: %v", err)
		}

		<-done
	},
}

var startCmd = &cobra.Command{
	Use:   "start AGENT...",
	Short: "Start one or more agents",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(version.Summary())
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop AGENT...",
	Short: "Stop one or more agents",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(version.Summary())
	},
}

var restartCmd = &cobra.Command{
	Use:   "restart AGENT...",
	Short: "Start or restart one or more agents",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(version.Summary())
	},
}

var reloadCmd = &cobra.Command{
	Use:   "reload AGENT...",
	Short: "Reload one or more agents",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(version.Summary())
	},
}

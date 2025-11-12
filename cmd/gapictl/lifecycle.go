package main

import (
	"fmt"
	"log"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/internal/eventbus"
	protopkg "github.com/goppydae/gapi/internal/proto"
	"github.com/goppydae/gapi/internal/transport"
)

// sendLifecycleCommand sends control commands in parallel to multiple agents
// and prints their results (success, error, or timeout).
func sendLifecycleCommand(agentIDs []string, action protopkg.LifecycleControl_Action) {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	t, err := transport.NewClientFromConfig[*anypb.Any](cfg.Transport)
	if err != nil {
		log.Fatalf("failed to init transport: %v", err)
	}

	bus := eventbus.NewEventBus(t)

	type result struct {
		AgentID string
		State   string
		Err     error
	}

	results := make(chan result, len(agentIDs))

	for _, agentID := range agentIDs {
		agentID := agentID // capture loop variable

		go func() {
			done := make(chan struct{})

			// Listen for status reply
			bus.SubscribeOnce("system", "agent/lifecycle.status", func(e eventbus.Event[*anypb.Any]) {
				var res protopkg.LifecycleStatus
				if err := e.UnmarshalPayload(&res); err != nil {
					results <- result{AgentID: agentID, Err: fmt.Errorf("unmarshal error: %v", err)}
					return
				}
				if res.AgentId == agentID {
					results <- result{AgentID: res.AgentId, State: res.State}
					close(done)
				}
			})

			msg := &protopkg.LifecycleControl{
				AgentId: agentID,
				Action:  action,
			}
			payload, err := anypb.New(msg)
			if err != nil {
				results <- result{AgentID: agentID, Err: fmt.Errorf("marshal error: %v", err)}
				return
			}

			event := eventbus.NewEvent("system", "agent/lifecycle.control", "gapictl", payload, true)
			if err := bus.Publish(event); err != nil {
				results <- result{AgentID: agentID, Err: fmt.Errorf("publish error: %v", err)}
				return
			}

			select {
			case <-done:
			case <-time.After(5 * time.Second):
				results <- result{AgentID: agentID, Err: fmt.Errorf("timeout")}
			}
		}()
	}

	// Collect all results
	for i := 0; i < len(agentIDs); i++ {
		res := <-results
		if res.Err != nil {
			fmt.Printf("❌ agent %s: %v\n", res.AgentID, res.Err)
		} else {
			fmt.Printf("✅ agent %s is now in state: %s\n", res.AgentID, res.State)
		}
	}
}

// Lifecycle CLI commands

var lifecycleStartCmd = &cobra.Command{
	Use:   "start AGENT...",
	Short: "Send start command to one or more agents",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		sendLifecycleCommand(args, protopkg.LifecycleControl_START)
	},
}

var lifecycleStopCmd = &cobra.Command{
	Use:   "stop AGENT...",
	Short: "Send stop command to one or more agents",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		sendLifecycleCommand(args, protopkg.LifecycleControl_STOP)
	},
}

var lifecycleRestartCmd = &cobra.Command{
	Use:   "restart AGENT...",
	Short: "Send restart command to one or more agents",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		sendLifecycleCommand(args, protopkg.LifecycleControl_RESTART)
	},
}

var lifecycleReloadCmd = &cobra.Command{
	Use:   "reload AGENT...",
	Short: "Send reload command to one or more agents",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		sendLifecycleCommand(args, protopkg.LifecycleControl_RELOAD)
	},
}

var lifecycleStatusCmd = &cobra.Command{
	Use:   "status AGENT...",
	Short: "Query lifecycle state of one or more agents",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		sendLifecycleCommand(args, protopkg.LifecycleControl_ACTION_UNSPECIFIED)
	},
}

// Register the lifecycle command group
func init() {
	lifecycleCmd := &cobra.Command{
		Use:   "lifecycle",
		Short: "Control agent lifecycle",
	}
	lifecycleCmd.AddCommand(
		lifecycleStartCmd,
		lifecycleStopCmd,
		lifecycleRestartCmd,
		lifecycleReloadCmd,
		lifecycleStatusCmd,
	)
	rootCmd.AddCommand(lifecycleCmd)
}

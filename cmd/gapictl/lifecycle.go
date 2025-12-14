package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/internal/eventbus"
	protopkg "github.com/goppydae/gapi/internal/proto"
	"github.com/goppydae/gapi/internal/transport"
)

func waitForPendingThenTerminal(
	ctx context.Context,
	bus *eventbus.EventBus[*anypb.Any],
	agentID string,
	pendingTimeout time.Duration,
	finalTimeout time.Duration,
) (*protopkg.LifecycleStatus, error) {

	statusCh := make(chan *protopkg.LifecycleStatus, 8)

	// Subscribe BEFORE publish; your EventBus doesn't return an unsub handle.
	bus.SubscribePrefix("system", "agent/lifecycle.status", func(e eventbus.Event[*anypb.Any]) {
		var st protopkg.LifecycleStatus
		if err := eventbus.UnmarshalAnyPayload(e, &st); err != nil {
			return
		}
		if st.GetAgentId() != agentID {
			return
		}
		select {
		case statusCh <- &st:
		default:
			// drop if full
		}
	})

	isTerminal := func(state string) bool {
		s := strings.ToUpper(strings.TrimSpace(state))
		switch s {
		case "PENDING", "STARTING", "STOPPING", "RELOADING", "INITIALIZING", "":
			return false
		default:
			return true // RUNNING, STOPPED, FAILED, INITIALIZED, or anything else non-empty
		}
	}

	// Timers: start with PENDING window, then switch to terminal window.
	pendingTimer := time.NewTimer(pendingTimeout)
	defer pendingTimer.Stop()

	finalTimer := time.NewTimer(finalTimeout)
	if !finalTimer.Stop() {
		select {
		case <-finalTimer.C:
		default:
		}
	}
	activeTimer := pendingTimer.C
	seenPending := false

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case <-activeTimer:
			if !seenPending {
				return nil, fmt.Errorf("timeout waiting for PENDING")
			}
			return nil, fmt.Errorf("timeout waiting for terminal state")

		case st := <-statusCh:
			// Terminal may arrive before PENDING.
			if isTerminal(st.GetState()) {
				return st, nil
			}
			if !seenPending && st.GetState() == "PENDING" {
				seenPending = true
				if !pendingTimer.Stop() {
					select {
					case <-pendingTimer.C:
					default:
					}
				}
				finalTimer.Reset(finalTimeout)
				activeTimer = finalTimer.C
				continue
			}
			// Ignore other non-terminal updates.
		}
	}
}

// sendLifecycleCommand sends control commands in parallel to multiple agents,
// waits for PENDING then terminal, and prints results.
func sendLifecycleCommand(agentIDs []string, action protopkg.LifecycleControl_Action) {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	t, err := transport.NewClientFromConfig(cfg.Transport)
	if err != nil {
		log.Fatalf("failed to init transport: %v", err)
	}
	bus := eventbus.NewEventBus(t)

	type result struct {
		AgentID string
		Status  *protopkg.LifecycleStatus
		Err     error
	}

	results := make(chan result, len(agentIDs))

	for _, id := range agentIDs {
		agentID := id // capture loop var

		go func() {
			// 1) Subscribe first and kick off the waiter
			ctx := context.Background()

			// 2) Publish the control request
			req := &protopkg.LifecycleControl{AgentId: agentID, Action: action}
			packed, err := anypb.New(req)
			if err != nil {
				results <- result{AgentID: agentID, Err: fmt.Errorf("marshal request: %w", err)}
				return
			}
			ev := eventbus.NewEvent("system", "agent/lifecycle.action", "gapictl", packed, true)
			if err := bus.Publish(ev); err != nil {
				results <- result{AgentID: agentID, Err: fmt.Errorf("publish control: %w", err)}
				return
			}

			// 3) Await PENDING then terminal
			st, err := waitForPendingThenTerminal(ctx, bus, agentID, 2*time.Second, 20*time.Second)
			results <- result{AgentID: agentID, Status: st, Err: err}
		}()
	}

	// Collect results
	for i := 0; i < len(agentIDs); i++ {
		r := <-results
		if r.Err != nil {
			fmt.Printf("❌ %s: %v\n", r.AgentID, r.Err)
			continue
		}
		fmt.Printf("✅ %s → %s: %s\n", r.AgentID, r.Status.GetState(), r.Status.GetMessage())
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

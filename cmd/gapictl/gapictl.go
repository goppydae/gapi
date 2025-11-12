package main

import (
	"fmt"
	"log"
	"strings"
	"time"

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
	Long:  "CLI for controlling GAPI agents and supervisors.",
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version info",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(version.Summary())
	},
}

// ping
var pingCmd = &cobra.Command{
	Use:   "ping",
	Short: "Ping gapid",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, _ := config.Load()
		t, _ := transport.NewClientFromConfig[*anypb.Any](cfg.Transport)
		bus := eventbus.NewEventBus(t)

		done := make(chan struct{})
		bus.SubscribeOnce("system", "system/pong", func(e eventbus.Event[*anypb.Any]) {
			var pong protopkg.PingStatus
			_ = e.Payload.UnmarshalTo(&pong)
			fmt.Println("message:", pong.Status)
			close(done)
		})

		msg := &protopkg.PingStatus{Status: "ping"}
		payload, _ := anypb.New(msg)
		_ = bus.Publish(eventbus.NewEvent("system", "system/ping", "gapictl", payload, true))
		<-done
	},
}

// agent-reload
var agentReloadCmd = &cobra.Command{
	Use:   "agent-reload",
	Short: "Trigger a reload of registered agents",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, _ := config.Load()
		t, _ := transport.NewClientFromConfig[*anypb.Any](cfg.Transport)
		bus := eventbus.NewEventBus[*anypb.Any](t)
		evt := eventbus.NewEvent[*anypb.Any]("system", "system/agent.reload", "gapictl", nil, true)
		_ = bus.Publish(evt)
		fmt.Println("reload event dispatched")
	},
}

// agent-status
var agentStatusCmd = &cobra.Command{
	Use:   "agent-status [PATTERN...]",
	Short: "Show current registered agents",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, _ := config.Load()
		t, _ := transport.NewClientFromConfig[*anypb.Any](cfg.Transport)
		bus := eventbus.NewEventBus(t)

		done := make(chan struct{})
		bus.SubscribeOnce("system", "system/agents.reply", func(e eventbus.Event[*anypb.Any]) {
			var res protopkg.AgentStatusResponse
			_ = e.Payload.UnmarshalTo(&res)
			for _, agent := range res.Agents {
				if len(args) == 0 {
					fmt.Printf(" - %s (%s) [%s]\n", agent.Id, agent.Type, toProtoState(agent.State))
					continue
				}
				for _, pat := range args {
					if strings.Contains(agent.Id, pat) {
						fmt.Printf(" - %s (%s) [%s]\n", agent.Id, agent.Type, toProtoState(agent.State))
						break
					}
				}
			}
			close(done)
		})

		req := &protopkg.AgentStatusRequest{}
		packed, _ := anypb.New(req)
		bus.Publish(eventbus.NewEvent("system", "system/agents/", "gapictl", packed, true))
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			log.Println("timeout: no response")
		}
	},
}

func toProtoState(s protopkg.AgentState) string {
	switch s {
	case protopkg.AgentState_AGENT_STATE_INIT:
		return "initializing"
	case protopkg.AgentState_AGENT_STATE_STARTING:
		return "starting"
	case protopkg.AgentState_AGENT_STATE_RUNNING:
		return "running"
	case protopkg.AgentState_AGENT_STATE_STOPPING:
		return "stopping"
	case protopkg.AgentState_AGENT_STATE_STOPPED:
		return "stopped"
	case protopkg.AgentState_AGENT_STATE_FAILED:
		return "failed"
	case protopkg.AgentState_AGENT_STATE_RELOADING:
		return "reloading"
	default:
		return "unknown"
	}
}

package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/cmd/gapictl/tui"
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
		t, _ := transport.NewClientFromConfig(cfg.Transport)
		bus := eventbus.NewEventBus(t)

		done := make(chan struct{})
		bus.SubscribeOnce("system", "pong", func(e eventbus.Event[*anypb.Any]) {
			var pong protopkg.PingStatus
			_ = e.Payload.UnmarshalTo(&pong)
			fmt.Println("message:", pong.Status)
			close(done)
		})

		msg := &protopkg.PingStatus{Status: "ping"}
		payload, _ := anypb.New(msg)
		_ = bus.Publish(eventbus.NewEvent("system", "ping", "gapictl", payload, true))
		<-done
	},
}

// agent-reload
var agentReloadCmd = &cobra.Command{
	Use:   "agent-reload",
	Short: "Trigger a reload of registered agents",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, _ := config.Load()
		t, _ := transport.NewClientFromConfig(cfg.Transport)
		bus := eventbus.NewEventBus[*anypb.Any](t)
		evt := eventbus.NewEvent[*anypb.Any]("system", "agent.reload", "gapictl", nil, true)
		_ = bus.Publish(evt)
		fmt.Println("reload event dispatched")
	},
}

// agent-status
var agentStatusCmd = &cobra.Command{
	Use:   "agent-status [PATTERN...]",
	Short: "Show current registered agents",
	Run: func(cmd *cobra.Command, args []string) {
		treeView, _ := cmd.Flags().GetBool("tree")
		cfg, err := config.Load()
		if err != nil {
			log.Fatalf("failed to load config: %v", err)
		}
		t, err := transport.NewClientFromConfig(cfg.Transport)
		if err != nil {
			log.Fatalf("failed to create transport: %v", err)
		}
		bus := eventbus.NewEventBus[*anypb.Any](t)

		done := make(chan struct{})
		bus.SubscribeOnce("system", "agents.reply", func(e eventbus.Event[*anypb.Any]) {
			var res protopkg.AgentStatusResponse
			_ = e.Payload.UnmarshalTo(&res)

			// Filter first
			var filtered []*protopkg.AgentStatus
			for _, agent := range res.Agents {
				if len(args) == 0 {
					filtered = append(filtered, agent)
					continue
				}
				for _, pat := range args {
					if strings.Contains(agent.Id, pat) {
						filtered = append(filtered, agent)
						break
					}
				}
			}

			if !treeView {
				// Flat view
				for _, agent := range filtered {
					deps := ""
					if len(agent.Dependencies) > 0 {
						deps = fmt.Sprintf(" (deps: %s)", strings.Join(agent.Dependencies, ", "))
					}
					fmt.Printf(" - %s (%s) [%s]%s\n", agent.Id, agent.Type, toProtoState(agent.State), deps)
				}
			} else {
				// Tree View
				printTree(filtered)
			}
			close(done)
		})

		req := &protopkg.AgentStatusRequest{}
		packed, _ := anypb.New(req)
		bus.Publish(eventbus.NewEvent("system", "agents/", "gapictl", packed, true))
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			log.Println("timeout: no response")
		}
	},
}

// TUI command
var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive TUI for monitoring agents",
	Long:  "Start an interactive terminal UI for real-time agent monitoring and control.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.Run()
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(pingCmd)
	rootCmd.AddCommand(agentStatusCmd)
	rootCmd.AddCommand(keygenCmd)
	rootCmd.AddCommand(signCmd)
	rootCmd.AddCommand(verifyCmd)
	rootCmd.AddCommand(tuiCmd)

	agentStatusCmd.Flags().BoolP("tree", "t", false, "Show dependency tree")
}

func printTree(agents []*protopkg.AgentStatus) {
	byId := make(map[string]*protopkg.AgentStatus)
	inDegree := make(map[string]int)

	for _, a := range agents {
		byId[a.Id] = a
		if _, ok := inDegree[a.Id]; !ok {
			inDegree[a.Id] = 0
		}
		for _, dep := range a.Dependencies {
			inDegree[dep]++
		}
	}

	// Roots are those with in-degree 0 (or not in the list at all, but we only print what we have)
	var roots []*protopkg.AgentStatus
	for _, a := range agents {
		if inDegree[a.Id] == 0 {
			roots = append(roots, a)
		}
	}

	// If cycle or everything depends on something (e.g. A->B->A), pick arbitrary or just print all?
	// Fallback: if no roots but agents exist, print all.
	if len(roots) == 0 && len(agents) > 0 {
		roots = agents
	}

	var printNode func(id string, prefix string, isLast bool, visited map[string]bool)
	printNode = func(id string, prefix string, isLast bool, visited map[string]bool) {
		agent, ok := byId[id]
		if !ok {
			// Dependency not in list (maybe filtered out or missing?)
			nodeStr := fmt.Sprintf("%s [missing]", id)
			marker := "├── "
			if isLast {
				marker = "└── "
			}
			fmt.Printf("%s%s%s\n", prefix, marker, nodeStr)
			return
		}

		// Print self
		status := toProtoState(agent.State)
		marker := "├── "
		if isLast {
			marker = "└── "
		}
		fmt.Printf("%s%s%s (%s) [%s]\n", prefix, marker, agent.Id, agent.Type, status)

		if visited[id] {
			return // cycle break
		}
		visited[id] = true

		newPrefix := prefix + "│   "
		if isLast {
			newPrefix = prefix + "    "
		}

		for i, dep := range agent.Dependencies {
			isLastDep := i == len(agent.Dependencies)-1
			printNode(dep, newPrefix, isLastDep, copyMap(visited))
		}
	}

	for _, root := range roots {
		// Roots don't need prefix/marker logic exactly like children, but let's emulate `tree`
		// We treat roots as top level list.
		// However, standard `tree` output starts with `.`, here we list roots.

		// We'll reset visited for each root to allow shared subtrees to print fully?
		// Systemd prints shared subtrees fully.
		printNode(root.Id, "", true, make(map[string]bool))
		// Note: passing true for isLast is hacky for roots, but works for the marker.
		// Actually, for multiple roots, we should handle them as a list too.
	}
}

func copyMap(m map[string]bool) map[string]bool {
	out := make(map[string]bool)
	for k, v := range m {
		out[k] = v
	}
	return out
}

func toProtoState(s protopkg.AgentState) string {
	switch s {
	case protopkg.AgentState_AGENT_STATE_INITIALIZING:
		return "initializing"
	case protopkg.AgentState_AGENT_STATE_INITIALIZED:
		return "initialized"
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

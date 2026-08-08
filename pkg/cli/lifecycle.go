// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	protopkg "github.com/goppydae/gapi/pkg/proto"
)

// sendLifecycleCommand sends control commands in parallel to multiple agents.
//
// IT RETURNS AN ERROR BECAUSE A FAILED LIFECYCLE ACTION USED TO EXIT 0.
// Every one of these commands was wired through cobra's Run, which has
// no error return, so a request that was never delivered printed
// "[FAIL] ..." and exited SUCCESSFULLY. Any caller reading the exit
// status - which is the only thing a shell or a test harness can read -
// was told the action had been applied.
//
// That is GAPI-DIV-120's finding applied to the verbs it did not cover.
// `gapictl ping` had exactly this defect and was fixed; the five
// lifecycle verbs kept it. test/adk's SendLifecycleAction checks the
// process exit code and nothing else, so a lost stop was indistinguishable
// from a delivered one, and the suite then spent sixty seconds waiting
// for a transition nobody had asked for.
//
// EVERY failure is reported, not the first: the results are collected
// across agents and the loop must print all of them before returning, or
// naming one failed agent would hide the rest.
func sendLifecycleCommand(agentIDs []string, action protopkg.LifecycleControl_Action) error {
	cfg, err := controlConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	c, err := newControlClient(cfg)
	if err != nil {
		return fmt.Errorf("init client: %w", err)
	}

	defer closeControlClient(c)

	// Long timeout for lifecycle
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	results := c.Lifecycle(ctx, agentIDs, action)

	failed := 0
	// A DELIVERED REQUEST THE DAEMON REFUSED IS NOT A SUCCESS, and
	// checking r.Err alone could not tell the difference. r.Err reports
	// whether the REQUEST completed; the returned state reports whether
	// the ACTION did. Measured against a live daemon before this changed:
	//
	//     $ gapictl lifecycle stop nosuchagent
	//     [OK] nosuchagent -> FAILED: unknown agent
	//     exit 0
	//
	// The daemon answered plainly that it had not done the thing, and the
	// client printed [OK] and exited successfully, because the only
	// question asked was whether the round trip worked.
	//
	// A QUERY IS EXEMPT, and the distinction is not cosmetic. `lifecycle
	// status` on an agent that is genuinely FAILED is a SUCCESSFUL query
	// returning a true answer; making it exit non-zero would break every
	// caller that asks after a broken agent in order to report on it. An
	// ACTION resulting in FAILED is a failed action. Same field, two
	// meanings, discriminated by what was asked.
	isQuery := action == protopkg.LifecycleControl_ACTION_UNSPECIFIED

	for _, r := range results {
		switch {
		case r.Err != nil:
			fmt.Printf("[FAIL] %s: %v\n", r.AgentID, r.Err)
			failed++
		case !isQuery && isFailedState(r.Status.GetState()):
			fmt.Printf("[FAIL] %s -> %s: %s\n", r.AgentID, r.Status.GetState(), r.Status.GetMessage())
			failed++
		default:
			fmt.Printf("[OK] %s -> %s: %s\n", r.AgentID, r.Status.GetState(), r.Status.GetMessage())
		}
	}

	if failed > 0 {
		return fmt.Errorf("%d of %d agents failed", failed, len(results))
	}
	return nil
}

// isFailedState reports whether a lifecycle state string means the
// action failed.
//
// IT MATCHES TWO SPELLINGS BECAUSE THE WIRE CARRIES TWO, and that is
// GAPI-DIV-083 - the runtime state surface is a string rather than the
// declared enum, so nothing holds its values to one vocabulary. The
// supervisor writes the bare literal "FAILED" on one path while other
// paths carry AgentState.String(), which renders the same condition as
// "AGENT_STATE_FAILED".
//
// Matching both is DELIBERATE, not defensive padding. Matching one would
// make this gate silently blind to half the daemon's own outputs, which
// is the failure mode the gate exists to end. When -083 lands and the
// state becomes the enum it declares, this function collapses to a
// single comparison and its caller does not change.
func isFailedState(state string) bool {
	return state == "FAILED" || state == protopkg.AgentState_AGENT_STATE_FAILED.String()
}

// Lifecycle CLI commands

var lifecycleStartCmd = &cobra.Command{
	Use:   "start AGENT...",
	Short: "Send start command to one or more agents",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return sendLifecycleCommand(args, protopkg.LifecycleControl_ACTION_START)
	},
}

var lifecycleStopCmd = &cobra.Command{
	Use:   "stop AGENT...",
	Short: "Send stop command to one or more agents",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return sendLifecycleCommand(args, protopkg.LifecycleControl_ACTION_STOP)
	},
}

var lifecycleRestartCmd = &cobra.Command{
	Use:   "restart AGENT...",
	Short: "Send restart command to one or more agents",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return sendLifecycleCommand(args, protopkg.LifecycleControl_ACTION_RESTART)
	},
}

var lifecycleReloadCmd = &cobra.Command{
	Use:   "reload AGENT...",
	Short: "Send reload command to one or more agents",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return sendLifecycleCommand(args, protopkg.LifecycleControl_ACTION_RELOAD)
	},
}

var lifecycleStatusCmd = &cobra.Command{
	Use:   "status AGENT...",
	Short: "Query lifecycle state of one or more agents",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return sendLifecycleCommand(args, protopkg.LifecycleControl_ACTION_UNSPECIFIED)
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

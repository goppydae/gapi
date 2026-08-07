// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/core/eventbus"
	"github.com/goppydae/gapi/core/transport"
	"github.com/goppydae/gapi/internal/statewatch"
	protopkg "github.com/goppydae/gapi/pkg/proto"
)

// Client provides a programmatic interface to interact with a running GAPI daemon.
type Client struct {
	bus *eventbus.EventBus[*anypb.Any]
}

// Result represents the outcome of a lifecycle action on a specific agent.
type Result struct {
	AgentID string
	Status  *protopkg.LifecycleStatus
	Err     error
}

// New creates a new Client using the provided configuration.
func New(cfg *config.Config) (*Client, error) {
	t, err := transport.NewClientFromConfig(cfg.Transport)
	if err != nil {
		return nil, fmt.Errorf("failed to init transport: %w", err)
	}
	bus := eventbus.NewEventBus[*anypb.Any](t)
	return &Client{bus: bus}, nil
}

// NewFromBus creates a new Client using an existing EventBus (for in-process use).
func NewFromBus(bus *eventbus.EventBus[*anypb.Any]) *Client {
	return &Client{bus: bus}
}

// Ping sends a ping to the daemon and waits for a pong.
func (c *Client) Ping(ctx context.Context) (string, error) {
	done := make(chan string, 1)
	errCh := make(chan error, 1)

	msg := &protopkg.PingStatus{Status: "ping"}
	payload, err := anypb.New(msg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal ping: %w", err)
	}
	req := eventbus.NewEvent("system", "", "ping", "client", payload)

	// Correlate the reply to this request's ID so concurrent Ping callers don't
	// steal each other's pong. The daemon echoes the request ID onto the reply.
	// NON-BLOCKING sends, because the retry below can draw more than one
	// reply for a single request ID and both channels hold exactly one.
	// A second blocking send would park this handler goroutine forever.
	if err := c.bus.SubscribeCorrelated("system", "", "pong", req.ID, func(e eventbus.Event[*anypb.Any]) {
		var pong protopkg.PingStatus
		if err := e.Payload.UnmarshalTo(&pong); err != nil {
			select {
			case errCh <- fmt.Errorf("failed to unmarshal pong: %w", err):
			default:
			}
			return
		}
		select {
		case done <- pong.Status:
		default:
		}
	}); err != nil {
		return "", fmt.Errorf("failed to subscribe to pong: %w", err)
	}

	// Publishing and retrying both belong to awaitCorrelated; see it for
	// why a read may be republished and a lifecycle verb may not.
	return awaitCorrelated(ctx, c, req, done, errCh)
}

// requestRetryInterval is how often an idempotent request is republished
// while awaiting its correlated reply.
//
// Chosen against the measured cost of the window it covers rather than
// picked round: agent bring-up held the subscription off for ~0.3s in
// the ADK fixture set, so an interval well under that closes the gap in
// a handful of attempts while staying negligible against a 30s deadline.
const requestRetryInterval = 250 * time.Millisecond

// awaitCorrelated publishes req and waits for its correlated reply,
// REPUBLISHING on an interval until the reply lands or ctx expires.
//
// GAPI-DIV-120 and -122. A request published while the daemon is still
// in setupAgents has no subscriber and is silently dropped, and a single
// publish then waits out the entire deadline for a reply nobody will
// send. Measured in a clean NixOS guest: `gapictl agent status` issued
// immediately after wait_for_unit reported the request at 17:06:05 and
// gave up at 17:06:35 - exactly the client's 30s deadline, with one call
// and no load.
//
// FOR IDEMPOTENT READS ONLY, and that restriction is the whole design.
// Re-publishing a read costs nothing and the correlated subscription
// discards the duplicate replies. Re-publishing a MUTATING verb could
// execute it twice: operator decision 42 records four concurrent starts
// spawning four processes. So Ping and AgentStatus use this and the
// lifecycle verbs deliberately do not - see GAPI-DIV-122, which names
// each exclusion.
//
// The SAME req is republished, deliberately. The caller's subscription
// is correlated to req.ID, so a fresh event per attempt would need a
// fresh subscription and would leak one per retry.
//
// A free function rather than a method: Go does not allow type
// parameters on methods, and the reply type differs per caller.
func awaitCorrelated[T any](
	ctx context.Context,
	c *Client,
	req eventbus.Event[*anypb.Any],
	done <-chan T,
	errCh <-chan error,
) (T, error) {
	var zero T

	if err := c.bus.Publish(req); err != nil {
		return zero, fmt.Errorf("publish %s: %w", req.Topic, err)
	}

	ticker := time.NewTicker(requestRetryInterval)
	defer ticker.Stop()

	for {
		select {
		case v := <-done:
			return v, nil
		case err := <-errCh:
			return zero, err
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-ticker.C:
			if err := c.bus.Publish(req); err != nil {
				return zero, fmt.Errorf("republish %s: %w", req.Topic, err)
			}
		}
	}
}

// ReloadAgents triggers a reload of the agent registry on the daemon.
func (c *Client) ReloadAgents(ctx context.Context) error {
	evt := eventbus.NewEvent[*anypb.Any]("system", "", "agent.reload", "client", nil)
	if err := c.bus.Publish(evt); err != nil {
		return fmt.Errorf("failed to publish reload: %w", err)
	}
	return nil
}

// Shutdown requests a system shutdown from the daemon. action is one
// of "poweroff", "reboot", "halt" (the topic's documented payload
// vocabulary); in PID-1 mode the daemon answers with the full init
// teardown.
func (c *Client) Shutdown(ctx context.Context, action string) error {
	payload, err := anypb.New(wrapperspb.String(action))
	if err != nil {
		return fmt.Errorf("encode shutdown action: %w", err)
	}
	evt := eventbus.NewEvent[*anypb.Any]("system", "", eventbus.TopicSystemShutdown, "client", payload)
	if err := c.bus.Publish(evt); err != nil {
		return fmt.Errorf("failed to publish shutdown: %w", err)
	}
	return nil
}

// AgentStatus queries the daemon for the status of all agents.
func (c *Client) AgentStatus(ctx context.Context) ([]*protopkg.AgentStatus, error) {
	done := make(chan []*protopkg.AgentStatus, 1)
	errCh := make(chan error, 1)

	reqMsg := &protopkg.AgentStatusRequest{}
	packed, err := anypb.New(reqMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal status request: %w", err)
	}
	req := eventbus.NewEvent("system", "", "agents/", "client", packed)

	// Correlate the reply to this request's ID so concurrent status callers don't
	// steal each other's reply. The daemon echoes the request ID onto the reply.
	// NON-BLOCKING sends, because awaitCorrelated may republish and draw
	// more than one reply for a single request id while both channels hold
	// exactly one. A second blocking send would park this handler forever.
	if err := c.bus.SubscribeCorrelated("system", "", "agents.reply", req.ID, func(e eventbus.Event[*anypb.Any]) {
		var res protopkg.AgentStatusResponse
		if err := e.Payload.UnmarshalTo(&res); err != nil {
			select {
			case errCh <- fmt.Errorf("failed to unmarshal agent status: %w", err):
			default:
			}
			return
		}
		select {
		case done <- res.Agents:
		default:
		}
	}); err != nil {
		return nil, fmt.Errorf("failed to subscribe to agents.reply: %w", err)
	}

	// RETRIES, and it is safe to because a status read is idempotent
	// (GAPI-DIV-122). The daemon subscribes agents/ only after setupAgents
	// returns, so a request issued during bring-up is dropped and this call
	// otherwise waits out its whole deadline - measured at exactly 30s in a
	// clean NixOS guest, which is what reddened the VM check.
	//
	// Answering that request EARLY was the rejected alternative: the
	// handler reads the registry, so subscribing it before bring-up
	// finishes returns a confidently PARTIAL agent list. Retrying yields
	// the complete one, because the republish lands after setupAgents.
	return awaitCorrelated(ctx, c, req, done, errCh)
}

// LifecycleOptions defines optional parameters for lifecycle actions.
type LifecycleOptions struct {
	Env           map[string]string
	RestartPolicy string
}

// Lifecycle sends a lifecycle action to a set of agents and waits for their transition.
func (c *Client) Lifecycle(ctx context.Context, agentIDs []string, action protopkg.LifecycleControl_Action) []Result {
	return c.LifecycleWithOpts(ctx, agentIDs, action, LifecycleOptions{})
}

// LifecycleWithOpts sends a lifecycle action with options.
func (c *Client) LifecycleWithOpts(ctx context.Context, agentIDs []string, action protopkg.LifecycleControl_Action, opts LifecycleOptions) []Result {
	results := make(chan Result, len(agentIDs))

	for _, id := range agentIDs {
		agentID := id // capture loop var
		go func() {
			// Publish control request
			req := &protopkg.LifecycleControl{
				AgentId:       agentID,
				Action:        action,
				Env:           opts.Env,
				RestartPolicy: opts.RestartPolicy,
			}
			packed, err := anypb.New(req)
			if err != nil {
				results <- Result{AgentID: agentID, Err: fmt.Errorf("marshal request: %w", err)}
				return
			}
			ev := eventbus.NewEvent("system", "", eventbus.TopicAgentLifecycleAction, "client", packed)
			if err := c.bus.Publish(ev); err != nil {
				results <- Result{AgentID: agentID, Err: fmt.Errorf("publish control: %w", err)}
				return
			}

			// Await transition
			st, err := statewatch.WaitForPendingThenTerminal(
				ctx, c.bus, agentID, config.ClientPendingTimeout, config.ClientTerminalTimeout,
			)
			results <- Result{AgentID: agentID, Status: st, Err: err}
		}()
	}

	var finalResults []Result
	for i := 0; i < len(agentIDs); i++ {
		finalResults = append(finalResults, <-results)
	}
	return finalResults
}

// Start triggers the START action for the given agents.
func (c *Client) Start(ctx context.Context, agentIDs []string) []Result {
	return c.Lifecycle(ctx, agentIDs, protopkg.LifecycleControl_ACTION_START)
}

// StartWithOpts triggers the START action with options.
func (c *Client) StartWithOpts(ctx context.Context, agentIDs []string, opts LifecycleOptions) []Result {
	return c.LifecycleWithOpts(ctx, agentIDs, protopkg.LifecycleControl_ACTION_START, opts)
}

// Stop triggers the STOP action for the given agents.
func (c *Client) Stop(ctx context.Context, agentIDs []string) []Result {
	return c.Lifecycle(ctx, agentIDs, protopkg.LifecycleControl_ACTION_STOP)
}

// GetLogs subscribes to the logs for a specific agent and returns a channel of log lines.
// The subscription is automatically cleaned up when the context is cancelled.
func (c *Client) GetLogs(ctx context.Context, agentID string) (<-chan string, error) {
	ch := make(chan string, 100)

	err := c.bus.SubscribePrefixWithContext(ctx, "system", "", "logs", func(e eventbus.Event[*anypb.Any]) {
		var logMsg protopkg.LogMessage
		if err := e.Payload.UnmarshalTo(&logMsg); err != nil {
			return
		}

		// Filter by agent ID
		if logMsg.GetAgentId() != agentID {
			return
		}

		// Format log line
		ts := time.UnixMilli(logMsg.GetTimestamp()).UTC().Format(time.RFC3339Nano)
		msg := fmt.Sprintf("[%s] %s [%s]: %s",
			ts,
			logMsg.GetAgentId(),
			logMsg.GetLevel(),
			logMsg.GetMessage())

		select {
		case ch <- msg:
		case <-ctx.Done():
		default: // drop if full to avoid blocking bus
		}
	})

	if err != nil {
		return nil, err
	}

	return ch, nil
}

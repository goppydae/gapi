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
	// ownsBus decides whether Close may shut the bus down.
	//
	// EXPLICIT BECAUSE THE TWO CONSTRUCTORS HAND OUT DIFFERENT
	// OWNERSHIP. New dials a transport this Client alone holds, so
	// closing it is the whole point. NewFromBus wraps a bus the CALLER
	// built and may still be using, and a Close that shut it down would
	// destroy a daemon's own event bus from a helper that merely
	// borrowed it. No caller does that today - NewFromBus has no
	// non-test users - but it is exported, and an exported constructor
	// whose result cannot safely be closed is the kind of surprise this
	// repository has spent a great deal of time removing.
	ownsBus bool
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
	return &Client{bus: bus, ownsBus: true}, nil
}

// NewFromBus creates a new Client using an existing EventBus (for in-process use).
//
// The returned Client does NOT own the bus: Close is a no-op on it, and
// the caller that built the bus remains responsible for shutting it down.
func NewFromBus(bus *eventbus.EventBus[*anypb.Any]) *Client {
	return &Client{bus: bus}
}

// Close releases the daemon connection this Client dialled.
//
// WITHOUT IT EVERY INVOCATION LEFT A DEAD PEER IN THE DAEMON FOR A FULL
// MINUTE (GAPI-DIV-124). A gapictl process exited without a graceful
// QUIC close, so the daemon learned of it only when MaxIdleTimeout
// expired - config.QUICIdleTimeout, 60 seconds - at which point
// handleConn's AcceptStream returned and the deferred delete removed the
// peer. The daemon therefore carried every control invocation of the
// last minute in its peer set, and fanned every reply out to all of
// them.
//
// Idempotent, because callers defer it and some paths also close
// explicitly: EventBus.Close guards on its own closed flag.
func (c *Client) Close() error {
	if !c.ownsBus || c.bus == nil {
		return nil
	}
	return c.bus.Close()
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

// requestRetryAttempts BOUNDS the republishing, and the bound is the
// point rather than a tidiness.
//
// The window this retry crosses is agent bring-up, measured at 0.02s to
// a few hundred milliseconds - eight attempts covers it with a wide
// margin. Republishing for the FULL deadline instead would turn one
// status call into ~120 requests against a daemon that is already slow,
// which is request amplification aimed at the exact condition that
// makes a suite flaky. Nothing measured says that amplification caused
// a failure; the cap means nothing has to.
//
// After the cap the original deadline still rides, so a genuinely
// unresponsive daemon fails exactly as it did before any retry existed.
const requestRetryAttempts = 8

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

	// Publish, NOT PublishRequest, AND THE RETRY IS THE REASON.
	// PublishRequest reports a remote send that did not happen, which is
	// what a SINGLE-SHOT request needs - failing fast on it here would
	// defeat the very window this function exists to cross. A daemon that
	// is listening but has not yet subscribed, or a peer set that is
	// momentarily empty, is exactly what republishing answers. So the
	// paths with no retry (ReloadAgents, Shutdown, the lifecycle verbs)
	// use PublishRequest and these two do not, which is the same split
	// GAPI-DIV-122 draws between idempotent reads and mutating verbs.
	//
	// RESIDUAL, recorded rather than fixed here: a request whose every
	// attempt failed to SEND still ends as a bare context deadline, so it
	// reads identically to one the daemon simply never answered. Removing
	// that ambiguity means retaining the last transport error and
	// reporting it on expiry, which is a change to this function's
	// contract and does not belong in a conflict resolution.
	if err := c.bus.Publish(req); err != nil {
		return zero, fmt.Errorf("publish %s: %w", req.Topic, err)
	}

	ticker := time.NewTicker(requestRetryInterval)
	defer ticker.Stop()

	attempts := 0
	for {
		select {
		case v := <-done:
			return v, nil
		case err := <-errCh:
			return zero, err
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-ticker.C:
			if attempts >= requestRetryAttempts {
				// Stop republishing and keep waiting out ctx. Stopping
				// the ticker is what ends the resends: its channel never
				// fires again, so this arm is unreachable afterwards.
				ticker.Stop()
				continue
			}
			attempts++
			if err := c.bus.Publish(req); err != nil {
				return zero, fmt.Errorf("republish %s: %w", req.Topic, err)
			}
		}
	}
}

// ReloadAgents triggers a reload of the agent registry on the daemon.
func (c *Client) ReloadAgents(ctx context.Context) error {
	evt := eventbus.NewEvent[*anypb.Any]("system", "", "agent.reload", "client", nil)
	// A COMMAND THAT WAS NEVER SENT MUST NOT REPORT SUCCESS. Nothing
	// here waits for a reply, so this call's return value is the only
	// thing the operator ever sees - and `gapictl lifecycle stop` exiting
	// 0 for a refused action is a defect this repository has already
	// fixed once. Publish would report nil for a send with no peer.
	if err := c.bus.PublishRequest(evt); err != nil {
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
	// Same reasoning as ReloadAgents: nothing waits for a reply, so this
	// return value is the whole report, and a shutdown that never left
	// the process must not read as a shutdown that was accepted.
	if err := c.bus.PublishRequest(evt); err != nil {
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
			// THIS IS THE PATH THE 2s FAILURES CAME THROUGH. One publish,
			// no retry, then a 2s wait for PENDING - so a send that never
			// happened surfaces as "timeout waiting for PENDING", which is
			// the assertion five occurrences of the test/adk flake carried
			// and is a statement about the SUPERVISOR. PublishRequest makes
			// it a statement about the send instead.
			//
			// NOT awaitCorrelated, and GAPI-DIV-122 draws that line: a
			// lifecycle verb MUTATES, and operator decision 42 records four
			// concurrent starts spawning four processes. Republishing one
			// would risk executing it twice, so this path gets honesty
			// about the send instead of a retry.
			if err := c.bus.PublishRequest(ev); err != nil {
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

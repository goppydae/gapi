package agent

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Run is the agent's entry point, called by the generated main.
//
// It owns flag dispatch, and that is the point rather than a convenience:
// GoAgent.Start execs the binary as '<path> --start', and while every Go
// template and fixture hand-wrote its own flag set declaring only
// -describe, flag.Parse rejected --start and the process exited 2 before
// a line of agent code ran. The compiled-Go supervised path had therefore
// never executed at all (GAPI-DIV-052). An agent that does not understand
// the verb its supervisor invokes cannot exist if no author writes the
// parsing.
func Run() {
	os.Exit(run(os.Args[1:]))
}

// run is Run's testable body: it returns the exit code instead of taking
// it, so the dispatch table can be tested without a subprocess per case.
func run(args []string) int {
	spec, err := registeredSpec()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fs := flag.NewFlagSet("agent", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var (
		describe = fs.Bool("describe", false, "print the agent's metadata as JSON and exit")
		start    = fs.Bool("start", false, "run the agent under a supervisor")
		stop     = fs.Bool("stop", false, "invoke the agent's Stop function and exit")
		reload   = fs.Bool("reload", false, "invoke the agent's Reload function and exit")
		restart  = fs.Bool("restart", false, "invoke the agent's Restart function and exit")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	switch {
	case *describe:
		return emitDescribe(spec)
	case *start:
		return supervised(spec, newControl())
	case *stop:
		return invoke(spec.Stop, "stop")
	case *reload:
		return invoke(spec.Reload, "reload")
	case *restart:
		return invoke(spec.Restart, "restart")
	default:
		// No verb is the timer path: TimerAgent.fireCommand runs a
		// non-Python agent with NO arguments for a single firing, and
		// waits for the process to exit before scheduling the next one
		// (core/agentmgr/timer_agent.go). Treating a bare invocation as
		// "run once and return" is what makes one binary serve both the
		// supervised and the scheduled path.
		return fire(spec)
	}
}

func emitDescribe(s *Spec) int {
	out, err := s.describeJSON()
	if err != nil {
		fmt.Fprintf(os.Stderr, "describe: %v\n", err)
		return 1
	}
	fmt.Println(string(out))
	return 0
}

func invoke(fn func() error, name string) int {
	if fn == nil {
		// Absent is not an error: the supervisor may send a verb this
		// agent does not implement, and capabilities already told it
		// which ones exist.
		return 0
	}
	if err := fn(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		return 1
	}
	return 0
}

// fire runs one timer firing: Initialize, Start, return. It must NOT
// block after Start returns - the scheduler waits for this process to
// exit before the next firing, so a service-style loop here would hold
// the timer inside a single run forever.
func fire(s *Spec) int {
	if code := invoke(s.Initialize, "initialize"); code != 0 {
		return code
	}
	if err := s.Start(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "start: %v\n", err)
		return 1
	}
	return 0
}

// supervised is the --start path. The control sink is a parameter rather
// than constructed here so a test can read the event sequence: the
// PENDING-to-RUNNING transition is the property this whole package exists
// to make possible, and a test that cannot observe it would be asserting
// only that the process did not crash.
func supervised(s *Spec, ctl *control) int {
	// Everything the AUTHOR writes to stdout from here on goes to stderr;
	// see fenceStdout for why the control channel must stay clean.
	restore, err := fenceStdout()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fence stdout: %v\n", err)
		return 1
	}
	defer restore()

	ctl.emit("starting", nil)

	if s.Initialize != nil {
		if err := s.Initialize(); err != nil {
			ctl.emit("error", map[string]any{"error": fmt.Sprintf("initialize: %v", err)})
			return 1
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigs)

	done := make(chan error, 1)
	go func() { done <- s.Start(ctx) }()

	// READY is reported once Start has been given the chance to fail
	// fast, not the moment the goroutine is spawned.
	//
	// The supervisor treats "ready" as the transition to RUNNING, and
	// nothing else in the system emits it - which is why a Go service
	// agent could never leave PENDING before this existed
	// (GAPI-DIV-051). Reporting it immediately would mean an agent whose
	// Start returns an error instantly still announced itself healthy.
	select {
	case err := <-done:
		return finish(ctl, s, err)
	case <-time.After(readyGrace):
		ctl.emit("ready", nil)
	}

	select {
	case err := <-done:
		return finish(ctl, s, err)
	case <-sigs:
		ctl.emit("stopping", nil)
		cancel()

		if s.Stop != nil {
			if err := s.Stop(); err != nil {
				ctl.emit("error", map[string]any{"error": fmt.Sprintf("stop: %v", err)})
				return 1
			}
		}

		// Give Start's goroutine the chance to unwind on the cancelled
		// context. A bounded wait rather than an unbounded one: the
		// supervisor has its own kill timeout, and hanging here would
		// spend it without ever reporting stopped.
		select {
		case <-done:
		case <-time.After(stopGrace):
		}
		ctl.emit("stopped", nil)
		return 0
	}
}

func finish(ctl *control, s *Spec, err error) int {
	if err != nil {
		ctl.emit("error", map[string]any{"error": err.Error()})
		return 1
	}
	// Start returned cleanly on its own: for a service that is an
	// orderly exit, so report stopped rather than leaving the supervisor
	// to infer it from the process disappearing.
	if s.Stop != nil {
		if serr := s.Stop(); serr != nil {
			ctl.emit("error", map[string]any{"error": fmt.Sprintf("stop: %v", serr)})
			return 1
		}
	}
	ctl.emit("stopped", nil)
	return 0
}

const (
	// readyGrace is how long Start is given to fail before the agent is
	// announced ready.
	readyGrace = 250 * time.Millisecond

	// stopGrace bounds the wait for Start to unwind after cancellation.
	stopGrace = 5 * time.Second
)

// control writes lifecycle events to the real stdout, which is the
// channel the supervisor scans.
type control struct {
	mu  sync.Mutex
	out io.Writer
}

func newControl() *control { return &control{out: controlOut()} }

// newControlTo sinks events into w. Tests only.
func newControlTo(w io.Writer) *control { return &control{out: w} }

func (c *control) emit(event string, kv map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	m := map[string]any{"event": event, "ts": float64(time.Now().UnixNano()) / 1e9}
	if id := os.Getenv("ADK_RUN_ID"); id != "" {
		m["run_id"] = id
	}
	for k, v := range kv {
		m[k] = v
	}

	// An unmarshalable payload must not take the agent down; the
	// supervisor treats a missing event as "no transition", which is
	// recoverable, where a panic here is not.
	line, err := json.Marshal(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "adk/agent: dropping %s event: %v\n", event, err)
		return
	}
	// A failed control write is not recoverable here - the supervisor
	// simply does not see the transition - but it must not be silent,
	// or an agent stuck in PENDING looks like an agent that never
	// reached ready rather than one whose report was lost.
	if _, werr := fmt.Fprintln(c.out, string(line)); werr != nil {
		fmt.Fprintf(os.Stderr, "adk/agent: control write failed for %s: %v\n", event, werr)
	}
}

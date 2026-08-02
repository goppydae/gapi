// Package agent is the Go Agent Development Kit runtime.
//
// A Go agent is a single file, not a program: it declares metadata and
// lifecycle functions and nothing else - no main, no flag parsing, no
// hand-written --describe JSON, no signal handling. This package supplies
// all of that, which is what makes the two languages equivalent rather
// than merely similar. See design/gapi-architecture.md, "Go ADK".
//
// THIS PACKAGE MUST NEVER MOVE UP INTO adk/go. That is a hard constraint
// rather than a preference: gopy binds EVERY exported symbol in adk/go
// into the Python ADK's native extension module (Magefile.go's gopy gen
// target), so a Run or Register placed there would leak the Go agent
// runtime into Python's C bindings. adk/go is the Python ADK's gopy
// backend; this is a separate package gopy never sees.
package agent

import (
	"context"
	"fmt"
	"sync"
)

// Spec is the whole of what an agent declares. The generated main built
// by 'gapictl agent build' fills it from the author's package-level
// consts, vars and functions; nothing here is written by hand.
//
// The wire spellings are the Python ADK's, because the supervisor's
// discovery reads one schema for both languages (core/agentmgr's
// pyDescribe). The Go-side field names are idiomatic Go.
type Spec struct {
	ID          string
	Name        string
	Type        string
	Version     string
	Description string

	// Enabled is a pointer so ABSENT is distinguishable from an explicit
	// false. Discovery resolves absent to enabled, and a plain bool here
	// would render every undeclared agent as disabled the moment the
	// field started being emitted.
	Enabled *bool

	Requires   []string
	Wants      []string
	WantedBy   []string
	RequiredBy []string

	Schedule     string
	ListenStream string
	CPULimit     string
	MemoryLimit  string

	// Capabilities are names this agent advertises BEYOND its lifecycle
	// functions. The Python ADK collects these from @capability
	// decorators; Go declares them as a slice. Two idiomatic surfaces,
	// one wire field - which is what the parity contract asks for, as
	// opposed to two languages that merely resemble each other.
	//
	// Without this a Go agent could advertise nothing but initialize,
	// start, stop, reload and restart, so any capability a Python agent
	// could express was unreachable from Go.
	Capabilities []string

	// Lifecycle. Start is required; the rest are optional and nil when
	// the author did not declare them.
	//
	// Start always takes a context here even though an author may write
	// 'func Start() error'. The generated main adapts the no-context
	// form, so this runtime has ONE shape to reason about rather than
	// two - the adaptation happens once, at generation, instead of at
	// every call site.
	Start      func(ctx context.Context) error
	Initialize func() error
	Stop       func() error
	Reload     func() error
	Restart    func() error
}

var (
	registryMu sync.Mutex
	registered *Spec
)

// Register records the agent this binary serves. The generated main calls
// it from func init(), so it runs before Run reads it.
//
// A second call panics rather than overwriting. One binary is one agent:
// silently keeping the last registration would make a duplicated init the
// kind of defect that only shows up as the wrong agent being supervised.
func Register(s Spec) {
	registryMu.Lock()
	defer registryMu.Unlock()

	if registered != nil {
		panic(fmt.Sprintf("adk/agent: Register called twice (already registered %q, now %q); "+
			"one binary serves exactly one agent", registered.ID, s.ID))
	}
	if s.ID == "" {
		panic("adk/agent: Register called with an empty ID")
	}
	if s.Start == nil {
		panic(fmt.Sprintf("adk/agent: agent %q declares no Start function; "+
			"Start is required for every agent type", s.ID))
	}

	registered = &s
}

// registeredSpec returns the registered agent, or an error naming the
// omission. Run turns that into a non-zero exit rather than a panic,
// because a supervised process that dies with a stack trace is harder to
// read in an agent log than one line saying what was missing.
func registeredSpec() (*Spec, error) {
	registryMu.Lock()
	defer registryMu.Unlock()

	if registered == nil {
		return nil, fmt.Errorf("no agent registered: the generated main must call " +
			"adk.Register before adk.Run (rebuild with 'gapictl agent build')")
	}
	return registered, nil
}

// resetForTest clears the registry. Tests only: Register's
// register-once rule is the property under test in several of them.
func resetForTest() {
	registryMu.Lock()
	defer registryMu.Unlock()
	registered = nil
}

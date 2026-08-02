package agent

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
)

// ErrNoInheritedListener is returned by Listener when the process was not
// socket-activated. It is a typed error on purpose: an agent that wants
// to run standalone during development needs to branch on this, and
// string matching on the message is not a contract.
var ErrNoInheritedListener = errors.New("adk/agent: no inherited listener")

// listenFDsBase is the first inherited descriptor, per the systemd
// convention the supervisor follows (core/agentmgr's go_agent.go and
// python_agent.go both pass ExtraFiles and set LISTEN_FDS to the COUNT,
// plus LISTEN_PID=self).
//
// LISTEN_FDS is a count, not a descriptor number. 'fd + 3' is correct
// only for the accidental single-socket case, which is exactly why
// docs/features.md warns about it and why this arithmetic lives here once
// instead of in every agent.
const listenFDsBase = 3

// Listener returns the listener the supervisor bound and passed down.
//
// An agent must NEVER bind its own address. The supervisor has already
// bound it, so a fresh bind either fails with EADDRINUSE or - worse -
// races the supervisor and wins, leaving the socket the supervisor is
// queueing connections on different from the socket the agent is
// serving. Connections that arrived before the agent was ready are then
// silently stranded.
//
// That failure is invisible to any test which connects AFTER the agent is
// up, which is why this is the ADK's job rather than a documented example
// for authors to copy (GAPI-DIV-055).
func Listener() (net.Listener, error) {
	return listenerAt(0)
}

// ListenerAt returns the i'th inherited listener, for an agent passed
// more than one socket. Listener is ListenerAt(0).
func ListenerAt(i int) (net.Listener, error) {
	return listenerAt(i)
}

func listenerAt(i int) (net.Listener, error) {
	n, err := listenFDs()
	if err != nil {
		return nil, err
	}
	if i < 0 || i >= n {
		return nil, fmt.Errorf("adk/agent: listener %d requested, LISTEN_FDS is %d", i, n)
	}

	fd := uintptr(listenFDsBase + i)
	f := os.NewFile(fd, fmt.Sprintf("listen-fd-%d", fd))
	if f == nil {
		return nil, fmt.Errorf("adk/agent: fd %d is not a valid file", fd)
	}

	// FileListener dups the descriptor, so the original stays owned by
	// this process and closing the returned listener does not close a
	// descriptor the runtime may still be holding.
	l, err := net.FileListener(f)
	closeErr := f.Close()
	if err != nil {
		return nil, fmt.Errorf("adk/agent: fd %d is not a listening socket: %w", fd, err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("adk/agent: closing fd %d after dup: %w", fd, closeErr)
	}
	return l, nil
}

// listenFDs reads and validates the activation environment.
//
// LISTEN_PID is checked against this process. Without that check an agent
// re-execing a child would hand it an environment claiming descriptors
// the child does not have, and the child would then serve whatever fd 3
// happens to be - which is the shape of bug that reads as a mysterious
// protocol error rather than as a configuration mistake.
func listenFDs() (int, error) {
	raw, ok := os.LookupEnv("LISTEN_FDS")
	if !ok {
		return 0, fmt.Errorf("%w: LISTEN_FDS is unset, so this process was not "+
			"socket-activated. A socket agent is started by the supervisor, which "+
			"binds LISTEN_STREAM and passes the descriptor down", ErrNoInheritedListener)
	}

	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("adk/agent: LISTEN_FDS=%q is not a number: %w", raw, err)
	}
	if n < 1 {
		return 0, fmt.Errorf("%w: LISTEN_FDS=%d, so no descriptors were passed",
			ErrNoInheritedListener, n)
	}

	// LISTEN_PID=self is what the supervisor writes; exec resolves it for
	// the child, so both spellings are accepted here.
	if pid, ok := os.LookupEnv("LISTEN_PID"); ok && pid != "self" {
		want, err := strconv.Atoi(pid)
		if err != nil {
			return 0, fmt.Errorf("adk/agent: LISTEN_PID=%q is neither a pid nor \"self\": %w", pid, err)
		}
		if want != os.Getpid() {
			return 0, fmt.Errorf("%w: LISTEN_PID=%d but this process is %d, so the "+
				"descriptors belong to another process", ErrNoInheritedListener, want, os.Getpid())
		}
	}

	return n, nil
}

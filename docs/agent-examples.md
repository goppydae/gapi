# Agent Examples

Worked examples for each agent type. Every metadata block here is a real
module-level assignment: metadata is read with `getattr` on the imported
module, so a commented directive is silently dropped and the agent falls
back to defaults.

## Service agents

A service runs continuously until stopped.

### Basic

```python
# agents/python/services/hello.py.service
ID = "hello"
TYPE = "service"
ENABLED = True

import time


def start():
    print("hello")
    while True:
        time.sleep(60)
```

### Cooperative shutdown

If `start` takes one parameter, the runner passes a `threading.Event`
that is set when the agent is asked to stop. This is the clean way to
exit a loop - it needs no globals and no signal handler.

```python
# agents/python/services/worker.py.service
ID = "worker"
TYPE = "service"


def start(stop_evt):
    print("worker started")
    while not stop_evt.wait(1.0):
        do_work()
    print("worker stopped")


def do_work():
    pass
```

The runner also accepts a separate `stop` hook, under any of the names
`stop`, `shutdown` or `teardown`:

```python
def start():
    ...


def stop():
    print("cleaning up")
```

### Dependencies

```python
# agents/python/services/api.py.service
ID = "api"
TYPE = "service"
REQUIRES = ["database", "cache"]
WANTS = ["telemetry"]


def start():
    print("api starting")
```

`REQUIRES` is a hard dependency: the supervisor skips the start and logs
a missing-dependency warning if it did not come up. `WANTS` is
advisory.

**`DEPENDENCIES` is not an accepted key.** The aliases are `REQUIRES`,
`requires`, `DEPS`, `deps` and `Dependencies`. Spelling it
`DEPENDENCIES` yields an agent with no dependencies at all and therefore
no ordering guarantee - a silent failure, not an error.

## Timer agents

Timers work in either language. `gapictl agent new --type timer`
defaults to Go:

```bash
gapictl agent new backup --type timer
```

```bash
gapictl agent new backup --lang python --type timer
```

Each fire runs the agent to completion and exits - a Python fire spawns
a fresh interpreter, a Go fire executes the binary. A timer body is a
one-shot script, not a loop. **Each fire is bounded at 30 seconds**;
longer work is killed mid-run, and stopping the agent cancels a fire
already in flight.

### Intervals

```python
# agents/python/timers/backup.py.timer
ID = "backup"
TYPE = "timer"
SCHEDULE = "OnUnitActiveSec=5m"


def start():
    print("running backup")
```

The first fire happens one interval after start, not immediately.

`OnBootSec` and `OnStartupSec` are **one-shots**, and differ only in
their anchor - the system's boot, or the moment the timer started:

```python
SCHEDULE = "OnBootSec=30s"      # once, 30s after boot
```

```python
SCHEDULE = "OnStartupSec=10s"   # once, 10s after this timer started
```

A missed elapse point still fires, once, immediately.

### Cron

```python
# agents/python/timers/daily-report.py.timer
ID = "daily-report"
TYPE = "timer"
SCHEDULE = "0 9 * * 1-5"


def start():
    print("generating report")
```

Five fields, standard syntax. Descriptors work too:

```python
SCHEDULE = "@daily"
```

`@hourly`, `@daily`, `@weekly` and `@monthly` are the supported ones.

### Raw duration

```python
# agents/python/timers/heartbeat.py.timer
ID = "heartbeat"
TYPE = "timer"
SCHEDULE = "30s"


def start():
    print("heartbeat")
```

Parsed by Go's `time.ParseDuration`: `30s`, `1m30s` and `24h` work,
`1d` does not.

### A real one

```python
# agents/python/timers/db-backup.py.timer
ID = "db-backup"
TYPE = "timer"
SCHEDULE = "0 2 * * *"
REQUIRES = ["database"]

import datetime
import subprocess


def start():
    stamp = datetime.datetime.now().strftime("%Y%m%d_%H%M%S")
    out = f"/backups/db_{stamp}.sql"
    subprocess.run(
        ["pg_dump", "-h", "localhost", "-U", "postgres", "-f", out, "mydb"],
        check=True,
        timeout=25,
    )
    print(f"backup created: {out}")
```

The inner `timeout=25` is deliberate: it leaves the agent room to report
a failure before the supervisor's own 30-second bound kills the process
without one.

## Socket-activated agents

An agent that declares `LISTEN_STREAM` is **armed**, not started: the
supervisor binds and holds the socket, and starts the agent on the first
connection. The trigger is the declared listen address, not
`TYPE = "socket"`.

Descriptors arrive as `ExtraFiles` starting at **file descriptor 3**.
`LISTEN_FDS` is a *count*, not a descriptor number - `fd + 3` is wrong
for every value except by accident.

### TCP echo

```python
# agents/python/sockets/echo.py.socket
ID = "echo"
TYPE = "socket"
LISTEN_STREAM = "0.0.0.0:8080"

import os
import socket


def start():
    if int(os.environ.get("LISTEN_FDS", "0")) < 1:
        raise RuntimeError("not socket-activated")

    sock = socket.socket(fileno=3)
    while True:
        conn, addr = sock.accept()
        with conn:
            conn.sendall(conn.recv(1024))
```

### UNIX socket

A `LISTEN_STREAM` beginning with `/` or `@` binds a UNIX socket instead:

```python
LISTEN_STREAM = "/run/myagent.sock"
```

A bare port is expanded to `:<port>` and bound as TCP.

**There is no UDP socket activation.** The bind goes through
`net.Listen`, which yields TCP and UNIX sockets only, and there is no
`LISTEN_DATAGRAM` metadata key. An agent needing UDP must bind its own
socket in `start()`.

### HTTP

Wrap the inherited socket. Do **not** construct `HTTPServer` with a bind
address - that binds a second socket on a port the supervisor already
holds, and fails with `EADDRINUSE`.

```python
# agents/python/sockets/web.py.socket
ID = "web"
TYPE = "socket"
LISTEN_STREAM = "0.0.0.0:8000"

import os
import socket
from http.server import BaseHTTPRequestHandler, HTTPServer


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Type", "text/plain")
        self.end_headers()
        self.wfile.write(b"hello from gapi")


class InheritedServer(HTTPServer):
    def __init__(self, sock, handler):
        # Skip HTTPServer's bind and listen entirely; the supervisor
        # already did both and owns the socket.
        super().__init__(("", 0), handler, bind_and_activate=False)
        self.socket.close()
        self.socket = sock
        self.server_address = sock.getsockname()


def start():
    if int(os.environ.get("LISTEN_FDS", "0")) < 1:
        raise RuntimeError("not socket-activated")
    sock = socket.socket(fileno=3)
    InheritedServer(sock, Handler).serve_forever()
```

## Resource limits

Limits are **strings**. `CPU_LIMIT = 0.5` is a float where a string is
expected, and `MEMORY_LIMIT = 100MB` is a syntax error.

```python
# agents/python/services/batch.py.service
ID = "batch"
TYPE = "service"
CPU_LIMIT = "2.0"
MEMORY_LIMIT = "2GB"


def start():
    process_batches()
```

`CPU_LIMIT` is a fraction of one core; millicores also parse, so
`"500m"` equals `"0.5"`. `MEMORY_LIMIT` accepts `K`/`KB`, `M`/`MB`,
`G`/`GB` in base 1024, or a bare number for bytes. Exceeding it means an
OOM kill.

Without cgroup delegation the supervisor logs the failure and runs the
agent **unconstrained** rather than refusing to start it. See
[features.md](features.md) for the delegation setup.

## Go agents

A Go agent is any executable that answers `--describe` with a JSON
metadata block. `gapictl agent new <name>` scaffolds one:

```go
// agents/go/foundational/my_service/main.go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

var describe = flag.Bool("describe", false, "Print agent metadata")

func main() {
	flag.Parse()

	if *describe {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"describe": map[string]any{
				"id":           "my_service",
				"type":         "service",
				"version":      "1.0.0",
				"language":     "go",
				"capabilities": []string{"start", "stop"},
			},
		})
		return
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)

	fmt.Println("my_service running")
	<-sig
	fmt.Println("my_service stopping")
}
```

Go agents do not emit an `enabled` key, which counts as enabled - only
an explicit `false` disables auto-start.

```bash
gapictl agent build
```

That produces the binary plus its `.b3` digest. Sign it before running
under `productionMode`:

```bash
gapictl crypto sign agents/go/foundational/my_service/my_service --key signing-key.pem
```

## Events

Agents do **not** publish to the event bus directly. There is no
`gapi.eventbus` module - the Python package exports the protocol types,
the metadata schemas and the `capability` decorator, and nothing else:

```python
from gapi import Agent, AgentMetadata, capability
```

The runner emits lifecycle events on the agent's behalf. Anything an
agent prints goes to the supervisor's stdout, which is the supported
channel for agent-authored output today.

## Disabling an agent

```python
ENABLED = False
```

The agent is still discovered and registered, still appears in
`gapictl agent status`, and is simply not auto-started - the systemd
model. Start it explicitly when you want it:

```bash
gapictl lifecycle start my-agent
```

#!/usr/bin/env bash
#
# Timer agent end-to-end check.
#
# Asserts a FIRE COUNT within a bounded window, not the presence of a log
# substring. The previous version accepted "fired at least once", which is
# what a timer that fires once and then blocks forever looks like -
# exactly the defect this test failed to catch (GAPI-DIV-039).
set -euo pipefail

cd "$(dirname "$0")/.."
ROOT="$(pwd)"

INTERVAL=2          # the fixture's schedule, in seconds
WINDOW=11           # observation window
EXPECT_MIN=4        # floor(WINDOW / INTERVAL) - 1, allowing one slipped fire

WORK="$(mktemp -d)"
trap 'kill "${GAPID_PID:-}" 2>/dev/null || true; rm -rf "$WORK"' EXIT

echo "[TEST] Testing timer agents..."

# The dev shell owns the toolchain. Hardcoding /nix/store paths here pinned
# a Go version that the flake had since moved past, which is precisely what
# 'mage doctor' exists to prevent.
command -v python3 >/dev/null || { echo "[FAIL] python3 not on PATH; run inside 'nix develop'"; exit 1; }
[ -x ./bin/gapid ] || { echo "[FAIL] ./bin/gapid missing; run 'mage build' first"; exit 1; }

# A fixture directory this test owns, so the result does not depend on
# whatever else happens to be under agents/.
mkdir -p "$WORK/agents"
cat > "$WORK/agents/ticktest.py.timer" <<EOF
ID = "ticktest"
ENABLED = True
TYPE = "timer"
SCHEDULE = "OnUnitActiveSec=${INTERVAL}s"


def start():
    print("TIMER_FIRED_PY")
EOF

# A Go timer, to hold the dual-ADK parity claim. Discovery used to route
# TYPE=timer to the scheduler only for Python paths, so this agent ran
# once at discovery and never again (GAPI-DIV-037).
mkdir -p "$WORK/gosrc"
cat > "$WORK/gosrc/go.mod" <<'EOF'
module gapitest/gotimer

go 1.26.0
EOF
cat > "$WORK/gosrc/main.go" <<EOF
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func main() {
	describe := flag.Bool("describe", false, "print metadata")
	flag.Parse()

	if *describe {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"describe": map[string]any{
				"id":           "gotick",
				"type":         "timer",
				"version":      "1.0.0",
				"language":     "go",
				"schedule":     "OnUnitActiveSec=${INTERVAL}s",
				"capabilities": []string{"start"},
			},
		})
		return
	}

	fmt.Println("TIMER_FIRED_GO")
}
EOF
( cd "$WORK/gosrc" && GOWORK=off go build -o "$WORK/agents/gotick" . ) \
  || { echo "[FAIL] could not build the Go timer fixture"; exit 1; }

# A one-shot. OnStartupSec used to be an alias for OnUnitActiveSec - the
# prefix was stripped and the duration became a repeating interval, so
# this would have fired every 3 seconds forever (GAPI-DIV-036).
cat > "$WORK/agents/oneshot.py.timer" <<'EOF'
ID = "oneshot"
ENABLED = True
TYPE = "timer"
SCHEDULE = "OnStartupSec=3s"


def start():
    print("TIMER_FIRED_ONCE")
EOF

PORT=$((10000 + RANDOM % 10000))
cat > "$WORK/config.yaml" <<EOF
transport:
  type: quic
  address: 127.0.0.1:$PORT
EOF

# tlsCert/tlsKey are the names the loader reads; certFile/keyFile are
# dropped silently by viper and configure nothing.
export RUNTIME_CONFIG="$WORK/config.yaml"
export RUNTIME_AGENT_PATH="$WORK/agents"
export RUNTIME_SKIP_SYSTEM_AGENTS=1
export RUNTIME_PY_RUNNER="$ROOT/adk/python/agent/runner.py"

echo "[TEST] Starting gapid on 127.0.0.1:$PORT..."
./bin/gapid > "$WORK/gapid.log" 2>&1 &
GAPID_PID=$!

echo "[TEST] Observing for ${WINDOW}s (schedule ${INTERVAL}s, expecting at least $EXPECT_MIN fires)..."
sleep "$WINDOW"

kill "$GAPID_PID" 2>/dev/null || true
wait "$GAPID_PID" 2>/dev/null || true
GAPID_PID=""

fail() { echo "[FAIL] $1"; echo "--- gapid.log ---"; cat "$WORK/gapid.log"; exit 1; }

grep -q "timer agent started" "$WORK/gapid.log" || fail "timer agent never started"
echo "[OK] Timer agent started"

TRIGGERS=$(grep -c "timer triggered" "$WORK/gapid.log" || true)
COMPLETED=$(grep -c "timer execution completed" "$WORK/gapid.log" || true)
FIRED_PY=$(grep -c "TIMER_FIRED_PY" "$WORK/gapid.log" || true)
FIRED_GO=$(grep -c "TIMER_FIRED_GO" "$WORK/gapid.log" || true)

echo "[TEST] triggered=$TRIGGERS completed=$COMPLETED python=$FIRED_PY go=$FIRED_GO"

# Two timers, so the trigger floor doubles.
[ "$TRIGGERS" -ge "$((EXPECT_MIN * 2))" ] \
  || fail "expected at least $((EXPECT_MIN * 2)) triggers in ${WINDOW}s across two timers, got $TRIGGERS"
echo "[OK] Timers fired $TRIGGERS times"

# Every fire must terminate. A trigger without a matching completion is a
# fire that blocked until the execution deadline.
[ "$COMPLETED" -eq "$TRIGGERS" ] \
  || fail "$TRIGGERS triggers but only $COMPLETED completed; a fire is not terminating"
echo "[OK] Every fire completed"

[ "$FIRED_PY" -ge "$EXPECT_MIN" ] \
  || fail "Python timer body ran $FIRED_PY times, expected at least $EXPECT_MIN"
echo "[OK] Python timer body ran $FIRED_PY times"

# Parity: a Go timer must be scheduled, not run once at discovery.
[ "$FIRED_GO" -ge "$EXPECT_MIN" ] \
  || fail "Go timer body ran $FIRED_GO times, expected at least $EXPECT_MIN (a Go timer must be scheduled, not run once)"
echo "[OK] Go timer body ran $FIRED_GO times"

# OnStartupSec is a one-shot: EXACTLY one fire in a window three times its
# duration. An alias for OnUnitActiveSec would have fired three times.
FIRED_ONCE=$(grep -c "TIMER_FIRED_ONCE" "$WORK/gapid.log" || true)
[ "$FIRED_ONCE" -eq 1 ] \
  || fail "OnStartupSec=3s fired $FIRED_ONCE times in ${WINDOW}s, want exactly 1"
echo "[OK] OnStartupSec fired exactly once"

grep -q "timer schedule exhausted" "$WORK/gapid.log" \
  || fail "a one-shot schedule did not report itself exhausted; the run loop is still cycling"
echo "[OK] One-shot schedule reported exhausted"

echo "[TEST] Timer agents test PASSED"

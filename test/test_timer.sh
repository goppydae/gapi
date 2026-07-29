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
    print("TIMER_FIRED")
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
FIRED=$(grep -c "TIMER_FIRED" "$WORK/gapid.log" || true)

echo "[TEST] triggered=$TRIGGERS completed=$COMPLETED agent-output=$FIRED"

[ "$TRIGGERS" -ge "$EXPECT_MIN" ] \
  || fail "expected at least $EXPECT_MIN triggers in ${WINDOW}s, got $TRIGGERS"
echo "[OK] Timer fired $TRIGGERS times"

# Every fire must terminate. A trigger without a matching completion is a
# fire that blocked until the execution deadline.
[ "$COMPLETED" -eq "$TRIGGERS" ] \
  || fail "$TRIGGERS triggers but only $COMPLETED completed; a fire is not terminating"
echo "[OK] Every fire completed"

[ "$FIRED" -ge "$EXPECT_MIN" ] \
  || fail "agent body ran $FIRED times, expected at least $EXPECT_MIN"
echo "[OK] Agent body ran $FIRED times"

echo "[TEST] Timer agents test PASSED"

#!/usr/bin/env bash
#
# Agent provenance end-to-end check.
#
# Production mode must start a signed agent binary and refuse an unsigned
# one. This is the end-to-end regression test for GAPI-DIV-032, where the
# CLI signed the canonical digest and the verifier checked the sidecar's
# raw bytes, so no signature the shipped CLI produced could ever verify.
#
# It has to use BINARY agents. Verification lives in safeToExecute, which
# is only reached for executables; a Python agent is described by running
# the interpreter and never touches the signature path, so the previous
# version of this test signed a .py.service and asserted a property the
# code cannot enforce (GAPI-DIV-039).
set -euo pipefail

cd "$(dirname "$0")/.."
ROOT="$(pwd)"

WORK="$(mktemp -d)"
trap 'kill "${GAPID_PID:-}" 2>/dev/null || true; rm -rf "$WORK"' EXIT

echo "[TEST] Testing agent provenance..."

# The dev shell owns the toolchain; hardcoding /nix/store paths here
# pinned a Go the flake had since moved past.
command -v go >/dev/null || { echo "[FAIL] go not on PATH; run inside 'nix develop'"; exit 1; }
[ -x ./bin/gapid ] && [ -x ./bin/gapictl ] \
  || { echo "[FAIL] binaries missing; run 'mage build' first"; exit 1; }

echo "[TEST] Generating signing key..."
# keygen writes <out>.pem (private) and <out>.pub.hex (public).
./bin/gapictl crypto keygen --out "$WORK/testkey" >/dev/null
[ -f "$WORK/testkey.pem" ] && [ -f "$WORK/testkey.pub.hex" ] \
  || { echo "[FAIL] keygen did not produce testkey.pem and testkey.pub.hex"; exit 1; }

echo "[TEST] Building two binary agents..."
mkdir -p "$WORK/src" "$WORK/agents"
cat > "$WORK/src/go.mod" <<'EOF'
module gapitest/probe

go 1.26.0
EOF
cat > "$WORK/src/main.go" <<'EOF'
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

func main() {
	describe := flag.Bool("describe", false, "print metadata")
	flag.Parse()

	// The id is the binary's own name, so one source builds both agents.
	id := filepath.Base(os.Args[0])

	if *describe {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"describe": map[string]any{
				"id":           id,
				"type":         "service",
				"version":      "1.0.0",
				"language":     "go",
				"capabilities": []string{"start", "stop"},
			},
		})
		return
	}

	fmt.Println(id, "running")
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT)
	<-c
}
EOF

for agent in signed_agent unsigned_agent; do
	# GOWORK=off: the fixture is its own module and must not be pulled
	# into the repo's workspace.
	( cd "$WORK/src" && GOWORK=off go build -o "$WORK/agents/$agent" . ) \
	  || { echo "[FAIL] could not build $agent"; exit 1; }
done

echo "[TEST] Signing one of them..."
# crypto sign writes both sidecars: the .b3 digest and the .sig over it.
# VerifySignedBinary requires both.
./bin/gapictl crypto sign "$WORK/agents/signed_agent" --key "$WORK/testkey.pem" >/dev/null
[ -f "$WORK/agents/signed_agent.b3" ] || { echo "[FAIL] sign produced no .b3"; exit 1; }
[ -f "$WORK/agents/signed_agent.sig" ] || { echo "[FAIL] sign produced no .sig"; exit 1; }

echo "[TEST] Creating config (production mode on)..."
PORT=$((10000 + RANDOM % 10000))
cat > "$WORK/config.yaml" <<EOF
transport:
  type: quic
  address: 127.0.0.1:$PORT
security:
  verifyKey: $WORK/testkey.pub.hex
supervisor:
  productionMode: true
logging:
  level: debug
EOF

# These could equally be exported as GAPI_ variables now that every
# key binds; the file keeps the test's intent readable in one place.
export GAPI_CONFIG="$WORK/config.yaml"
export GAPI_AGENT_PATH="$WORK/agents"
export GAPI_SKIP_SYSTEM_AGENTS=1
export GAPI_PY_RUNNER="$ROOT/adk/python/agent/runner.py"

echo "[TEST] Starting gapid on 127.0.0.1:$PORT..."
./bin/gapid > "$WORK/gapid.log" 2>&1 &
GAPID_PID=$!
sleep 5
kill "$GAPID_PID" 2>/dev/null || true
wait "$GAPID_PID" 2>/dev/null || true
GAPID_PID=""

fail() { echo "[FAIL] $1"; echo "--- gapid.log ---"; cat "$WORK/gapid.log"; exit 1; }

grep -q "integrity verification enabled" "$WORK/gapid.log" \
  || fail "verification key was not loaded"
echo "[OK] Verification key loaded"

grep -q "refusing to execute agent binary" "$WORK/gapid.log" \
  || fail "unsigned agent was NOT rejected; production mode is not enforcing"
echo "[OK] Unsigned agent rejected"

grep -q '"agent_id":"signed_agent"' "$WORK/gapid.log" \
  || fail "signed agent did not register; its signature failed to verify"
echo "[OK] Signed agent registered"

if grep -q '"agent_id":"unsigned_agent"' "$WORK/gapid.log"; then
	fail "unsigned agent registered anyway"
fi
echo "[OK] Unsigned agent did not register"

echo "[TEST] Agent provenance test PASSED"

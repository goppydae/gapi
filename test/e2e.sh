#!/usr/bin/env bash
# Copyright (c) 2025 Steven Verhelle (enqack)
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.
#
# SPDX-License-Identifier: MPL-2.0

set -e

# Setup colors
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

log() {
    echo -e "${GREEN}[TEST]${NC} $1"
}

fail() {
    echo -e "${RED}[FAIL]${NC} $1"
    exit 1
}

cleanup() {
    log "Cleaning up..."
    if [ -n "$GAPID_PID" ]; then
        kill $GAPID_PID 2>/dev/null || true
    fi
    # Both are unset if the script dies before mktemp runs, and a bare
    # 'rm -rf ""' fails - which under set -e would turn a passing run
    # into a nonzero exit from the trap.
    rm -rf "${TEST_AGENTS_DIR:-}" "${WORK:-}" 2>/dev/null || true
}

trap cleanup EXIT

# 0. Build
# 0. Build
# (Handled by Magefile dependency)
# log "Building project..."
# CGO_ENABLED=0 go run github.com/magefile/mage@latest buildall

# 1. Setup Test Environment
pkill -f bin/gapid || true
WORK=$(mktemp -d)
TEST_AGENTS_DIR=$(mktemp -d)
log "Created temp agents dir: $TEST_AGENTS_DIR"
PORT=$((10000 + RANDOM % 10000))
log "Using port: $PORT"

# Generate Config
#
# No TLS material is configured, and none is generated. This script used
# to write an RSA key and a 24-hour self-signed certificate straight into
# the tracked config/certs directory, then name them here as certFile and
# keyFile - a spelling viper drops silently (the loader reads tlsCert and
# tlsKey), so the daemon never loaded them and fell back to its own
# self-signed certificate regardless. The only durable effect was a fresh
# private key dirtying the working tree on every run, which is how six of
# them reached version control. Leaving the keys unset reaches the same
# transport by the honest route: gapid warns and generates in-memory, and
# transport.insecureSkipVerify defaults to true so the client connects.
cat <<EOF > "$WORK/config.yaml"
transport:
  type: "quic"
  address: "localhost:$PORT"
agents:
  dir: "$TEST_AGENTS_DIR"
EOF
log "Created $WORK/config.yaml"
export GAPI_CONFIG="$WORK/config.yaml"

# 2. Setup Agents
cp agents/python/services/heartbeat.py.service $TEST_AGENTS_DIR/heartbeat.py.service
# Create a dependency
cat <<EOF > $TEST_AGENTS_DIR/base.py.service
ID = "base"
TYPE = "service"
DESCRIPTION = "Base Svc"
def start():
   import time
   while True: time.sleep(1)
EOF
# Verify it creates a file
ls -l $TEST_AGENTS_DIR/base.py.service

# Inject dependency into heartbeat
sed -i 's/DEPS = \[\]/DEPS = ["base"]/g' $TEST_AGENTS_DIR/heartbeat.py.service
# If DEPS line doesn't exist, append it (heartbeat might not have it explicitly)
# Check heartbeat content first... it imports from adk, but metadata?
# heartbeat.py.service usually has metadata constants.
grep -q "DEPS =" $TEST_AGENTS_DIR/heartbeat.py.service || echo 'DEPS = ["base"]' >> $TEST_AGENTS_DIR/heartbeat.py.service

# 2. Start Daemon
log "Starting gapid..."
export GAPI_AGENT_PATH="$TEST_AGENTS_DIR"
# GAPI-DIV-063: AGENT_PATH is additive now, so the fence is explicit.
# Without this the built-in tiers come back underneath and this run
# discovers whatever agents the host has installed.
export GAPI_AGENT_PATH_EXCLUSIVE=1
export ADK_FORCE_DUMMY=1
# GAPI-DIV-057: the daemon root refuses a bare invocation, so the
# subcommand is mandatory - without it gapid prints usage and exits 1.
./bin/gapid start > gapid.log 2>&1 &
GAPID_PID=$!
log "gapid PID: $GAPID_PID"

# Wait for startup (simple sleep for now, could act on 'supervisor running' log)
sleep 2

# 3. Verify Agent Status
log "Starting base agent (dependency)..."
./bin/gapictl lifecycle start base
sleep 2

log "Starting agent..."
./bin/gapictl lifecycle start heartbeat

log "Checking agent status..."
STATUS_OUTPUT=$(./bin/gapictl agent status)
echo "$STATUS_OUTPUT"

if echo "$STATUS_OUTPUT" | grep -q "heartbeat" && echo "$STATUS_OUTPUT" | grep -qE "running|starting"; then
    log "SUCCESS: heartbeat agent found and running/starting"
else
    fail "heartbeat agent not found or not running"
fi

log "Checking dependency tree..."
TREE_OUTPUT=$(./bin/gapictl agent status --tree)
echo "$TREE_OUTPUT"
if echo "$TREE_OUTPUT" | grep -q "heartbeat" && echo "$TREE_OUTPUT" | grep -q "base"; then
    log "SUCCESS: tree view contains agents"
else
    fail "tree view missing content"
fi

# 4. Verify Ping
log "Pinging daemon..."
./bin/gapictl ping

log "E2E Test Passed!"

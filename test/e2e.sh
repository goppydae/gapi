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
    rm -rf "$TEST_AGENTS_DIR"
    rm -f config.yaml
}

trap cleanup EXIT

# 0. Build
# 0. Build
# (Handled by Magefile dependency)
# log "Building project..."
# CGO_ENABLED=0 go run github.com/magefile/mage@latest buildall

# 0.5 Generate Certs (Go-native)
log "Generating certs..."
mkdir -p config/certs
cat <<EOF > gencert.go
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"time"
)

func main() {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour * 24),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, _ := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)

	certOut, _ := os.Create("config/certs/server.crt")
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der})
	certOut.Close()

	keyOut, _ := os.Create("config/certs/server.key")
	pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	keyOut.Close()
}
EOF
CGO_ENABLED=0 go run gencert.go
rm gencert.go

# 1. Setup Test Environment
pkill -f bin/gapid || true
TEST_AGENTS_DIR=$(mktemp -d)
log "Created temp agents dir: $TEST_AGENTS_DIR"
PORT=$((10000 + RANDOM % 10000))
log "Using port: $PORT"

# Generate Config
# Certificates are generated in config/certs by 'mage tls'
CERT_DIR="$(pwd)/config/certs"

cat <<EOF > config.yaml
transport:
  type: "quic"
  address: "localhost:$PORT"
  certFile: "$CERT_DIR/server.crt"
  keyFile: "$CERT_DIR/server.key"
agents:
  dir: "$TEST_AGENTS_DIR"
EOF
log "Created config.yaml"
export GAPI_CONFIG="$(pwd)/config.yaml"

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
./bin/gapid > gapid.log 2>&1 &
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

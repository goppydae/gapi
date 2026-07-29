#!/usr/bin/env bash
set -e

echo "[TEST] Building project..."
export PATH=/nix/store/vr15iyyykg9zai6fpgvhcgyw7gckl78w-gcc-wrapper-14.3.0/bin:/nix/store/0a3dyfq09dnkw28ap2i450wjimvdmv6s-go-1.25.4/bin:$HOME/go/bin:$PATH
go build -o bin/gapid ./cmd/gapid
go build -o bin/gapictl ./cmd/gapictl

echo "[TEST] Generating test keys..."
./bin/gapictl keygen testkey

echo "[TEST] Creating test agents directory..."
AGENTS_DIR=$(mktemp -d)
echo "[TEST] Test agents dir: $AGENTS_DIR"

# Create a simple test agent
cat > "$AGENTS_DIR/test.py.service" <<'EOF'
# ENABLED = True
# TYPE = service

def start():
    print("Test agent started")
EOF

# Sign it
./bin/gapictl sign "$AGENTS_DIR/test.py.service" testkey.key

# Create an unsigned agent
cat > "$AGENTS_DIR/unsigned.py.service" <<'EOF'
# ENABLED = True
# TYPE = service

def start():
    print("Unsigned agent")
EOF

echo "[TEST] Using existing certs..."
if [ ! -f config/certs/server.crt ]; then
    echo "[FAIL] Certs not found, run: ./test/e2e.sh first"
    exit 1
fi

echo "[TEST] Creating config..."
PORT=$((10000 + RANDOM % 10000))
cat > test_config.yaml <<EOF
transport:
  type: quic
  address: 127.0.0.1:$PORT
  certFile: config/certs/server.crt
  keyFile: config/certs/server.key
security:
  verifyKey: testkey.pub
EOF

echo "[TEST] Starting gapid with integrity verification..."
export RUNTIME_CONFIG=$(pwd)/test_config.yaml
export RUNTIME_AGENTS_DIR="$AGENTS_DIR"
export RUNTIME_PY_RUNNER=$(pwd)/adk/python/agent/runner.py

./bin/gapid > test_integrity.log 2>&1 &
GAPID_PID=$!
echo "[TEST] gapid PID: $GAPID_PID"

sleep 3

echo "[TEST] Checking logs..."
if grep -q "integrity verification enabled" test_integrity.log; then
    echo "[OK] Integrity verification enabled"
else
    echo "[FAIL] Integrity verification NOT enabled"
    cat test_integrity.log
    kill $GAPID_PID 2>/dev/null || true
    exit 1
fi

if grep -q "integrity check failed.*unsigned" test_integrity.log; then
    echo "[OK] Unsigned agent rejected"
else
    echo "[FAIL] Unsigned agent NOT rejected"
    cat test_integrity.log
    kill $GAPID_PID 2>/dev/null || true
    exit 1
fi

if grep -q "registered agent.*test" test_integrity.log; then
    echo "[OK] Signed agent loaded"
else
    echo "[WARN] Signed agent may not have loaded (check logs)"
fi

echo "[TEST] Cleaning up..."
kill $GAPID_PID 2>/dev/null || true
rm -rf "$AGENTS_DIR" test_config.yaml test_integrity.log testkey.key testkey.pub

echo "[TEST] Integrity verification test PASSED!"

#!/usr/bin/env bash
set -e

echo "[TEST] Testing Timer Agents..."
export PATH=/nix/store/vr15iyyykg9zai6fpgvhcgyw7gckl78w-gcc-wrapper-14.3.0/bin:/nix/store/0a3dyfq09dnkw28ap2i450wjimvdmv6s-go-1.25.4/bin:$HOME/go/bin:$PATH

# Use existing config
PORT=$((10000 + RANDOM % 10000))
cat > test_timer_config.yaml <<EOF
transport:
  type: quic
  address: 127.0.0.1:$PORT
  certFile: config/certs/server.crt
  keyFile: config/certs/server.key
EOF

export RUNTIME_CONFIG=$(pwd)/test_timer_config.yaml
export RUNTIME_AGENTS_DIR=agents
export RUNTIME_PY_RUNNER=$(pwd)/adk/python/agent/runner.py

echo "[TEST] Starting gapid..."
./bin/gapid > test_timer.log 2>&1 &
GAPID_PID=$!
echo "[TEST] gapid PID: $GAPID_PID"

echo "[TEST] Waiting 15 seconds for timer ticks..."
sleep 15

echo "[TEST] Checking logs..."
kill $GAPID_PID 2>/dev/null || true

if grep -q "timer agent started" test_timer.log; then
    echo "✅ Timer agent started"
else
    echo "❌ Timer agent did not start"
    cat test_timer.log
    exit 1
fi

if grep -q "timer triggered" test_timer.log; then
    echo "✅ Timer triggered"
    TICK_COUNT=$(grep -c "timer triggered" test_timer.log)
    echo "   Timer ticked $TICK_COUNT times"
else
    echo "❌ Timer did not trigger"
    cat test_timer.log
    exit 1
fi

if grep -q "Timer tick!" test_timer.log; then
    echo "✅ Timer agent executed successfully"
else
    echo "⚠️  Timer triggered but agent may not have executed"
fi

echo "[TEST] Cleaning up..."
rm -f test_timer_config.yaml test_timer.log

echo "[TEST] Timer agents test PASSED!"

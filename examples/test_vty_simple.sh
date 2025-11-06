#!/bin/bash
# Simple VTY interaction test

set -e

echo "=== Simple VTY Test ==="

# Cleanup function
cleanup() {
    if [ -n "$BGRUN_PID" ]; then
        kill $BGRUN_PID 2>/dev/null || true
    fi
}
trap cleanup EXIT

# Test 1: Simple cat echo
echo "Test 1: Cat echo test"
echo "---------------------"
./bgrun -vty cat > /tmp/vty_simple1.log 2>&1 &
BGRUN_PID=$!
sleep 1

# Find socket
SOCKET=$(find /run/user/$(id -u) /tmp/.bgrun-$(id -u) -name "control.sock" -newer /tmp/bgrun_marker 2>/dev/null | head -1)
echo "Socket: $SOCKET"

# Test with simple echo
echo "Sending: hello world"
(echo "hello world"; sleep 0.5) | timeout 2 ./bgctl -socket "$SOCKET" attach 2>&1 | grep -i "hello" && echo "✓ Echo works!" || echo "✗ Echo failed"

kill $BGRUN_PID 2>/dev/null || true
wait $BGRUN_PID 2>/dev/null || true
BGRUN_PID=""

echo ""
echo "Test 2: AWK uppercase transform"
echo "--------------------------------"
touch /tmp/bgrun_marker
./bgrun -vty awk '{print toupper($0)}' > /tmp/vty_simple2.log 2>&1 &
BGRUN_PID=$!
sleep 1

SOCKET=$(find /run/user/$(id -u) /tmp/.bgrun-$(id -u) -name "control.sock" -newer /tmp/bgrun_marker 2>/dev/null | head -1)
echo "Socket: $SOCKET"

echo "Sending: test input"
(echo "test input"; sleep 0.5) | timeout 2 ./bgctl -socket "$SOCKET" attach 2>&1 | tee /tmp/vty_awk_output.txt

grep -q "TEST INPUT" /tmp/vty_awk_output.txt && echo "✓ Uppercase transform works!" || echo "✗ Transform failed"

kill $BGRUN_PID 2>/dev/null || true
wait $BGRUN_PID 2>/dev/null || true
BGRUN_PID=""

echo ""
echo "Done!"

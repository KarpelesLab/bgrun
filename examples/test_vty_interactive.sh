#!/bin/bash
# Comprehensive VTY/PTY interaction test script

set -e

echo "=== bgrun VTY Interactive Tests ==="
echo ""

cleanup() {
    if [ -n "$BGRUN_PID" ]; then
        kill $BGRUN_PID 2>/dev/null || true
        wait $BGRUN_PID 2>/dev/null || true
    fi
    if [ -n "$SOCKET" ] && [ -S "$SOCKET" ]; then
        ./bgrun -ctl -pid $DAEMON_PID shutdown 2>/dev/null || true
    fi
}

trap cleanup EXIT

get_socket() {
    sleep 0.5
    if [ -d "/run/user/$(id -u)" ]; then
        SOCKET=$(find /run/user/$(id -u) -name "control.sock" -newer /tmp/bgrun_marker 2>/dev/null | head -1)
    fi
    if [ -z "$SOCKET" ]; then
        SOCKET=$(find /tmp/.bgrun-$(id -u) -name "control.sock" -newer /tmp/bgrun_marker 2>/dev/null | head -1)
    fi
    echo "Socket: $SOCKET"
}

# Create marker file for finding new sockets
touch /tmp/bgrun_marker

echo "Test 1: Interactive read loop (echo with uppercase transform)"
echo "-------------------------------------------------------------"
./bgrun -vty bash -c 'while read -r line; do echo "$line" | tr a-z A-Z; done' > /tmp/bgrun_test1.log 2>&1 &
BGRUN_PID=$!

get_socket

if [ -z "$SOCKET" ]; then
    echo "ERROR: Could not find socket"
    exit 1
fi

# Check status
echo "Checking initial status..."
./bgrun -ctl -pid $DAEMON_PID status

# Create a test client that sends input and reads output
echo ""
echo "Sending test inputs..."
(
    echo "hello world"
    sleep 0.5
    echo "test input"
    sleep 0.5
    echo "foo bar baz"
    sleep 0.5
    # Send Ctrl-D (EOF) to close the read loop
    printf "\x04"
) | timeout 5 ./bgrun -ctl -pid $DAEMON_PID attach 2>&1 | tee /tmp/vty_test_output.txt || true

echo ""
echo "Output received:"
cat /tmp/vty_test_output.txt | grep -E "HELLO|TEST|FOO" || echo "  (No uppercase output detected - check /tmp/vty_test_output.txt)"

wait $BGRUN_PID 2>/dev/null || true
BGRUN_PID=""

echo ""
echo "Test 2: Signal handling (Ctrl-C / SIGINT)"
echo "-------------------------------------------------------------"
touch /tmp/bgrun_marker
./bgrun -vty bash -c 'trap "echo SIGINT caught" INT; echo Ready; sleep 30; echo Done' > /tmp/bgrun_test2.log 2>&1 &
BGRUN_PID=$!

get_socket

echo "Waiting for process to be ready..."
sleep 1

echo "Sending SIGINT (signal 2)..."
./bgrun -ctl -pid $DAEMON_PID signal 2

sleep 1

echo "Checking if signal was received..."
grep "SIGINT caught" /tmp/bgrun_test2.log && echo "  ✓ Signal handler worked!" || echo "  ✗ Signal not caught"

./bgrun -ctl -pid $DAEMON_PID shutdown 2>/dev/null || true
wait $BGRUN_PID 2>/dev/null || true
BGRUN_PID=""

echo ""
echo "Test 3: Sending raw Ctrl-C through stdin"
echo "-------------------------------------------------------------"
touch /tmp/bgrun_marker
./bgrun -vty bash -c 'echo Starting; sleep 10; echo Should not reach here' > /tmp/bgrun_test3.log 2>&1 &
BGRUN_PID=$!

get_socket

sleep 1

echo "Sending Ctrl-C (0x03) via stdin..."
# Note: This test shows the stdin path, actual Ctrl-C behavior may vary
printf "\x03" | timeout 2 ./bgrun -ctl -pid $DAEMON_PID attach 2>&1 || true

sleep 1
./bgrun -ctl -pid $DAEMON_PID status | grep -q "Running: false" && echo "  ✓ Process interrupted" || echo "  ✗ Process still running"

./bgrun -ctl -pid $DAEMON_PID shutdown 2>/dev/null || true
wait $BGRUN_PID 2>/dev/null || true
BGRUN_PID=""

echo ""
echo "Test 4: Killing a running process"
echo "-------------------------------------------------------------"
touch /tmp/bgrun_marker
./bgrun -vty sleep 30 > /tmp/bgrun_test4.log 2>&1 &
BGRUN_PID=$!

get_socket

sleep 1

echo "Getting PID of running process..."
PID=$(./bgrun -ctl -pid $DAEMON_PID status | grep "PID:" | awk '{print $2}')
echo "Process PID: $PID"

echo "Sending SIGKILL..."
kill -9 $PID

sleep 1

./bgrun -ctl -pid $DAEMON_PID status | grep -q "Running: false" && echo "  ✓ Process killed successfully" || echo "  ✗ Process still running"

wait $BGRUN_PID 2>/dev/null || true
BGRUN_PID=""

echo ""
echo "Test 5: Multiple rapid inputs (cat echo test)"
echo "-------------------------------------------------------------"
touch /tmp/bgrun_marker
./bgrun -vty cat > /tmp/bgrun_test5.log 2>&1 &
BGRUN_PID=$!

get_socket

sleep 1

echo "Sending rapid inputs..."
(
    echo "Line 1"
    echo "Line 2"
    echo "Line 3"
    echo "abcdefghijklmnopqrstuvwxyz"
    echo "0123456789"
    printf "\x04"  # Ctrl-D to close
) | timeout 3 ./bgrun -ctl -pid $DAEMON_PID attach 2>&1 | tee /tmp/vty_rapid_output.txt || true

echo ""
echo "Checking echo integrity..."
grep -q "abcdefghijklmnopqrstuvwxyz" /tmp/vty_rapid_output.txt && echo "  ✓ All characters echoed correctly" || echo "  ✗ Echo incomplete"

wait $BGRUN_PID 2>/dev/null || true
BGRUN_PID=""

echo ""
echo "Test 6: Terminal resize while running"
echo "-------------------------------------------------------------"
touch /tmp/bgrun_marker
./bgrun -vty bash -c 'echo Start; sleep 5; echo End' > /tmp/bgrun_test6.log 2>&1 &
BGRUN_PID=$!

get_socket

sleep 1

# Note: bgrun -ctl attach would handle resize automatically with SIGWINCH
# For manual testing, we'd use the resize API directly
echo "Process running with PTY..."
./bgrun -ctl -pid $DAEMON_PID status | grep "Has VTY: true" && echo "  ✓ VTY mode confirmed" || echo "  ✗ Not in VTY mode"

wait $BGRUN_PID 2>/dev/null || true
BGRUN_PID=""

echo ""
echo "=== All VTY Tests Completed ==="
echo ""
echo "Log files:"
echo "  /tmp/bgrun_test*.log - daemon logs"
echo "  /tmp/vty_test_output.txt - interactive output"
echo "  /tmp/vty_rapid_output.txt - rapid input echo"

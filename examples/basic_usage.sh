#!/bin/bash
# Basic usage examples for bgrun

set -e

echo "=== bgrun Basic Usage Examples ==="
echo ""

# Example 1: Run a simple command
echo "1. Running a simple sleep command..."
./bgrun sleep 5 &
BGRUN_PID=$!
sleep 1

# Get the runtime directory
RUNTIME_DIR=$(ls -dt /run/user/$(id -u)/* /tmp/.bgrun-$(id -u)/* 2>/dev/null | head -1)
SOCKET="$RUNTIME_DIR/control.sock"

echo "   Runtime directory: $RUNTIME_DIR"
echo "   Socket: $SOCKET"
echo ""

# Example 2: Check status
echo "2. Checking process status..."
./bgctl -socket "$SOCKET" status
echo ""

# Example 3: Send signal
echo "3. Sending SIGTERM..."
./bgctl -socket "$SOCKET" signal 15
sleep 1
echo ""

# Wait for bgrun to exit
wait $BGRUN_PID 2>/dev/null || true

# Example 4: Run command with output
echo "4. Running command with output logging..."
./bgrun -stdout log -stderr log sh -c 'for i in 1 2 3; do echo "Line $i"; sleep 0.5; done' &
BGRUN_PID=$!
sleep 1

# Get new runtime directory
RUNTIME_DIR=$(ls -dt /run/user/$(id -u)/* /tmp/.bgrun-$(id -u)/* 2>/dev/null | head -1)
SOCKET="$RUNTIME_DIR/control.sock"

echo "   Attaching to output..."
timeout 5 ./bgctl -socket "$SOCKET" attach || true
echo ""

# Wait for bgrun to exit
wait $BGRUN_PID 2>/dev/null || true

# Example 5: Interactive stdin
echo "5. Running command with stdin streaming..."
./bgrun -stdin stream cat &
BGRUN_PID=$!
sleep 1

RUNTIME_DIR=$(ls -dt /run/user/$(id -u)/* /tmp/.bgrun-$(id -u)/* 2>/dev/null | head -1)
SOCKET="$RUNTIME_DIR/control.sock"

# This would require a custom client to send stdin data and close it
# For now, just shutdown
echo "   Sending shutdown..."
./bgctl -socket "$SOCKET" shutdown

# Wait for bgrun to exit
wait $BGRUN_PID 2>/dev/null || true

echo ""
echo "=== Examples completed ==="

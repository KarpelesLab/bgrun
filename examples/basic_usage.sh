#!/bin/bash
# Basic usage examples for bgrun

set -e

echo "=== bgrun Basic Usage Examples ==="
echo ""

# Example 1: Run a simple command
echo "1. Running a simple sleep command..."
DAEMON_PID=$(./bgrun -background sleep 5)
echo "   Daemon PID: $DAEMON_PID"
sleep 1

# Example 2: Check status
echo "2. Checking process status..."
./bgrun -ctl -pid $DAEMON_PID status
echo ""

# Example 3: Send signal
echo "3. Sending SIGTERM..."
./bgrun -ctl -pid $DAEMON_PID signal 15
sleep 1
echo ""

# Example 4: Run command with output
echo "4. Running command with output logging..."
DAEMON_PID=$(./bgrun -background -stdout log -stderr log sh -c 'for i in 1 2 3; do echo "Line $i"; sleep 0.5; done')
echo "   Daemon PID: $DAEMON_PID"
sleep 1

echo "   Attaching to output..."
timeout 5 ./bgrun -ctl -pid $DAEMON_PID attach || true
echo ""

# Example 5: Interactive stdin
echo "5. Running command with stdin streaming..."
DAEMON_PID=$(./bgrun -background -stdin stream cat)
echo "   Daemon PID: $DAEMON_PID"
sleep 1

# This would require a custom client to send stdin data and close it
# For now, just shutdown
echo "   Sending shutdown..."
./bgrun -ctl -pid $DAEMON_PID shutdown

echo ""
echo "=== Examples completed ==="

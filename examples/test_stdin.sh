#!/bin/bash
# Quick test of sending stdin and reading output

DAEMON_PID="$1"

if [ -z "$DAEMON_PID" ]; then
    echo "Usage: $0 <daemon-pid>"
    exit 1
fi

echo "Testing stdin/stdout with daemon PID: $DAEMON_PID"
echo "Type something and press Enter (Ctrl-D to exit):"

# Use bgrun -ctl to attach and interact
timeout 10 ./bgrun -ctl -pid "$DAEMON_PID" attach

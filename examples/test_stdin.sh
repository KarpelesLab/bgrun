#!/bin/bash
# Quick test of sending stdin and reading output

SOCKET="$1"

if [ -z "$SOCKET" ]; then
    echo "Usage: $0 <socket-path>"
    exit 1
fi

echo "Testing stdin/stdout with socket: $SOCKET"
echo "Type something and press Enter (Ctrl-D to exit):"

# Use cat to read stdin and send to bgctl
timeout 10 ./bgctl -socket "$SOCKET" attach

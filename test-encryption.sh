#!/bin/bash

echo "=== Testing UDP Network Encryption ==="
echo

# Build secure chat example
echo "Building secure chat example..."
go build -o secure-chat-server ./examples/secure-chat/
go build -o secure-chat-client ./examples/secure-chat/

# Start server in background
echo "Starting secure server..."
./secure-chat-server -server -addr localhost:9999 &
SERVER_PID=$!
sleep 1

# Test client connection
echo "Testing secure client connection..."
echo -e "Hello encrypted world!\nThis is a secure message.\nquit" | ./secure-chat-client -addr localhost:9999 &
CLIENT_PID=$!

# Wait a bit for messages
sleep 2

# Clean up
kill $SERVER_PID 2>/dev/null
kill $CLIENT_PID 2>/dev/null

echo
echo "Encryption test complete!"
echo "Note: Use 'go run tests/crypto-benchmark/main.go' to test performance impact"
#!/bin/bash

echo "=== Manual Server/Client Test ==="
echo ""
echo "Starting server in background..."

# Start server and capture output
./bin/server > server.log 2>&1 &
SERVER_PID=$!
echo "Server PID: $SERVER_PID"

sleep 2

echo ""
echo "Starting client..."
echo "Type 'Hello from manual test' and then '/quit'"
echo ""

# Send a test message
echo -e "Hello from manual test\n/ping\n/stats\n/quit" | ./bin/client localhost:8080

echo ""
echo "Client finished. Waiting 2 seconds..."
sleep 2

# Kill server
kill $SERVER_PID 2>/dev/null

echo ""
echo "Server log:"
echo "==========="
cat server.log

# Cleanup
rm -f server.log
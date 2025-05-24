#!/bin/bash

echo "Quick UDP Protocol Test"
echo "======================"

# Start server in background
echo "Starting server..."
./bin/test-server &
SERVER_PID=$!
sleep 1

# Test client connection
echo "Testing client connection..."
echo -e "Hello, UDP!\n/ping\n/quit" | timeout 5s ./bin/test-client localhost:8080

# Check exit status
if [ $? -eq 0 ]; then
    echo "✓ Connection successful!"
else
    echo "✗ Connection failed!"
fi

# Cleanup
kill $SERVER_PID 2>/dev/null
wait $SERVER_PID 2>/dev/null

echo "Test complete"
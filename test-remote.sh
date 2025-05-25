#!/bin/bash

# UDP Network Remote Test Script
echo "UDP Network Remote Test Script"
echo "============================="

if [ $# -eq 0 ]; then
    echo "Usage: $0 <host:port> [options]"
    echo ""
    echo "Examples:"
    echo "  $0 192.168.1.100:8080                    # Test regular server"
    echo "  $0 192.168.1.100:8443 --secure           # Test secure server"
    echo "  $0 example.com:8080 --messages 50        # Send 50 test messages"
    echo "  $0 10.0.0.5:8080 --size 1024             # Send 1KB messages"
    echo ""
    echo "Options:"
    echo "  --secure                  Use encrypted connection (default: false)"
    echo "  --messages N              Number of messages to send (default: 10)"
    echo "  --size N                  Size of each message in bytes (default: 256)"
    echo "  --timeout Ns              Connection timeout (default: 30s)"
    echo ""
    echo "Regular server ports: 8080"
    echo "Secure server ports:  8443"
    exit 1
fi

# Parse host:port
HOST_PORT=$1
shift

# Extract host and port
if [[ $HOST_PORT == *":"* ]]; then
    HOST=${HOST_PORT%:*}
    PORT=${HOST_PORT#*:}
else
    echo "Error: Please specify both host and port (e.g., 192.168.1.100:8080)"
    exit 1
fi

echo "Testing connection to $HOST:$PORT"
echo ""

# Run the test
go run tests/remote-test/main.go -host "$HOST" -port "$PORT" "$@"
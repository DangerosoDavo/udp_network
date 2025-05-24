#!/bin/bash

echo "Building UDP Network Protocol..."

# Create build directory
mkdir -p bin

# Build server
echo "Building server..."
go build -o bin/server ./cmd/server/main.go

# Build client  
echo "Building client..."
go build -o bin/client ./cmd/client/main.go

# Build chat example
echo "Building chat example..."
go build -o bin/chat ./examples/chat/main.go

# Build tests
echo "Building test programs..."
go build -o bin/perf-test ./tests/performance/main.go
go build -o bin/stress-test ./tests/stress/main.go

echo "Build complete! Binaries are in ./bin/"
echo ""
echo "Quick start:"
echo "  Terminal 1: ./bin/server"
echo "  Terminal 2: ./bin/client localhost:8080"
echo ""
echo "Or try the chat example:"
echo "  Terminal 1: ./bin/chat -server"
echo "  Terminal 2+: ./bin/chat -addr localhost:8080"
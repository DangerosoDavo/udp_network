#!/bin/bash

echo "UDP Network Protocol Test Suite"
echo "==============================="
echo ""

# Function to run test in background
run_test() {
    echo "Starting $1..."
    $2 &
    PID=$!
    echo "Started with PID $PID"
    return $PID
}

case "${1:-basic}" in
    "basic")
        echo "Running basic connectivity test..."
        echo "1. Starting server..."
        ./bin/server &
        SERVER_PID=$!
        sleep 2
        
        echo "2. Starting client..."
        echo -e "Hello, World!\n/stats\n/quit" | ./bin/client localhost:8080
        
        echo "3. Cleaning up..."
        kill $SERVER_PID 2>/dev/null
        ;;
        
    "chat")
        echo "Running chat example..."
        echo "1. Starting chat server..."
        ./bin/chat -server &
        SERVER_PID=$!
        sleep 1
        
        echo "2. Starting two chat clients..."
        echo -e "Alice here!\nHow's everyone?\n/quit" | ./bin/chat -addr localhost:8080 &
        CLIENT1_PID=$!
        
        sleep 1
        echo -e "Bob joining!\nHey Alice!\n/quit" | ./bin/chat -addr localhost:8080 &
        CLIENT2_PID=$!
        
        sleep 3
        kill $SERVER_PID $CLIENT1_PID $CLIENT2_PID 2>/dev/null
        ;;
        
    "perf")
        echo "Running performance test..."
        echo "1. Starting performance server..."
        ./bin/perf-test -server &
        SERVER_PID=$!
        sleep 1
        
        echo "2. Running performance client..."
        ./bin/perf-test -messages 50000 -size 200 -clients 5
        
        sleep 2
        kill $SERVER_PID 2>/dev/null
        ;;
        
    "stress")
        echo "Running stress test with packet loss..."
        echo "1. Starting stress server with 5% packet loss..."
        ./bin/stress-test -server -loss 0.05 &
        SERVER_PID=$!
        sleep 1
        
        echo "2. Running stress client for 20 seconds..."
        ./bin/stress-test -duration 20s -concurrent 5
        
        sleep 2
        kill $SERVER_PID 2>/dev/null
        ;;
        
    "fragment")
        echo "Running fragmentation test..."
        echo "1. Starting stress server..."
        ./bin/stress-test -server &
        SERVER_PID=$!
        sleep 1
        
        echo "2. Running client with large messages..."
        ./bin/stress-test -duration 10s -large -concurrent 3
        
        sleep 2
        kill $SERVER_PID 2>/dev/null
        ;;
        
    "all")
        echo "Running all tests..."
        for test in basic chat perf stress fragment; do
            echo ""
            echo ">>> Running $test test..."
            $0 $test
            echo ">>> $test test complete"
            sleep 2
        done
        ;;
        
    *)
        echo "Usage: $0 [test]"
        echo "Tests:"
        echo "  basic    - Basic connectivity test"
        echo "  chat     - Chat example"
        echo "  perf     - Performance benchmark" 
        echo "  stress   - Stress test with packet loss"
        echo "  fragment - Large message fragmentation test"
        echo "  all      - Run all tests"
        ;;
esac

echo ""
echo "Test complete!"
#!/bin/bash

# UDP Network Server Deployment Script
echo "UDP Network Server Deployment"
echo "============================="

# Check if we're in the right directory
if [ ! -f "docker-compose.yml" ]; then
    echo "Error: docker-compose.yml not found. Make sure you're in the docker/ directory."
    exit 1
fi

echo "Found docker-compose.yml"

# Try different docker-compose commands
echo "Attempting to deploy servers..."

# Try Docker Compose V2 first (newer)
if command -v docker &> /dev/null && docker compose version &> /dev/null; then
    echo "Using Docker Compose V2..."
    if docker compose -f docker-compose.yml up -d 2>/dev/null; then
        echo "Deployment successful with Docker Compose V2"
        DEPLOYED=true
    else
        echo "Docker Compose V2 failed, trying V1..."
        if command -v docker-compose &> /dev/null; then
            if docker-compose -f docker-compose.yml up -d; then
                echo "Deployment successful with docker-compose V1"
                DEPLOYED=true
            fi
        fi
    fi
elif command -v docker-compose &> /dev/null; then
    echo "Using Docker Compose V1..."
    # Try with explicit file specification
    if docker-compose -f docker-compose.yml up -d; then
        echo "Deployment successful with docker-compose.yml"
        DEPLOYED=true
    elif docker-compose -f docker-compose.yaml up -d; then
        echo "Deployment successful with docker-compose.yaml"
        DEPLOYED=true
    fi
fi

if [ "$DEPLOYED" = true ]; then
    echo ""
    echo "Deployment complete!"
else
    echo ""
    echo "Deployment failed! Try manual commands:"
    echo "  docker-compose -f docker-compose.yml up -d"
    echo "  OR"
    echo "  docker compose -f docker-compose.yml up -d"
    exit 1
fi
echo ""
echo "Services:"
echo "- Regular UDP Server: port 8080/udp"
echo "- Secure UDP Server:  port 8443/udp (ChaCha20-Poly1305 encrypted)"
echo ""
echo "Check status with:"
echo "  docker-compose ps"
echo "  docker-compose logs -f"
echo ""
echo "Stop with:"
echo "  docker-compose down"
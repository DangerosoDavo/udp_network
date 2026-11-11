> Historical documentation: API, security, size and performance claims below may describe pre-v2 behavior. Follow the [current README](../README.md) and [migration notes](../MAINTENANCE.md) for supported contracts.

# Docker Deployment

This folder contains Docker configuration for deploying the UDP Network servers.

## Quick Start

```bash
# From the project root
cd docker

# Option 1: Use the deployment script (recommended)
./deploy.sh

# Option 2: Manual deployment
docker-compose -f docker-compose.yml up -d

# Option 3: For newer Docker Compose V2
docker compose up -d
```

This will start:
- **Regular UDP Server** on port 8080
- **Secure UDP Server** on port 8443 (encrypted with ChaCha20-Poly1305)

## Files

- `Dockerfile` - Multi-stage build for both server types
- `docker-compose.yml` - Orchestration for both servers
- `DEPLOYMENT.md` - Complete deployment guide
- `.dockerignore` - Build optimization

## Usage

```bash
# Start both servers
docker-compose up -d

# Start only regular server
docker-compose up -d udp-server

# Start only secure server  
docker-compose up -d udp-secure-server

# View logs
docker-compose logs -f

# Stop services
docker-compose down
```

See `DEPLOYMENT.md` for complete documentation.
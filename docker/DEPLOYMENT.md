# UDP Network Server Deployment Guide

This guide shows you how to deploy the UDP network server using Docker.

## Prerequisites

- Docker and Docker Compose installed on your server
- Git installed
- Ports 8080/udp and 8443/udp available

## Quick Deployment

1. **Clone the repository:**
   ```bash
   git clone https://github.com/DangerosoDavo/udp_network.git
   cd udp_network/docker
   ```

2. **Deploy both servers:**
   ```bash
   # Try with explicit file specification first
   docker-compose -f docker-compose.yml up -d
   
   # Alternative if above fails:
   docker-compose -f docker-compose.yaml up -d
   
   # Or if using newer Docker Compose V2:
   docker compose up -d
   ```

3. **Check status:**
   ```bash
   docker-compose ps
   docker-compose logs
   ```

## Individual Server Deployment

### Regular UDP Server

Deploy the standard UDP server on port 8080:

```bash
# Build and run
docker-compose up -d udp-server

# Or build manually
docker build -t udp-network -f docker/Dockerfile .
docker run -d --name udp-server -p 8080:8080/udp udp-network ./udp-server
```

### Secure UDP Server

Deploy the encrypted UDP server on port 8443:

```bash
# Build and run
docker-compose up -d udp-secure-server

# Or build manually
docker build -t udp-network -f docker/Dockerfile .
docker run -d --name udp-secure-server -p 8443:8443/udp udp-network ./udp-secure-server
```

## Custom Configuration

### Environment Variables

- `UDP_PORT`: Port to listen on (default: 8080 for regular, 8443 for secure)
- `LOG_LEVEL`: Logging level (info, debug, warn, error)

### Custom Ports

Edit `docker-compose.yml` to change ports:

```yaml
services:
  udp-server:
    ports:
      - "9999:8080/udp"  # External:Internal
    environment:
      - UDP_PORT=8080
```

## Testing Your Deployment

### Test Regular Server
```bash
# Using the included client
go run cmd/client/main.go

# Or using netcat
echo "Hello" | nc -u your-server-ip 8080
```

### Test Secure Server
```bash
# Run the secure client test
go run tests/crypto-functional/main.go
```

## Server Features

### Regular UDP Server (Port 8080)
- High-performance UDP communication
- Reliable message delivery with ACK/NACK
- Connection management
- Message fragmentation and reassembly
- Flow control and congestion management

### Secure UDP Server (Port 8443)
- All features of regular server
- **ChaCha20-Poly1305 encryption** for all data
- **Curve25519 key exchange** for perfect forward secrecy
- Automatic crypto handshake
- Transparent encryption/decryption

## Monitoring

### Check Logs
```bash
# All services
docker-compose logs -f

# Specific service
docker-compose logs -f udp-server
docker-compose logs -f udp-secure-server
```

### Health Checks
```bash
# Check if containers are healthy
docker-compose ps
```

## Production Considerations

1. **Firewall**: Ensure UDP ports 8080 and 8443 are open
2. **Resource Limits**: Add memory/CPU limits in docker-compose.yml
3. **Logging**: Configure log rotation and centralized logging
4. **Monitoring**: Set up monitoring for connection counts and performance
5. **Backup**: The servers are stateless, no data backup needed

## Stopping Services

```bash
# Stop all services
docker-compose down

# Stop specific service
docker-compose stop udp-server
```

## Troubleshooting

### Common Issues

1. **Port already in use:**
   ```bash
   # Check what's using the port
   sudo netstat -tulpn | grep :8080
   ```

2. **Container won't start:**
   ```bash
   # Check logs
   docker-compose logs udp-server
   ```

3. **Can't connect:**
   - Verify firewall allows UDP traffic
   - Check if container is running: `docker-compose ps`
   - Test locally first: `nc -u localhost 8080`

### Performance Tuning

For high-load deployments, consider:

```yaml
# In docker-compose.yml
services:
  udp-server:
    deploy:
      resources:
        limits:
          memory: 512M
          cpus: '0.5'
    ulimits:
      nofile: 65536
```

## License

This server is released under a non-commercial license. See LICENSE file for details.
Commercial use requires written permission from the author.
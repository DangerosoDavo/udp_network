# Build stage
FROM golang:1.21-alpine AS builder

# Set working directory
WORKDIR /app

# Install git (needed for go mod download)
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the server binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o udp-server cmd/server/main.go

# Build the secure server binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o udp-secure-server cmd/secure-server/main.go

# Runtime stage
FROM alpine:latest

# Install ca-certificates for any HTTPS calls
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1001 -S udpserver && \
    adduser -u 1001 -S udpserver -G udpserver

WORKDIR /app

# Copy binaries from builder stage
COPY --from=builder /app/udp-server .
COPY --from=builder /app/udp-secure-server .

# Change ownership to non-root user
RUN chown -R udpserver:udpserver /app
USER udpserver

# Expose UDP ports (regular and secure)
EXPOSE 8080/udp 8443/udp

# Default command (can be overridden)
CMD ["./udp-server"]
# PinAI Subnet - Multi-stage Docker Build
# This Dockerfile creates an optimized image for running Subnet components

# ============================================
# Stage 1: Builder
# ============================================
FROM golang:1.24-alpine AS builder

# Install build dependencies including bash (required by Makefile)
RUN apk add --no-cache git make gcc musl-dev bash

# Set working directory
WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build all binaries
RUN make build

# ============================================
# Stage 2: Runtime
# ============================================
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    bash \
    curl \
    jq \
    openssl

# Create non-root user
RUN addgroup -g 1000 subnet && \
    adduser -D -u 1000 -G subnet subnet

# Set working directory
WORKDIR /app

# Copy binaries from builder
COPY --from=builder /build/bin/* /app/bin/

# Copy scripts
COPY --from=builder /build/scripts /app/scripts

# Copy config templates
COPY --from=builder /build/config /app/config

# Create directories for data and logs
RUN mkdir -p /app/data /app/logs && \
    chown -R subnet:subnet /app

# Switch to non-root user
USER subnet

# Expose ports
# Registry: 8091 (gRPC), 8101 (HTTP)
# Matcher: 8090 (gRPC), 8091 (HTTP)
# Validator: 9090 (gRPC), 7400 (Raft), 7950 (Gossip)
EXPOSE 8090 8091 8101 9090 7400 7950

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD curl -f http://localhost:8091/health || exit 1

# Default command (can be overridden)
CMD ["/bin/bash"]


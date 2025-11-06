# PinAI Subnet - Production Binary Distribution
# This Dockerfile packages ONLY pre-compiled binaries
# Source code is NOT included - suitable for production deployment

FROM alpine:latest

# Metadata
LABEL org.opencontainers.image.title="PinAI Subnet"
LABEL org.opencontainers.image.description="PinAI Subnet Validator/Matcher/Registry - Binary Distribution"
LABEL org.opencontainers.image.vendor="PIN AI"

# Install minimal runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    bash \
    curl \
    netcat-openbsd \
    gettext

# Create non-root user for security
RUN addgroup -g 1000 subnet && \
    adduser -D -u 1000 -G subnet subnet

# Set working directory
WORKDIR /app

# Copy pre-compiled binaries
# IMPORTANT: These must be built before running docker build
COPY bin/validator /app/bin/validator
COPY bin/matcher /app/bin/matcher
COPY bin/registry /app/bin/registry
COPY bin/simple-agent /app/bin/simple-agent

# Copy entrypoint script
COPY deployment/docker/entrypoint.sh /app/entrypoint.sh

# Set permissions
RUN chmod +x /app/bin/* /app/entrypoint.sh && \
    mkdir -p /app/data /app/logs /app/config && \
    chown -R subnet:subnet /app

# Switch to non-root user
USER subnet

# Set entrypoint
ENTRYPOINT ["/app/entrypoint.sh"]

# Expose common ports (can be overridden in docker-compose)
EXPOSE 8090 8091 8101 9090 7400 7950

# Health check (generic - will be overridden per service)
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD exit 0

# Default command
CMD ["/bin/bash"]


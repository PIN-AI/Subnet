#!/bin/bash
# PinAI Subnet - Production Entrypoint Script
# Handles environment variable substitution for mounted config files

set -e

# Function to substitute environment variables in config files
substitute_env_vars() {
    local source_file=$1
    local dest_file=$2
    
    if [ -f "$source_file" ]; then
        echo "[entrypoint] Processing config: $source_file -> $dest_file"
        if command -v envsubst >/dev/null 2>&1; then
            envsubst < "$source_file" > "$dest_file"
        else
            echo "[entrypoint] Warning: envsubst not found, copying without substitution"
            cp "$source_file" "$dest_file"
        fi
    fi
}

# Create runtime config directory
RUNTIME_CONFIG_DIR="/tmp/config"
mkdir -p "$RUNTIME_CONFIG_DIR"

# Process matcher config if it exists
if [ -f "/app/config/matcher.yaml" ]; then
    substitute_env_vars "/app/config/matcher.yaml" "$RUNTIME_CONFIG_DIR/matcher.yaml"
fi

# Process other config files
if [ -f "/app/config/auth_config.yaml" ]; then
    substitute_env_vars "/app/config/auth_config.yaml" "$RUNTIME_CONFIG_DIR/auth_config.yaml"
fi

if [ -f "/app/config/policy_config.yaml" ]; then
    substitute_env_vars "/app/config/policy_config.yaml" "$RUNTIME_CONFIG_DIR/policy_config.yaml"
fi

echo "[entrypoint] Starting service: $@"

# Execute the main command
exec "$@"


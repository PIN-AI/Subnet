#!/bin/bash
# Docker Entrypoint Script
# Performs environment variable substitution in configuration files before starting services

set -e

# Function to substitute environment variables in a file
# Creates a new file in /tmp to avoid permission issues
substitute_env_vars() {
    local source_file=$1
    local dest_file=$2
    
    if [ -f "$source_file" ]; then
        echo "Substituting environment variables: $source_file -> $dest_file"
        # Use envsubst to replace variables
        if command -v envsubst >/dev/null 2>&1; then
            envsubst < "$source_file" > "$dest_file"
            echo "Environment variable substitution completed for $dest_file"
        else
            echo "Warning: envsubst not found, copying file without substitution"
            cp "$source_file" "$dest_file"
        fi
    fi
}

# Create runtime config directory in /tmp (writable by non-root user)
RUNTIME_CONFIG_DIR="/tmp/config"
mkdir -p "$RUNTIME_CONFIG_DIR"

# Substitute variables in matcher config if it exists
if [ -f "/app/config/matcher.yaml" ]; then
    substitute_env_vars "/app/config/matcher.yaml" "$RUNTIME_CONFIG_DIR/matcher.yaml"
    # Update command to use the new config path if it references /app/config/matcher.yaml
    set -- "${@/\/app\/config\/matcher.yaml/$RUNTIME_CONFIG_DIR\/matcher.yaml}"
fi

# Substitute variables in other config files that might use env vars
if [ -f "/app/config/auth_config.yaml" ]; then
    substitute_env_vars "/app/config/auth_config.yaml" "$RUNTIME_CONFIG_DIR/auth_config.yaml"
fi

# Execute the main command
exec "$@"


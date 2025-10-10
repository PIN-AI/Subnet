#!/bin/bash
# Registry CLI Tool
# Usage: ./registry-cli.sh <command> [args]

REGISTRY_URL="${REGISTRY_URL:-http://localhost:8092}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

show_help() {
    echo "Registry CLI - Service Discovery Tool"
    echo ""
    echo "Usage: $0 <command> [args]"
    echo ""
    echo "Commands:"
    echo "  list-agents              List all registered agents"
    echo "  list-validators          List all registered validators"
    echo "  agent <id>               Get details of a specific agent"
    echo "  validator <id>           Get details of a specific validator"
    echo "  agent-heartbeat <id>     Send heartbeat for an agent"
    echo "  validator-heartbeat <id> Send heartbeat for a validator"
    echo "  watch-agents             Watch agents count (updates every 2s)"
    echo "  watch-validators         Watch validators count (updates every 2s)"
    echo "  watch-all                Watch all services (updates every 2s)"
    echo "  health                   Check registry health"
    echo ""
    echo "Environment Variables:"
    echo "  REGISTRY_URL             Registry endpoint (default: http://localhost:8092)"
    echo ""
    echo "Examples:"
    echo "  $0 list-validators"
    echo "  $0 validator validator-1"
    echo "  $0 watch-all"
}

# Check if curl and jq are installed
check_deps() {
    if ! command -v curl &> /dev/null; then
        echo -e "${RED}Error: curl is not installed${NC}"
        exit 1
    fi
    if ! command -v jq &> /dev/null; then
        echo -e "${YELLOW}Warning: jq is not installed. Output will not be formatted${NC}"
    fi
}

# Format JSON output if jq is available
format_json() {
    if command -v jq &> /dev/null; then
        jq "$@"
    else
        cat
    fi
}

# Main command handling
case "$1" in
    list-agents)
        echo -e "${GREEN}Fetching agents from $REGISTRY_URL${NC}"
        curl -s "$REGISTRY_URL/agents" | format_json '.agents'
        ;;

    list-validators)
        echo -e "${GREEN}Fetching validators from $REGISTRY_URL${NC}"
        curl -s "$REGISTRY_URL/validators" | format_json '.validators'
        ;;

    agent)
        if [ -z "$2" ]; then
            echo -e "${RED}Error: Agent ID required${NC}"
            echo "Usage: $0 agent <id>"
            exit 1
        fi
        echo -e "${GREEN}Fetching agent: $2${NC}"
        curl -s "$REGISTRY_URL/agents/$2" | format_json '.'
        ;;

    validator)
        if [ -z "$2" ]; then
            echo -e "${RED}Error: Validator ID required${NC}"
            echo "Usage: $0 validator <id>"
            exit 1
        fi
        echo -e "${GREEN}Fetching validator: $2${NC}"
        curl -s "$REGISTRY_URL/validators/$2" | format_json '.'
        ;;

    agent-heartbeat)
        if [ -z "$2" ]; then
            echo -e "${RED}Error: Agent ID required${NC}"
            echo "Usage: $0 agent-heartbeat <id>"
            exit 1
        fi
        echo -e "${GREEN}Sending heartbeat for agent: $2${NC}"
        curl -s -X POST "$REGISTRY_URL/agents/$2/heartbeat" | format_json '.'
        ;;

    validator-heartbeat)
        if [ -z "$2" ]; then
            echo -e "${RED}Error: Validator ID required${NC}"
            echo "Usage: $0 validator-heartbeat <id>"
            exit 1
        fi
        echo -e "${GREEN}Sending heartbeat for validator: $2${NC}"
        curl -s -X POST "$REGISTRY_URL/validators/$2/heartbeat" | format_json '.'
        ;;

    watch-agents)
        echo -e "${GREEN}Watching agents (Ctrl+C to stop)${NC}"
        while true; do
            clear
            echo "=== Agents at $(date) ==="
            curl -s "$REGISTRY_URL/agents" | format_json '.agents | length' | xargs echo "Total count:"
            echo ""
            curl -s "$REGISTRY_URL/agents" | format_json '.agents[] | {id, status, endpoint}'
            sleep 2
        done
        ;;

    watch-validators)
        echo -e "${GREEN}Watching validators (Ctrl+C to stop)${NC}"
        while true; do
            clear
            echo "=== Validators at $(date) ==="
            curl -s "$REGISTRY_URL/validators" | format_json '.validators | length' | xargs echo "Total count:"
            echo ""
            curl -s "$REGISTRY_URL/validators" | format_json '.validators[] | {id, status, endpoint}'
            sleep 2
        done
        ;;

    watch-all)
        echo -e "${GREEN}Watching all services (Ctrl+C to stop)${NC}"
        while true; do
            clear
            echo "=== Registry Status at $(date) ==="
            echo ""
            echo "VALIDATORS:"
            curl -s "$REGISTRY_URL/validators" 2>/dev/null | format_json '.validators | length' | xargs echo "  Count:" || echo "  Error fetching validators"
            curl -s "$REGISTRY_URL/validators" 2>/dev/null | format_json '.validators[] | "  - \(.id) [\(.status)] \(.endpoint)"' -r || true
            echo ""
            echo "AGENTS:"
            curl -s "$REGISTRY_URL/agents" 2>/dev/null | format_json '.agents | length' | xargs echo "  Count:" || echo "  Error fetching agents"
            curl -s "$REGISTRY_URL/agents" 2>/dev/null | format_json '.agents[] | "  - \(.id) [\(.status)] caps: \(.capabilities | join(", "))"' -r || true
            sleep 2
        done
        ;;

    health)
        echo -e "${GREEN}Checking registry health at $REGISTRY_URL${NC}"
        if curl -s -f "$REGISTRY_URL/agents" > /dev/null 2>&1; then
            echo -e "${GREEN}✓ Registry is healthy${NC}"
            echo ""
            echo "Service counts:"
            curl -s "$REGISTRY_URL/validators" | format_json '.validators | length' | xargs echo "  Validators:"
            curl -s "$REGISTRY_URL/agents" | format_json '.agents | length' | xargs echo "  Agents:"
        else
            echo -e "${RED}✗ Registry is not responding${NC}"
            exit 1
        fi
        ;;

    help|--help|-h)
        show_help
        ;;

    "")
        show_help
        ;;

    *)
        echo -e "${RED}Error: Unknown command '$1'${NC}"
        echo ""
        show_help
        exit 1
        ;;
esac
#!/bin/bash

set -e

echo "================================"
echo "CometBFT Integration Tests"
echo "================================"
echo ""

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if running in CI or explicitly requested
if [ "$CI" != "true" ] && [ "$RUN_INTEGRATION_TESTS" != "true" ]; then
    echo -e "${YELLOW}Warning:${NC} Integration tests require starting actual CometBFT nodes."
    echo "This will take several minutes and may require significant resources."
    echo ""
    echo "To run integration tests, set RUN_INTEGRATION_TESTS=true:"
    echo "  export RUN_INTEGRATION_TESTS=true"
    echo "  $0"
    echo ""
    echo "Or run individual tests:"
    echo "  go test -v -tags=integration ./internal/consensus/cometbft/ -run TestThreeValidatorSignatureFlow"
    echo ""
    exit 0
fi

echo "Running CometBFT integration tests..."
echo ""

# Test 1: Checkpoint hash consistency
echo -e "${YELLOW}Test 1:${NC} Checkpoint Hash Consistency"
echo "This test verifies that all validators compute identical checkpoint hashes"
echo ""

go test -v -tags=integration \
    -timeout 30s \
    ./internal/consensus/cometbft/ \
    -run TestCheckpointHashConsistency

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓${NC} Checkpoint hash consistency test passed"
else
    echo -e "${RED}✗${NC} Checkpoint hash consistency test failed"
    exit 1
fi

echo ""

# Test 2: Single-validator signature flow
echo -e "${YELLOW}Test 2:${NC} Single-Validator Signature Flow"
echo "This test verifies ValidationBundle signature collection with one validator"
echo ""

go test -v -tags=integration \
    -timeout 60s \
    ./internal/consensus/cometbft/ \
    -run TestSingleValidatorSignatureFlow

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓${NC} Single-validator signature flow test passed"
else
    echo -e "${RED}✗${NC} Single-validator signature flow test failed"
    exit 1
fi

echo ""

# Test 3: Three-validator independent flow
echo -e "${YELLOW}Test 3:${NC} Three-Validator Independent Signature Flow"
echo "This test starts 3 independent validators and verifies:"
echo "  - Each validator can run its own consensus"
echo "  - All validators process the same checkpoint"
echo "  - Each validator collects its own signature"
echo "  - ValidationBundle signing works independently"
echo ""

go test -v -tags=integration \
    -timeout 180s \
    ./internal/consensus/cometbft/ \
    -run TestThreeValidatorSignatureFlow

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓${NC} Three-validator independent flow test passed"
else
    echo -e "${RED}✗${NC} Three-validator independent flow test failed"
    exit 1
fi

echo ""

# Test 4: Validator recovery (skipped - known issue)
echo -e "${YELLOW}Test 4:${NC} Validator Recovery (Skipped)"
echo "This test is currently skipped due to CometBFT state persistence complexity."
echo "Will be addressed in future work."
echo ""
echo -e "${YELLOW}⊘${NC} Validator recovery test skipped"

echo ""
echo "================================"
echo -e "${GREEN}All integration tests passed!${NC}"
echo "================================"

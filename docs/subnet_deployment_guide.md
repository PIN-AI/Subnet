# Subnet Complete Deployment and Custom Development Guide

## Table of Contents

1. [Overview](#overview)
2. [Quick Deployment (Using Default Configuration)](#quick-deployment-using-default-configuration)
3. [Custom Development Guide](#custom-development-guide)
   - [Matcher Strategy Customization](#matcher-strategy-customization)
   - [Validator Logic Customization](#validator-logic-customization)
   - [Agent Executor Customization](#agent-executor-customization)
4. [Production Deployment](#production-deployment)
5. [Troubleshooting](#troubleshooting)

---

## Overview

### Subnet Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        RootLayer                            │
│   (Blockchain + Intent Pool + Assignment + Validation)      │
└─────────────────┬───────────────────────────┬───────────────┘
                  │                           │
        Intents   │                           │  ValidationBundles
                  ↓                           ↑
┌─────────────────────────────┐   ┌──────────────────────────┐
│         Matcher             │   │      Validators          │
│  - Pull Intents             │   │  - Verify Reports        │
│  - Manage Bidding Windows   │   │  - Consensus Signatures  │
│  - Matching Algorithm       │   │  - Submit Bundles        │
│  - Assign Tasks to Agents   │   │  - Custom Logic          │
└──────────┬──────────────────┘   └────────┬─────────────────┘
           │ Assignments                   │ ExecutionReports
           ↓                               ↑
    ┌────────────────────────────────────────┐
    │            Agents (SDK)                │
    │  - Subscribe to Intents                │
    │  - Submit Bids                         │
    │  - Execute Tasks (Custom Logic)        │
    │  - Submit ExecutionReports             │
    └────────────────────────────────────────┘
```

### Components Requiring Customization

| Component | Required? | Customization |
|-----------|-----------|---------------|
| **Matcher Strategy** | Optional | How to select winner from agent bids |
| **Validator Logic** | **Required** | How to verify execution correctness |
| **Agent Executor** | **Required** | How to execute specific intent tasks |
| **Intent Type Definition** | **Required** | Define supported intent types and params |

---

## Quick Deployment (Using Default Configuration)

### Prerequisites

```bash
# 1. Install dependencies
go version  # >= 1.21 required
docker --version  # Optional, for NATS

# 2. Clone repository
git clone https://github.com/PIN-AI/Subnet
cd Subnet

# 3. Build all components
make build
```

### Step 1: Start Infrastructure

```bash
# Start NATS (required for validator consensus)
docker run -d --name nats -p 4222:4222 nats:latest

# Or use local NATS
brew install nats-server  # macOS
nats-server &
```

### Step 2: Create Subnet on Blockchain

```bash
# Set environment variables
export RPC_URL="https://sepolia.base.org"
export PRIVATE_KEY="<your-private-key>"
export PIN_NETWORK="base_sepolia"

# Run creation script
./scripts/create-subnet.sh --name "My Test Subnet"

# Example output:
# ✅ Subnet created successfully!
# Subnet ID: 0x0000000000000000000000000000000000000000000000000000000000000002
# Contract Address: 0x6538D64e4eeE64641f6c66aB2c7703edfC3e2aB6
```

**Save the Subnet ID and Contract Address!**

### Step 3: Configure and Start Matcher

Edit `config/matcher-config.yaml`:

```yaml
identity:
  subnet_id: "0x0000...0002"  # From Step 2
  matcher_id: "matcher-1"

# RootLayer connection
rootlayer:
  grpc_endpoint: "3.17.208.238:9001"
  http_url: "http://3.17.208.238:8081/api"

# Blockchain configuration
blockchain:
  enabled: true
  rpc_url: "https://sepolia.base.org"
  subnet_contract: "0x6538..."  # From Step 2

# Matching strategy (using default weighted strategy)
matcher:
  strategy:
    type: "weighted"
    config:
      price_weight: 0.5
      reputation_weight: 0.3
      capability_weight: 0.2

# Bidding window
limits:
  bidding_window_sec: 10

# Signing key (use KMS in production)
private_key: "<MATCHER_PRIVATE_KEY>"
```

Start Matcher:

```bash
./bin/matcher --config config/matcher-config.yaml
```

### Step 4: Configure and Start Validators

Edit `config/validator-1-config.yaml`:

```yaml
identity:
  subnet_id: "0x0000...0002"
  validator_id: "validator-1"

# RootLayer connection
rootlayer:
  grpc_endpoint: "3.17.208.238:9001"
  http_url: "http://3.17.208.238:8081/api"

# Blockchain configuration
blockchain:
  enabled: true
  rpc_url: "https://sepolia.base.org"
  intent_manager: "0xD04d23775D3B8e028e6104E31eb0F6c07206EB46"

# Consensus configuration
validator_set:
  min_validators: 3
  threshold_num: 2      # 2/3 threshold
  threshold_denom: 3

# NATS message bus
nats:
  url: "nats://localhost:4222"

# Storage
storage:
  leveldb_path: "./data/validator-1.db"

# Signing key
private_key: "<VALIDATOR_PRIVATE_KEY>"
```

Start multiple Validators (at least 3):

```bash
# Terminal 1
./bin/validator --config config/validator-1-config.yaml

# Terminal 2
./bin/validator --config config/validator-2-config.yaml

# Terminal 3
./bin/validator --config config/validator-3-config.yaml
```

### Step 5: Run Tests

```bash
# Run E2E tests to verify deployment
export SUBNET_ID="0x0000...0002"
export ROOTLAYER_GRPC="3.17.208.238:9001"
export ROOTLAYER_HTTP="http://3.17.208.238:8081"

./scripts/e2e-test.sh
```

---

## Custom Development Guide

### Matcher Strategy Customization

#### Scenario: Implement geo-based matching strategy

**Step 1: Create Custom Strategy**

Create `custom_matcher/geo_strategy.go`:

```go
package custom_matcher

import (
    "sort"
    "subnet/internal/matcher"
    "subnet/internal/types"
)

// GeoLocationStrategy prioritizes agents closest to target region
type GeoLocationStrategy struct {
    preferredRegions []string
    logger           logging.Logger
}

func NewGeoLocationStrategy(regions []string, logger logging.Logger) *GeoLocationStrategy {
    return &GeoLocationStrategy{
        preferredRegions: regions,
        logger:           logger,
    }
}

// Match implements the MatchingStrategy interface
func (s *GeoLocationStrategy) Match(snapshot *matcher.IntentBidSnapshot) *types.MatchingResult {
    if len(snapshot.Bids) == 0 {
        return nil
    }

    // 1. Group by region
    preferredBids := make([]*types.Bid, 0)
    otherBids := make([]*types.Bid, 0)

    for _, bid := range snapshot.Bids {
        region := bid.Metadata["region"] // Agents provide region in metadata
        if s.isPreferredRegion(region) {
            preferredBids = append(preferredBids, bid)
        } else {
            otherBids = append(otherBids, bid)
        }
    }

    // 2. Prefer bids from preferred regions
    var candidates []*types.Bid
    if len(preferredBids) > 0 {
        candidates = preferredBids
        s.logger.Infof("Found %d bids in preferred regions", len(preferredBids))
    } else {
        candidates = otherBids
        s.logger.Warnf("No bids in preferred regions, using other regions")
    }

    // 3. Sort by price
    sort.Slice(candidates, func(i, j int) bool {
        return candidates[i].Price < candidates[j].Price
    })

    // 4. Select lowest price winner
    winner := candidates[0]

    // 5. Select runner-ups
    runnerUps := make([]*types.Bid, 0)
    for i := 1; i < len(candidates) && i < 3; i++ {
        runnerUps = append(runnerUps, candidates[i])
    }

    return &types.MatchingResult{
        IntentID:       snapshot.IntentID,
        WinningBid:     winner,
        WinningAgentID: winner.AgentID,
        RunnerUpBids:   runnerUps,
        MatchedAt:      snapshot.BiddingEndTime,
        MatcherID:      s.logger.GetMatcherID(),
    }
}

func (s *GeoLocationStrategy) isPreferredRegion(region string) bool {
    for _, preferred := range s.preferredRegions {
        if region == preferred {
            return true
        }
    }
    return false
}
```

**Step 2: Modify Matcher Startup**

Edit `cmd/matcher/main.go`:

```go
import (
    "custom_matcher"
    "subnet/internal/matcher"
)

func main() {
    // ... existing config loading ...

    // Create custom matching strategy
    strategy := custom_matcher.NewGeoLocationStrategy(
        []string{"us-west", "us-east", "eu-central"},
        logger,
    )

    // Create matcher server (inject custom strategy)
    matcherServer := matcher.NewServerWithStrategy(cfg, logger, strategy)

    // Start service
    matcherServer.Start(ctx)
}
```

**Step 3: Configure and Run**

```yaml
# config/custom-matcher-config.yaml
matcher:
  strategy:
    type: "custom_geo"  # Identify custom strategy
    config:
      preferred_regions:
        - "us-west"
        - "us-east"
        - "eu-central"
```

```bash
go build -o bin/custom-matcher ./cmd/matcher
./bin/custom-matcher --config config/custom-matcher-config.yaml
```

---

### Validator Logic Customization

#### Scenario: Image processing subnet, verify output image dimensions

See detailed documentation in `docs/CUSTOM_VALIDATION_GUIDE.md`. Quick example:

**Step 1: Implement Validator**

Create `custom_validator/image_validator.go`:

```go
package custom_validator

import (
    "encoding/json"
    "fmt"
    "image"
    _ "image/jpeg"
    _ "image/png"
    "bytes"

    "subnet/internal/validator"
    pb "subnet/proto/subnet"
    rootpb "rootlayer/proto"
)

type ImageResizeValidator struct{}

func (v *ImageResizeValidator) ValidateResult(report *pb.ExecutionReport, intent *rootpb.Intent) error {
    // 1. Parse intent parameters
    var params struct {
        TargetWidth  int `json:"target_width"`
        TargetHeight int `json:"target_height"`
    }
    json.Unmarshal(intent.Params.IntentRaw, &params)

    // 2. Parse result data
    var result struct {
        ImageData []byte `json:"image_data"`
        Width     int    `json:"width"`
        Height    int    `json:"height"`
    }
    json.Unmarshal(report.ResultData, &result)

    // 3. Verify dimensions
    if result.Width != params.TargetWidth || result.Height != params.TargetHeight {
        return fmt.Errorf("dimensions incorrect: got %dx%d, expected %dx%d",
            result.Width, result.Height, params.TargetWidth, params.TargetHeight)
    }

    // 4. Verify image data validity
    img, _, err := image.Decode(bytes.NewReader(result.ImageData))
    if err != nil {
        return fmt.Errorf("invalid image data: %w", err)
    }

    // 5. Verify actual dimensions
    bounds := img.Bounds()
    if bounds.Dx() != result.Width || bounds.Dy() != result.Height {
        return fmt.Errorf("image dimensions mismatch")
    }

    return nil
}
```

**Step 2: Register to Validator**

Edit `cmd/validator/main.go`:

```go
import (
    "custom_validator"
    "subnet/internal/validator"
)

func main() {
    // ... existing config ...

    // Create validator registry
    registry := validator.NewIntentTypeValidatorRegistry()

    // Register custom validators
    registry.RegisterValidator("image-resize", &custom_validator.ImageResizeValidator{})
    registry.RegisterValidator("image-crop", &custom_validator.ImageCropValidator{})

    // Create validator node
    node := validator.NewNode(config, logger)

    // Set custom validator registry
    node.SetValidatorRegistry(registry)

    // Start
    node.Start(ctx)
}
```

---

### Agent Executor Customization

#### Scenario: Image Processing Agent

Agents should be developed in `../subnet-sdk`, but here's an example:

**Step 1: Create Agent using SDK**

Create `my-image-agent/main.go`:

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "image"
    "image/jpeg"
    "bytes"

    sdk "github.com/PIN-AI/subnet-sdk/go"
)

type ImageProcessingAgent struct {
    client *sdk.ValidatorClient
}

func (a *ImageProcessingAgent) ProcessIntent(ctx context.Context, task *sdk.ExecutionTask) error {
    // 1. Parse intent data
    var params struct {
        ImageURL     string `json:"image_url"`
        TargetWidth  int    `json:"target_width"`
        TargetHeight int    `json:"target_height"`
    }
    if err := json.Unmarshal(task.IntentData, &params); err != nil {
        return fmt.Errorf("invalid intent data: %w", err)
    }

    // 2. Download original image
    img, err := downloadImage(params.ImageURL)
    if err != nil {
        return fmt.Errorf("failed to download image: %w", err)
    }

    // 3. Resize image
    resizedImg := resizeImage(img, params.TargetWidth, params.TargetHeight)

    // 4. Encode to JPEG
    var buf bytes.Buffer
    if err := jpeg.Encode(&buf, resizedImg, &jpeg.Options{Quality: 90}); err != nil {
        return fmt.Errorf("failed to encode image: %w", err)
    }

    // 5. Construct result
    result := map[string]interface{}{
        "image_data": buf.Bytes(),
        "width":      params.TargetWidth,
        "height":     params.TargetHeight,
        "format":     "jpeg",
    }
    resultData, _ := json.Marshal(result)

    // 6. Submit ExecutionReport
    report := &sdk.ExecutionReport{
        AssignmentId: task.TaskId,
        IntentId:     task.IntentId,
        AgentId:      a.client.AgentID,
        Status:       sdk.ExecutionStatus_SUCCESS,
        ResultData:   resultData,
        Evidence: &sdk.VerificationEvidence{
            OutputsHash: computeHash(resultData),
        },
    }

    return a.client.SubmitExecutionReport(ctx, report)
}

func main() {
    // Create SDK client
    client := sdk.NewValidatorClient(sdk.Config{
        MatcherAddr:    "localhost:8090",
        ValidatorAddrs: []string{"localhost:9200"},
        AgentID:        "image-agent-001",
        PrivateKey:     "<AGENT_PRIVATE_KEY>",
    })

    // Create agent
    agent := &ImageProcessingAgent{client: client}

    // Subscribe to tasks
    ctx := context.Background()
    client.SubscribeToTasks(ctx, agent.ProcessIntent)
}
```

**Step 2: Build and Run**

```bash
cd my-image-agent
go mod init my-image-agent
go get github.com/PIN-AI/subnet-sdk/go
go build -o image-agent
./image-agent
```

---

## Production Deployment

### Security Checklist

- [ ] Use KMS for private key management (don't hardcode)
- [ ] Enable TLS/mTLS for gRPC
- [ ] Configure firewall rules
- [ ] Enable blockchain participant verification
- [ ] Set up log collection (ELK/Prometheus)
- [ ] Configure monitoring and alerts
- [ ] Set up auto-restart (systemd/Kubernetes)

### Docker Deployment Example

**Matcher Dockerfile:**

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o matcher ./cmd/matcher

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=builder /app/matcher /usr/local/bin/
COPY config/matcher-config.yaml /etc/subnet/

CMD ["matcher", "--config", "/etc/subnet/matcher-config.yaml"]
```

**Docker Compose:**

```yaml
version: '3.8'

services:
  nats:
    image: nats:latest
    ports:
      - "4222:4222"

  matcher:
    build:
      context: .
      dockerfile: Dockerfile.matcher
    environment:
      - SUBNET_ID=${SUBNET_ID}
      - PRIVATE_KEY=${MATCHER_KEY}
    depends_on:
      - nats

  validator-1:
    build:
      context: .
      dockerfile: Dockerfile.validator
    environment:
      - SUBNET_ID=${SUBNET_ID}
      - VALIDATOR_ID=validator-1
      - PRIVATE_KEY=${VALIDATOR_1_KEY}
    depends_on:
      - nats

  validator-2:
    build:
      context: .
      dockerfile: Dockerfile.validator
    environment:
      - SUBNET_ID=${SUBNET_ID}
      - VALIDATOR_ID=validator-2
      - PRIVATE_KEY=${VALIDATOR_2_KEY}
    depends_on:
      - nats

  validator-3:
    build:
      context: .
      dockerfile: Dockerfile.validator
    environment:
      - SUBNET_ID=${SUBNET_ID}
      - VALIDATOR_ID=validator-3
      - PRIVATE_KEY=${VALIDATOR_3_KEY}
    depends_on:
      - nats
```

Start:

```bash
docker-compose up -d
```

---

## Troubleshooting

### Common Issues

#### 1. Validators Cannot Reach Consensus

**Symptom:** Logs show "Failed to reach threshold"

**Debug:**
```bash
# Check NATS connection
docker logs <nats-container>

# Check validator logs
tail -f data/validator-1.log | grep "signature"

# Verify validator count
curl http://localhost:9200/validators | jq '.validators | length'
```

**Solution:** Ensure at least `threshold_num` validators are online

#### 2. Agent Not Receiving Tasks

**Symptom:** Agent logs show "No tasks received"

**Debug:**
```bash
# Check if matcher received intents
curl http://localhost:8090/intents | jq '.'

# Check if agent connected to matcher
curl http://localhost:8090/agents | jq '.agents[] | select(.id=="my-agent")'

# View matcher logs
tail -f logs/matcher.log | grep "bid"
```

**Solution:**
- Confirm agent is correctly subscribed to intent stream
- Confirm agent's bid is submitted within bidding window

#### 3. ValidationBundle Submission Failed

**Symptom:** Validator logs show "Failed to submit ValidationBundle"

**Debug:**
```bash
# Check RootLayer connection
nc -zv 3.17.208.238 9001

# Verify signature
validator-debug --verify-signature <bundle-file>

# View detailed error
tail -f logs/validator-1.log | grep "ValidationBundle"
```

**Solution:**
- Verify RootLayer is reachable
- Confirm signing key is configured correctly
- Verify Intent Manager contract address is correct

---

## Next Steps

1. **Read Architecture Docs** - `docs/ARCHITECTURE_OVERVIEW.md`
2. **Review API Docs** - `docs/API_REFERENCE.md`
3. **Join Community** - [Discord](https://discord.gg/pinai) / [GitHub Discussions](https://github.com/PIN-AI/protocol/discussions)
4. **Report Issues** - [GitHub Issues](https://github.com/PIN-AI/protocol/issues)

---

## Appendix

### Complete Configuration Reference

All configuration templates are in `config/` directory:
- `config/matcher-config.yaml` - Matcher configuration
- `config/validator-config.yaml` - Validator configuration
- `config/policy_config.yaml` - Policy configuration
- `config/auth_config.yaml` - Authentication configuration

### Development Tools

```bash
# Generate test intent
go run ./cmd/mock-rootlayer --generate-intent

# Validate configuration file
go run ./cmd/config-validator --config config.yaml

# View validator status
curl http://localhost:9200/status | jq '.'
```

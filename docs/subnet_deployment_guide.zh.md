# Subnet完整部署与自定义开发指南

## 目录

1. [概述](#概述)
2. [快速部署（使用默认配置）](#快速部署使用默认配置)
3. [自定义开发指南](#自定义开发指南)
   - [Matcher匹配策略自定义](#matcher匹配策略自定义)
   - [Validator验证逻辑自定义](#validator验证逻辑自定义)
   - [Agent执行器自定义](#agent执行器自定义)
4. [生产环境部署](#生产环境部署)
5. [故障排查](#故障排查)

---

## 概述

### Subnet架构

```
┌─────────────────────────────────────────────────────────────┐
│                        RootLayer                            │
│  (区块链 + Intent池 + Assignment记录 + Validation聚合)        │
└─────────────────┬───────────────────────────┬───────────────┘
                  │                           │
        Intents   │                           │  ValidationBundles
                  ↓                           ↑
┌─────────────────────────────┐   ┌──────────────────────────┐
│         Matcher             │   │      Validators          │
│  - 拉取Intents              │   │  - 验证ExecutionReports  │
│  - 管理竞价窗口             │   │  - 共识签名              │
│  - 匹配算法（可自定义）      │   │  - 提交ValidationBundles │
│  - 分配任务给Agent          │   │  - 验证逻辑（可自定义）   │
└──────────┬──────────────────┘   └────────┬─────────────────┘
           │ Assignments                   │ ExecutionReports
           ↓                               ↑
    ┌────────────────────────────────────────┐
    │            Agents (SDK)                │
    │  - 订阅Intents                         │
    │  - 提交Bids                            │
    │  - 执行任务（可自定义）                 │
    │  - 提交ExecutionReports                │
    └────────────────────────────────────────┘
```

### 需要自定义的部分

| 组件 | 是否必须自定义 | 自定义内容 |
|------|--------------|-----------|
| **Matcher匹配策略** | 可选 | 如何从多个agent bids中选择获胜者 |
| **Validator验证逻辑** | **必须** | 如何验证执行结果的正确性 |
| **Agent执行器** | **必须** | 如何执行具体的intent任务 |
| **Intent类型定义** | **必须** | 定义子网支持的intent类型和参数格式 |

---

## 快速部署（使用默认配置）

### 前置条件

```bash
# 1. 安装依赖
go version  # 需要 >= 1.21
docker --version  # 可选

# 2. Clone代码
git clone https://github.com/PIN-AI/Subnet
cd Subnet

# 3. 编译所有组件
make build
```

### 步骤1：启动基础设施

**注意**：系统使用 Raft+Gossip 共识。不需要外部消息代理（如 NATS）。

### 步骤2：在区块链上创建Subnet

```bash
# 设置环境变量
export RPC_URL="https://sepolia.base.org"
export PRIVATE_KEY="<你的私钥>"
export PIN_NETWORK="base_sepolia"

# 运行创建脚本
./scripts/create-subnet.sh --name "My Test Subnet"

# 输出示例：
# ✅ Subnet created successfully!
# Subnet ID: 0x0000000000000000000000000000000000000000000000000000000000000002
# Contract Address: 0x6538D64e4eeE64641f6c66aB2c7703edfC3e2aB6
```

**保存输出的 Subnet ID 和 Contract Address！**

### 步骤3：配置并启动Matcher

编辑 `config/matcher-config.yaml`:

```yaml
identity:
  subnet_id: "0x0000...0002"  # 步骤2获得的Subnet ID
  matcher_id: "matcher-1"

# RootLayer连接
rootlayer:
  grpc_endpoint: "3.17.208.238:9001"
  http_url: "http://3.17.208.238:8081/api"

# 区块链配置
blockchain:
  enabled: true
  rpc_url: "https://sepolia.base.org"
  subnet_contract: "0x6538..."  # 步骤2获得的Contract Address

# 匹配策略（使用默认加权策略）
matcher:
  strategy:
    type: "weighted"
    config:
      price_weight: 0.5
      reputation_weight: 0.3
      capability_weight: 0.2

# 竞价窗口
limits:
  bidding_window_sec: 10

# 签名密钥（生产环境使用KMS）
private_key: "<MATCHER_PRIVATE_KEY>"
```

启动Matcher:

```bash
./bin/matcher --config config/matcher-config.yaml
```

### 步骤4：配置并启动Validators

编辑 `config/validator-1-config.yaml`:

```yaml
identity:
  subnet_id: "0x0000...0002"
  validator_id: "validator-1"

# RootLayer连接
rootlayer:
  grpc_endpoint: "3.17.208.238:9001"
  http_url: "http://3.17.208.238:8081/api"

# 区块链配置
blockchain:
  enabled: true
  rpc_url: "https://sepolia.base.org"
  intent_manager: "0xD04d23775D3B8e028e6104E31eb0F6c07206EB46"

# 共识配置
validator_set:
  min_validators: 3
  threshold_num: 2      # 2/3阈值
  threshold_denom: 3

# 存储
storage:
  leveldb_path: "./data/validator-1.db"

# 签名密钥
private_key: "<VALIDATOR_PRIVATE_KEY>"
```

启动多个Validators（至少3个）:

```bash
# Terminal 1
./bin/validator --config config/validator-1-config.yaml

# Terminal 2
./bin/validator --config config/validator-2-config.yaml

# Terminal 3
./bin/validator --config config/validator-3-config.yaml
```

### 步骤5：运行测试

```bash
# 运行E2E测试验证部署
export SUBNET_ID="0x0000...0002"
export ROOTLAYER_GRPC="3.17.208.238:9001"
export ROOTLAYER_HTTP="http://3.17.208.238:8081"

./scripts/e2e-test.sh
```

---

## 自定义开发指南

### Matcher匹配策略自定义

#### 场景：你想实现地域优先的匹配策略

**步骤1：创建自定义策略**

创建 `custom_matcher/geo_strategy.go`:

```go
package custom_matcher

import (
    "sort"
    "subnet/internal/matcher"
    "subnet/internal/types"
)

// GeoLocationStrategy优先选择地理位置最近的agent
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

// Match实现MatchingStrategy接口
func (s *GeoLocationStrategy) Match(snapshot *matcher.IntentBidSnapshot) *types.MatchingResult {
    if len(snapshot.Bids) == 0 {
        return nil
    }

    // 1. 按地域分组
    preferredBids := make([]*types.Bid, 0)
    otherBids := make([]*types.Bid, 0)

    for _, bid := range snapshot.Bids {
        region := bid.Metadata["region"] // 假设agent在metadata中提供region
        if s.isPreferredRegion(region) {
            preferredBids = append(preferredBids, bid)
        } else {
            otherBids = append(otherBids, bid)
        }
    }

    // 2. 优先从preferred region选择，如果没有则从other region选择
    var candidates []*types.Bid
    if len(preferredBids) > 0 {
        candidates = preferredBids
        s.logger.Infof("Found %d bids in preferred regions", len(preferredBids))
    } else {
        candidates = otherBids
        s.logger.Warnf("No bids in preferred regions, using other regions")
    }

    // 3. 在候选中按价格排序
    sort.Slice(candidates, func(i, j int) bool {
        return candidates[i].Price < candidates[j].Price
    })

    // 4. 选择最低价
    winner := candidates[0]

    // 5. 选择runner-ups
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

**步骤2：修改Matcher启动代码**

编辑 `cmd/matcher/main.go`:

```go
import (
    "custom_matcher"
    "subnet/internal/matcher"
)

func main() {
    // ... 现有的配置加载 ...

    // 创建自定义匹配策略
    strategy := custom_matcher.NewGeoLocationStrategy(
        []string{"us-west", "us-east", "eu-central"},
        logger,
    )

    // 创建matcher server（注入自定义策略）
    matcherServer := matcher.NewServerWithStrategy(cfg, logger, strategy)

    // 启动服务
    matcherServer.Start(ctx)
}
```

**步骤3：配置并运行**

```yaml
# config/custom-matcher-config.yaml
matcher:
  strategy:
    type: "custom_geo"  # 标识自定义策略
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

### Validator验证逻辑自定义

#### 场景：图像处理子网，需要验证输出图像尺寸

详细文档见 `docs/CUSTOM_VALIDATION_GUIDE.md`，这里给出快速示例：

**步骤1：实现验证器**

创建 `custom_validator/image_validator.go`:

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
    // 1. 解析intent参数
    var params struct {
        TargetWidth  int `json:"target_width"`
        TargetHeight int `json:"target_height"`
    }
    json.Unmarshal(intent.Params.IntentRaw, &params)

    // 2. 解析result数据
    var result struct {
        ImageData []byte `json:"image_data"`
        Width     int    `json:"width"`
        Height    int    `json:"height"`
    }
    json.Unmarshal(report.ResultData, &result)

    // 3. 验证尺寸
    if result.Width != params.TargetWidth || result.Height != params.TargetHeight {
        return fmt.Errorf("dimensions incorrect: got %dx%d, expected %dx%d",
            result.Width, result.Height, params.TargetWidth, params.TargetHeight)
    }

    // 4. 验证图像数据有效性
    img, _, err := image.Decode(bytes.NewReader(result.ImageData))
    if err != nil {
        return fmt.Errorf("invalid image data: %w", err)
    }

    // 5. 验证实际尺寸
    bounds := img.Bounds()
    if bounds.Dx() != result.Width || bounds.Dy() != result.Height {
        return fmt.Errorf("image dimensions mismatch")
    }

    return nil
}
```

**步骤2：注册到Validator**

编辑 `cmd/validator/main.go`:

```go
import (
    "custom_validator"
    "subnet/internal/validator"
)

func main() {
    // ... 现有配置 ...

    // 创建验证器注册表
    registry := validator.NewIntentTypeValidatorRegistry()

    // 注册自定义验证器
    registry.RegisterValidator("image-resize", &custom_validator.ImageResizeValidator{})
    registry.RegisterValidator("image-crop", &custom_validator.ImageCropValidator{})

    // 创建validator node
    node := validator.NewNode(config, logger)

    // 设置自定义验证器
    node.SetValidatorRegistry(registry)

    // 启动
    node.Start(ctx)
}
```

---

### Agent执行器自定义

#### 场景：图像处理Agent

Agent开发应该在 `../subnet-sdk` 中进行，但这里给出示例：

**步骤1：使用SDK创建Agent**

创建 `my-image-agent/main.go`:

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
    // 1. 解析intent数据
    var params struct {
        ImageURL     string `json:"image_url"`
        TargetWidth  int    `json:"target_width"`
        TargetHeight int    `json:"target_height"`
    }
    if err := json.Unmarshal(task.IntentData, &params); err != nil {
        return fmt.Errorf("invalid intent data: %w", err)
    }

    // 2. 下载原图
    img, err := downloadImage(params.ImageURL)
    if err != nil {
        return fmt.Errorf("failed to download image: %w", err)
    }

    // 3. Resize图像
    resizedImg := resizeImage(img, params.TargetWidth, params.TargetHeight)

    // 4. 编码为JPEG
    var buf bytes.Buffer
    if err := jpeg.Encode(&buf, resizedImg, &jpeg.Options{Quality: 90}); err != nil {
        return fmt.Errorf("failed to encode image: %w", err)
    }

    // 5. 构造result
    result := map[string]interface{}{
        "image_data": buf.Bytes(),
        "width":      params.TargetWidth,
        "height":     params.TargetHeight,
        "format":     "jpeg",
    }
    resultData, _ := json.Marshal(result)

    // 6. 提交ExecutionReport
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
    // 创建SDK client
    client := sdk.NewValidatorClient(sdk.Config{
        MatcherAddr:    "localhost:8090",
        ValidatorAddrs: []string{"localhost:9200"},
        AgentID:        "image-agent-001",
        PrivateKey:     "<AGENT_PRIVATE_KEY>",
    })

    // 创建agent
    agent := &ImageProcessingAgent{client: client}

    // 订阅任务
    ctx := context.Background()
    client.SubscribeToTasks(ctx, agent.ProcessIntent)
}
```

**步骤2：编译运行**

```bash
cd my-image-agent
go mod init my-image-agent
go get github.com/PIN-AI/subnet-sdk/go
go build -o image-agent
./image-agent
```

---

## 生产环境部署

### 安全检查清单

- [ ] 使用KMS管理私钥（不要硬编码）
- [ ] 启用TLS/mTLS for gRPC
- [ ] 配置防火墙规则
- [ ] 启用区块链参与者验证
- [ ] 配置日志收集（ELK/Prometheus）
- [ ] 设置监控告警
- [ ] 配置自动重启（systemd/Kubernetes）

### Docker部署示例

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
  matcher:
    build:
      context: .
      dockerfile: Dockerfile.matcher
    environment:
      - SUBNET_ID=${SUBNET_ID}
      - PRIVATE_KEY=${MATCHER_KEY}

  validator-1:
    build:
      context: .
      dockerfile: Dockerfile.validator
    environment:
      - SUBNET_ID=${SUBNET_ID}
      - VALIDATOR_ID=validator-1
      - PRIVATE_KEY=${VALIDATOR_1_KEY}

  validator-2:
    build:
      context: .
      dockerfile: Dockerfile.validator
    environment:
      - SUBNET_ID=${SUBNET_ID}
      - VALIDATOR_ID=validator-2
      - PRIVATE_KEY=${VALIDATOR_2_KEY}

  validator-3:
    build:
      context: .
      dockerfile: Dockerfile.validator
    environment:
      - SUBNET_ID=${SUBNET_ID}
      - VALIDATOR_ID=validator-3
      - PRIVATE_KEY=${VALIDATOR_3_KEY}
```

启动：

```bash
docker-compose up -d
```

---

## 故障排查

### 常见问题

#### 1. Validator无法达成共识

**症状：** 日志显示 "Failed to reach threshold"

**排查：**
```bash
# 检查NATS连接
docker logs <nats-container>

# 检查validator日志
tail -f data/validator-1.log | grep "signature"

# 验证validator数量
curl http://localhost:9200/validators | jq '.validators | length'
```

**解决：** 确保至少有 `threshold_num` 个validators在线

#### 2. Agent收不到任务

**症状：** Agent日志显示 "No tasks received"

**排查：**
```bash
# 检查matcher是否收到intents
curl http://localhost:8090/intents | jq '.'

# 检查agent是否连接到matcher
curl http://localhost:8090/agents | jq '.agents[] | select(.id=="my-agent")'

# 查看matcher日志
tail -f logs/matcher.log | grep "bid"
```

**解决：**
- 确认agent正确订阅了intent stream
- 确认agent提交的bid在竞价窗口内

#### 3. ValidationBundle提交失败

**症状：** Validator日志显示 "Failed to submit ValidationBundle"

**排查：**
```bash
# 检查RootLayer连接
nc -zv 3.17.208.238 9001

# 检查签名
validator-debug --verify-signature <bundle-file>

# 查看详细错误
tail -f logs/validator-1.log | grep "ValidationBundle"
```

**解决：**
- 检查RootLayer是否可达
- 验证签名密钥配置正确
- 确认Intent Manager合约地址正确

---

## 下一步

1. **阅读架构文档** - `docs/ARCHITECTURE_OVERVIEW.md`
2. **查看API文档** - `docs/API_REFERENCE.md`
3. **加入社区** - [Discord](https://discord.gg/pinai) / [GitHub Discussions](https://github.com/PIN-AI/protocol/discussions)
4. **报告问题** - [GitHub Issues](https://github.com/PIN-AI/protocol/issues)

---

## 附录

### 完整配置参考

所有配置文件模板见 `config/` 目录：
- `config/matcher-config.yaml` - Matcher配置
- `config/validator-config.yaml` - Validator配置
- `config/policy_config.yaml` - 策略配置
- `config/auth_config.yaml` - 认证配置

### 开发工具

```bash
# 生成测试intent
go run ./cmd/mock-rootlayer --generate-intent

# 验证配置文件
go run ./cmd/config-validator --config config.yaml

# 查看validator状态
curl http://localhost:9200/status | jq '.'
```

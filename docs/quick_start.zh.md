# 快速开始

本指南将帮助您快速设置和运行 PinAI Subnet 服务，用于开发和测试。

## 前置要求

- **Go 1.21+** - [安装 Go](https://go.dev/doc/install)
- **Git** - 用于克隆代码库

**注意**：验证器使用 **Raft 共识**和 **Gossip 协议**进行协调 - 不需要外部消息代理！您可以运行单个验证器用于开发，或多个验证器用于生产。

## 设置

### 1. 构建二进制文件

```bash
# 构建所有二进制文件
make build

# 这将创建：
# - bin/registry
# - bin/matcher
# - bin/validator
# - bin/simple-agent
```

### 2. 配置环境

```bash
# 复制示例环境文件
cp .env.example .env

# 编辑 .env 并设置您的测试私钥
# 重要：使用仅用于测试的密钥，不要有真实资金！
vi .env
```

`.env` 中需要的环境变量：

**测试环境示例配置：**

```bash
# 测试私钥（必需）
# ⚠️ 使用仅用于测试的密钥，不要有真实资金！
TEST_PRIVATE_KEY=your_test_private_key_here

# 测试 Subnet 配置
SUBNET_ID=0x0000000000000000000000000000000000000000000000000000000000000002

# 测试 RootLayer 端点
ROOTLAYER_GRPC=3.17.208.238:9001
ROOTLAYER_HTTP=http://3.17.208.238:8081

# 测试区块链设置（Base Sepolia 测试网）
ENABLE_CHAIN_SUBMIT=true
CHAIN_RPC_URL=https://sepolia.base.org
CHAIN_NETWORK=base_sepolia

# 测试合约地址（Base Sepolia）
INTENT_MANAGER_ADDR=0xD04d23775D3B8e028e6104E31eb0F6c07206EB46
```

**生产环境：**
将以上所有值替换为您的生产环境配置。使用安全的密钥管理系统（KMS、Vault 等）而不是在文件中存储私钥。

## 启动服务

### 方式一：一键启动（推荐）

```bash
# 使用一个命令启动所有服务
./start-subnet.sh
```

此脚本将：
- 启动 Registry 服务（gRPC: 8091, HTTP: 8101）
- 启动 Matcher 服务（gRPC: 8090）
- 启动 Validator 服务以**单节点 Raft 模式**（gRPC: 9200）
- 启动一个测试 agent 用于演示
- 生成必要的配置文件
- 保存进程 ID 以便管理

验证器会自动引导为单节点 Raft 集群用于开发。对于生产环境的多验证器设置，请参见[部署指南](subnet_deployment_guide.zh.md)。

日志保存在 `subnet-logs/` 目录。

### 方式二：手动启动

```bash
# 1. 启动 Registry
./bin/registry -grpc ":8091" -http ":8101" > subnet-logs/registry.log 2>&1 &

# 2. 启动 Matcher（需要配置文件）
cat > /tmp/matcher-config.yaml <<EOF
subnet_id: "$SUBNET_ID"
identity:
  matcher_id: "matcher-main"
  subnet_id: "$SUBNET_ID"
rootlayer:
  grpc_endpoint: "$ROOTLAYER_GRPC"
  http_endpoint: "$ROOTLAYER_HTTP"
registry:
  grpc_address: "localhost:8091"
  http_address: "http://localhost:8101"
network:
  grpc_port: 8090
enable_chain_submit: true
chain_rpc_url: "$CHAIN_RPC_URL"
chain_network: "$CHAIN_NETWORK"
intent_manager_addr: "$INTENT_MANAGER_ADDR"
private_key: "$TEST_PRIVATE_KEY"
EOF

./bin/matcher --config /tmp/matcher-config.yaml > subnet-logs/matcher.log 2>&1 &

# 3. 启动 Validator（单节点 Raft 模式）
./bin/validator \
    -id "validator-main" \
    -grpc 9200 \
    -subnet-id "$SUBNET_ID" \
    -key "$TEST_PRIVATE_KEY" \
    -rootlayer-endpoint "$ROOTLAYER_GRPC" \
    -registry-grpc "localhost:8091" \
    -registry-http "localhost:8101" \
    -chain-rpc-url "$CHAIN_RPC_URL" \
    -chain-network "$CHAIN_NETWORK" \
    -intent-manager-addr "$INTENT_MANAGER_ADDR" \
    -enable-chain-submit \
    -enable-rootlayer \
    -validators 1 \
    -threshold-num 1 \
    -threshold-denom 1 \
    -raft-enable \
    -raft-bootstrap \
    -raft-bind "127.0.0.1:7400" \
    -raft-data-dir "./data/raft" \
    -raft-peers "validator-main:127.0.0.1:7400" \
    -gossip-enable \
    -gossip-bind "127.0.0.1" \
    -gossip-port 7950 \
    > subnet-logs/validator.log 2>&1 &

# 对于多验证器设置，请参见 subnet_deployment_guide.zh.md
```

## 验证服务

检查所有服务是否正在运行：

```bash
# 检查进程
ps aux | grep -E 'registry|matcher|validator'

# 检查 Registry HTTP 端点
curl http://localhost:8101/health

# 检查日志
tail -f subnet-logs/registry.log
tail -f subnet-logs/matcher.log
tail -f subnet-logs/validator.log
```

您应该看到：
- Registry: "Registry service started on :8091 (gRPC) and :8101 (HTTP)"
- Matcher: "Matcher service started successfully"
- Validator: "Validator started, ID: validator-main"

## 发送测试 Intent

### 方式一：交互式脚本

```bash
./send-intent.sh
```

这提供了一个交互式菜单：
1. 提交自定义 Intent
2. 提交 E2E 测试 Intent
3. 提交演示 Intent
4. 查看配置
5. 退出

### 方式二：运行 E2E 测试

```bash
# 完整的端到端测试
./scripts/e2e-test.sh --no-interactive

# 或使用便捷脚本
./run-e2e.sh --no-interactive
```

E2E 测试将：
1. 向 RootLayer 提交 Intent
2. Matcher 获取并分配 Intent
3. 测试 agent 执行任务
4. Validator 验证结果
5. Validator 向 RootLayer 提交 ValidationBundle
6. 验证收据

### 方式三：手动提交 Intent

```bash
# 使用 submit-intent-signed 工具
SUBNET_ID="0x0000000000000000000000000000000000000000000000000000000000000002" \
ROOTLAYER_HTTP="http://3.17.208.238:8081/api/v1" \
INTENT_TYPE="test-intent" \
PARAMS_JSON='{"task":"My test task"}' \
AMOUNT_WEI="100000000000000" \
./bin/submit-intent-signed
```

## 查看日志

```bash
# 跟踪所有日志
tail -f subnet-logs/*.log

# 查看特定服务日志
tail -f subnet-logs/registry.log
tail -f subnet-logs/matcher.log
tail -f subnet-logs/validator.log

# 搜索错误
grep -i error subnet-logs/*.log
```

## 停止服务

### 方式一：停止脚本（优雅关闭）

```bash
./stop-subnet.sh
```

这将优雅地停止所有服务，先发送 SIGTERM，如需要再发送 SIGKILL。

### 方式二：杀死进程

```bash
pkill -f 'bin/registry'
pkill -f 'bin/matcher'
pkill -f 'bin/validator'
```

### 方式三：停止单个服务

如果使用 `start-subnet.sh`，它会创建 PID 文件：

```bash
# 停止单个服务
kill $(cat subnet-logs/registry.pid)
kill $(cat subnet-logs/matcher.pid)
kill $(cat subnet-logs/validator.pid)
```

## 故障排除

### 服务无法启动

1. **检查端口可用性**：
   ```bash
   lsof -i :8090  # Matcher
   lsof -i :8091  # Registry gRPC
   lsof -i :8101  # Registry HTTP
   lsof -i :9200  # Validator gRPC
   lsof -i :7400  # Validator Raft
   lsof -i :7950  # Validator Gossip
   ```

3. **检查环境变量**：
   ```bash
   source .env
   echo $TEST_PRIVATE_KEY
   echo $SUBNET_ID
   ```

4. **检查日志中的错误**：
   ```bash
   grep -i error subnet-logs/*.log
   ```

### Intent 提交失败

1. **检查 RootLayer 连接**：
   ```bash
   curl http://3.17.208.238:8081/health
   nc -zv 3.17.208.238 9001
   ```

2. **验证私钥格式**：
   - 在大多数地方应该是不带 `0x` 前缀的十六进制
   - 检查 `.env` 文件格式是否正确

3. **检查 Matcher 日志**：
   ```bash
   tail -f subnet-logs/matcher.log | grep -i intent
   ```

### Validator 不处理报告

1. **检查 Raft 共识**：
   ```bash
   tail -f subnet-logs/validator.log | grep -i raft
   # 查找 "entering leader state" 或 "Became Raft leader"
   ```

2. **验证 validator 已注册**：
   ```bash
   curl http://localhost:8101/validators
   ```

3. **检查共识和 gossip 状态**：
   ```bash
   tail -f subnet-logs/validator.log | grep -E "consensus|gossip|checkpoint"
   ```

## 下一步

- 阅读[架构概览](ARCHITECTURE_OVERVIEW.md)了解系统设计
- 参考[生产环境部署](production_deployment.zh.md)了解生产设置
- 探索[测试指南](testing_guide.zh.md)了解全面测试
- 查看[API 文档](api_reference.zh.md)了解集成

## 开发工作流

```bash
# 1. 修改代码
vi internal/matcher/server.go

# 2. 重新构建二进制文件
make build

# 3. 停止服务
./stop-subnet.sh

# 4. 重启服务
./start-subnet.sh

# 5. 测试更改
./send-intent.sh

# 6. 检查日志
tail -f subnet-logs/*.log
```

## 常用开发命令

```bash
# 运行测试
make test

# 使用 race detector 运行
go test -race ./...

# 生成 protobuf 代码
make proto

# 格式化代码
go fmt ./...
gofmt -w .

# Lint 代码
golangci-lint run

# 清理构建产物
make clean
```

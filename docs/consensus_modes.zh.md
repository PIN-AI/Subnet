# 共识模式指南

本文档说明如何在 Raft 和 CometBFT 两种共识模式之间选择和切换。

## 概览

PinAI Subnet 支持两种共识机制：

1. **Raft** - 默认模式，适合开发和小规模部署
2. **CometBFT** - Tendermint BFT 共识，适合生产环境

## 共识模式对比

| 特性 | Raft | CometBFT |
|------|------|----------|
| **共识算法** | Raft (leader-based) | Tendermint BFT |
| **拜占庭容错** | ❌ 不支持 | ✅ 支持 (BFT) |
| **配置复杂度** | 简单 | 较复杂 |
| **性能** | 高吞吐量 | 更强一致性保证 |
| **网络类型** | HTTP endpoints | P2P (libp2p) |
| **最少节点数** | 1 (开发) / 3 (生产) | 3 (必须) |
| **适用场景** | 开发、测试、可信环境 | 生产、不可信环境 |

## 快速启动

### 方法一：使用快捷脚本（推荐）

```bash
# 启动 Raft 模式
./start-raft.sh

# 启动 CometBFT 模式
./start-cometbft.sh
```

### 方法二：设置环境变量

```bash
# Raft 模式（默认）
export CONSENSUS_TYPE="raft"
./scripts/start-subnet.sh

# CometBFT 模式
export CONSENSUS_TYPE="cometbft"
./scripts/start-subnet.sh
```

## 详细配置

### Raft 模式配置

```bash
# 环境变量
export SUBNET_ID="0x0000000000000000000000000000000000000000000000000000000000000009"
export ROOTLAYER_GRPC="3.17.208.238:9001"
export ROOTLAYER_HTTP="http://3.17.208.238:8081"
export CONSENSUS_TYPE="raft"
export TEST_MODE=true
export START_AGENT=true

# 启动
./scripts/start-subnet.sh
```

**Raft 模式特点：**
- 自动 leader 选举
- HTTP-based 通信
- 简单的配置
- 适合开发环境

**端口分配（3个验证器）：**
- Validator 1: gRPC=9090, Raft=7400
- Validator 2: gRPC=9100, Raft=7401
- Validator 3: gRPC=9110, Raft=7402

### CometBFT 模式配置

```bash
# 环境变量
export SUBNET_ID="0x0000000000000000000000000000000000000000000000000000000000000009"
export ROOTLAYER_GRPC="3.17.208.238:9001"
export ROOTLAYER_HTTP="http://3.17.208.238:8081"
export CONSENSUS_TYPE="cometbft"
export TEST_MODE=true
export START_AGENT=true

# 启动
./scripts/start-subnet.sh
```

**CometBFT 模式特点：**
- Tendermint BFT 共识
- P2P 网络发现
- 拜占庭容错
- 适合生产环境

**端口分配（3个验证器）：**
- Validator 1: gRPC=9090, CometBFT RPC=26657, P2P=26656
- Validator 2: gRPC=9100, CometBFT RPC=26667, P2P=26666
- Validator 3: gRPC=9110, CometBFT RPC=26677, P2P=26676

## 切换共识模式

### 从 Raft 切换到 CometBFT

1. 停止当前服务：
```bash
pkill -f 'bin/matcher|bin/validator|bin/registry|bin/simple-agent'
```

2. 清理数据：
```bash
rm -rf subnet-logs test-cometbft
```

3. 启动 CometBFT 模式：
```bash
./start-cometbft.sh
```

### 从 CometBFT 切换到 Raft

1. 停止当前服务：
```bash
pkill -f 'bin/matcher|bin/validator|bin/registry|bin/simple-agent'
```

2. 清理数据：
```bash
rm -rf subnet-logs test-cometbft
```

3. 启动 Raft 模式：
```bash
./start-raft.sh
```

**注意**：切换共识模式会清空所有本地状态！

## ValidationBundle 工作流程

### Raft 模式下的 ValidationBundle

1. Agent 执行任务并提交 ExecutionReport
2. Validator 接收并验证 ExecutionReport
3. ExecutionReport 通过 Raft 复制到所有验证器
4. Leader 创建 checkpoint
5. 验证器对 checkpoint 签名
6. Gossip 协议收集签名
7. 达到阈值后，Leader 提交 ValidationBundle 到 RootLayer

### CometBFT 模式下的 ValidationBundle

1. Agent 执行任务并提交 ExecutionReport
2. Validator 接收 ExecutionReport 并提交到 CometBFT mempool
3. ExecutionReport 通过 ABCI 进入 CometBFT 状态机
4. CometBFT 共识确认 ExecutionReport
5. Leader 创建 checkpoint 并提交到 CometBFT
6. 验证器在 ExtendVote 阶段对 checkpoint 签名（ECDSA）
7. CometBFT 在 PrepareProposal 阶段收集签名
8. 达到阈值后，提交 ValidationBundle 到 RootLayer（每个 Intent 一个 ValidationBundle）

**关键区别**：
- CometBFT 使用 vote extensions 收集 ECDSA 签名
- CometBFT 为每个 Intent 提交单独的 ValidationBundle（而不是每个 checkpoint 一个）
- CometBFT 的签名收集是同步的，集成在共识流程中

## 调试和监控

### 查看 Raft 状态

```bash
# 查看 leader 选举
grep -i "leader\|raft" subnet-logs/validator1.log

# 检查 Raft 端口
lsof -i :7400
```

### 查看 CometBFT 状态

```bash
# 查看 CometBFT 状态
curl http://localhost:26657/status | jq

# 查看共识日志
grep -i "consensus\|commit" subnet-logs/validator1.log

# 查看 P2P 连接
curl http://localhost:26657/net_info | jq
```

### 查看 ValidationBundle 提交

```bash
# 查看提交状态
grep -i "ValidationBundle\|Successfully submitted" subnet-logs/validator1.log

# 查看错误
grep -i "Failed to submit\|error" subnet-logs/validator1.log
```

## 常见问题

### Q: ValidationBundle 提交失败 "assignment not found"

**原因**：Assignment 还未在 blockchain 上确认。

**解决方案**：
- 这是正常的！Matcher 和 Validator 都有重试机制
- Matcher 会重试 3 次（15s/30s/45s 间隔）
- 等待 assignment 确认后会自动成功

### Q: CometBFT 节点无法连接

**原因**：P2P 端口冲突或防火墙阻止。

**解决方案**：
```bash
# 检查端口占用
lsof -i :26656
lsof -i :26657

# 检查 node ID
curl http://localhost:26657/status | jq '.result.node_info.id'
```

### Q: Raft leader 选举失败

**原因**：验证器无法互相通信。

**解决方案**：
```bash
# 检查 Raft 端口
lsof -i :7400
lsof -i :7401
lsof -i :7402

# 检查验证器配置
grep -i "raft" subnet-logs/validator*.log
```

### Q: 如何知道当前使用哪种共识？

```bash
# 查看日志
grep -i "consensus\|raft\|cometbft" subnet-logs/validator1.log | head -5

# Raft 会显示：
# "Raft consensus enabled"
# "Became Raft leader"

# CometBFT 会显示：
# "Starting CometBFT node"
# "Starting ABCI application"
```

## 性能建议

### Raft 模式
- 开发：1 个验证器
- 测试：3 个验证器
- 生产：3-5 个验证器（奇数）

### CometBFT 模式
- 最少：3 个验证器（2f+1，f=1）
- 推荐：4 个验证器（容忍 1 个故障）
- 生产：7 个验证器（容忍 2 个故障）

## 下一步

- 查看 [部署指南](subnet_deployment_guide.zh.md) 了解生产环境配置
- 查看 [E2E 测试指南](e2e_test_guide.md) 了解如何测试
- 查看 [脚本指南](scripts_guide.zh.md) 了解所有可用脚本

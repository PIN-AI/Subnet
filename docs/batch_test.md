  # 批量操作E2E测试指南

## 概述

批量操作E2E测试验证整个批量提交流程：

```
批量Intent提交 → Matcher拉取 → Agent批量投标 →
Matcher匹配 → Agent执行任务 → Agent批量提交报告 →
Validator验证 → ValidationBundle
```

## 核心组件

### 1. batch-submit-intents
批量Intent提交工具，可并行提交多个intents到RootLayer。

**功能：**
- ✅ 并行提交多个intents
- ✅ 自动生成唯一的intent IDs
- ✅ 性能统计（成功率、平均时间等）
- ✅ 详细的提交报告

### 2. batch-test-agent
支持批量操作的测试Agent。

**功能：**
- ✅ 批量提交bids
- ✅ 批量执行任务
- ✅ 批量提交execution reports
- ✅ 可配置批量大小和等待时间
- ✅ 后台批量worker

### 3. batch-e2e-test.sh
完整的批量操作E2E测试脚本。

**功能：**
- ✅ 自动启动Matcher和Validators
- ✅ 批量提交intents
- ✅ 启动批量agent
- ✅ 监控整个流程
- ✅ 生成测试报告

## 快速开始

### 前置要求

```bash
# 1. 确保已构建所有二进制
cd /Users/ty/pinai/protocol/Subnet
make build

# 2. 设置测试私钥（必须）
export TEST_PRIVATE_KEY="your_test_private_key_here"

# 3. 确保NATS运行
nats-server &
# 或
docker run -d --name nats -p 4222:4222 nats:latest
```

### 运行批量E2E测试

**方式一：使用快捷脚本（推荐）**

```bash
# 使用默认配置（5个intents，批量大小5）
./run-batch-e2e.sh --no-interactive

# 自定义配置
./run-batch-e2e.sh \
    --intent-count 10 \
    --batch-size 5 \
    --batch-wait 15s \
    --no-interactive
```

**方式二：直接运行测试脚本**

```bash
./scripts/batch-e2e-test.sh \
    --intent-count 10 \
    --batch-size 5 \
    --no-interactive
```

## 测试流程详解

### 第0步：准备环境

脚本会自动检查：
- ✅ 所需二进制文件是否存在
- ✅ NATS是否运行（如果没有会自动启动Docker容器）
- ✅ TEST_PRIVATE_KEY是否设置

### 第1步：构建批量测试二进制

```bash
# 构建batch-submit-intents
go build -o bin/batch-submit-intents ./cmd/batch-submit-intents

# 构建batch-test-agent
go build -o bin/batch-test-agent ./cmd/batch-test-agent
```

### 第2步：启动Subnet服务

- **Matcher**: 监听端口8090，连接RootLayer拉取intents
- **Validator**: 监听端口9200，接收execution reports

### 第3步：批量提交Intents

使用 `batch-submit-intents` 工具并行提交多个intents：

```bash
RPC_URL="https://sepolia.base.org" \
PRIVATE_KEY="your_key" \
SUBNET_ID="0x..." \
./bin/batch-submit-intents \
    --count 5 \
    --type "batch-test-intent" \
    --params '{"task":"Batch E2E test"}'
```

**输出示例：**
```
📦 Submitting 5 intents in parallel...
  [0] Submitting intent 0xabcd...
  [1] Submitting intent 0xdef0...
  [2] Submitting intent 0x1234...
  [3] Submitting intent 0x5678...
  [4] Submitting intent 0x9abc...

  [0] ✅ Submitted (tx: 0x7890..., duration: 2.3s)
  [1] ✅ Submitted (tx: 0xabcd..., duration: 2.5s)
  [2] ✅ Submitted (tx: 0xef01..., duration: 2.1s)
  [3] ✅ Submitted (tx: 0x2345..., duration: 2.4s)
  [4] ✅ Submitted (tx: 0x6789..., duration: 2.2s)

============================================================
Batch Intent Submission Summary
============================================================

Total Intents:   5
✅ Successful:    5 (100.0%)
❌ Failed:        0 (0.0%)

⏱️  Total Time:    11.5s
⏱️  Average Time:  2.3s
⏱️  Rate:          0.43 intents/sec
```

### 第4步：启动批量Agent

Agent以批量模式启动：

```bash
./bin/batch-test-agent \
    --matcher "localhost:8090" \
    --subnet-id "0x..." \
    --batch true \
    --batch-size 5 \
    --batch-wait 10s
```

**Agent行为：**
1. 订阅Matcher的intents流
2. 收集intents直到达到批量大小或超时
3. 批量提交bids
4. 执行任务
5. 批量提交execution reports（每10秒或达到5个）

### 第5步：监控批量操作

脚本会监控以下关键事件：

```
[12:34:56] ➜ Step 1: Waiting for Matcher to pull intents...
[12:34:59] ✓ Matcher received 5 intents

[12:35:01] ➜ Step 2: Waiting for batch bid submissions...
[12:35:15] 📦 Found 1 batch bid submissions

[12:35:17] ➜ Step 3: Waiting for task assignments...
[12:35:30] ✓ Agent received 5 task assignments

[12:35:32] ➜ Step 4: Waiting for batch execution report submissions...
[12:35:45] 📦 Found 1 batch execution report submissions

[12:35:47] ➜ Step 5: Checking validator processing...
[12:35:52] ✓ Validator processed 5 execution reports
```

### 第6步：生成测试报告

```
================================
Batch E2E Test Results
================================

Test Configuration:
  Intent Count:   5
  Batch Size:     5
  Batch Wait:     10s

Service Status:
  Matcher:        localhost:8090
  Validator:      localhost:9200

Statistics:
  Intents Received:        5
  Tasks Executed:          5
  Reports Processed:       5
  Batch Bid Submissions:   1
  Batch Report Submissions: 1

✅ Batch E2E Test PASSED
```

## 自定义测试

### 测试大批量（100个intents）

```bash
./run-batch-e2e.sh \
    --intent-count 100 \
    --batch-size 20 \
    --batch-wait 30s \
    --no-interactive
```

### 测试小批量快速提交

```bash
./run-batch-e2e.sh \
    --intent-count 3 \
    --batch-size 3 \
    --batch-wait 5s \
    --no-interactive
```

### 仅批量提交Intents（不运行完整E2E）

```bash
cd /Users/ty/pinai/protocol/Subnet

# 先构建工具
go build -o bin/batch-submit-intents ./cmd/batch-submit-intents

# 批量提交
RPC_URL="https://sepolia.base.org" \
PRIVATE_KEY="$TEST_PRIVATE_KEY" \
SUBNET_ID="0x0000000000000000000000000000000000000000000000000000000000000002" \
./bin/batch-submit-intents \
    --count 10 \
    --type "my-custom-intent" \
    --params '{"task":"Custom batch test"}' \
    --network base_sepolia
```

### 仅运行批量Agent（连接已有Matcher）

```bash
cd /Users/ty/pinai/protocol/Subnet

# 构建agent
go build -o bin/batch-test-agent ./cmd/batch-test-agent

# 运行
./bin/batch-test-agent \
    --matcher "localhost:8090" \
    --subnet-id "0x0000000000000000000000000000000000000000000000000000000000000002" \
    --batch true \
    --batch-size 10 \
    --batch-wait 15s \
    --name "MyBatchAgent"
```

## 查看日志

测试运行时，日志保存在 `batch-e2e-logs/` 目录：

```bash
# 查看Matcher日志
tail -f batch-e2e-logs/matcher.log

# 查看Validator日志
tail -f batch-e2e-logs/validator.log

# 查看批量Agent日志
tail -f batch-e2e-logs/batch-agent.log

# 查看批量提交日志
cat batch-e2e-logs/batch-submit.log

# 搜索批量操作
grep -i "batch" batch-e2e-logs/*.log
```

## 故障排查

### 问题1：批量提交失败

**错误：** `Failed to build batch-submit-intents`

**解决：**
```bash
# 确保Go依赖已安装
cd /Users/ty/pinai/protocol/Subnet
go mod download
go mod tidy

# 重新构建
make build
```

### 问题2：Intents未收到

**错误：** `Matcher received 0 intents`

**检查：**
```bash
# 1. 检查RootLayer连接
nc -zv 3.17.208.238 9001

# 2. 检查matcher日志
grep -i "rootlayer\|pull\|intent" batch-e2e-logs/matcher.log

# 3. 确认intents已提交到链上
# 查看batch-submit.log确认提交成功
```

### 问题3：批量操作未触发

**错误：** `Found 0 batch bid submissions`

**检查：**
```bash
# 1. 确认batch-test-agent以批量模式运行
grep "Batching: true" batch-e2e-logs/batch-agent.log

# 2. 检查batch worker是否启动
grep "Started batch worker" batch-e2e-logs/batch-agent.log

# 3. 查看pending results
grep "Pending Reports" batch-e2e-logs/batch-agent.log
```

### 问题4：NATS连接失败

**错误：** `Failed to connect to NATS`

**解决：**
```bash
# 检查NATS是否运行
ps aux | grep nats-server
lsof -i :4222

# 手动启动NATS
nats-server &

# 或使用Docker
docker run -d --name nats -p 4222:4222 nats:latest
```

## 性能基准

基于测试环境的预期性能：

| Intent数量 | 批量大小 | 预期耗时 | Intents/秒 |
|-----------|---------|---------|-----------|
| 5         | 5       | ~30s    | 0.17      |
| 10        | 5       | ~45s    | 0.22      |
| 20        | 10      | ~60s    | 0.33      |
| 50        | 20      | ~120s   | 0.42      |
| 100       | 20      | ~240s   | 0.42      |

**注意：** 实际性能取决于：
- RootLayer响应时间
- 区块链确认时间
- Matcher处理速度
- Validator共识速度

## 下一步

1. **性能测试** - 测试1000+ intents的批量处理
2. **压力测试** - 同时运行多个batch-test-agent
3. **故障恢复测试** - 测试批量操作失败时的重试机制
4. **集成测试** - 与真实Agent集成测试

## 相关文档

- [Subnet部署指南](subnet_deployment_guide.zh.md)
- [E2E测试指南](e2e_test_guide.md)
- [Python SDK批量操作](../../subnet-sdk/python/examples/README_BATCH.md)

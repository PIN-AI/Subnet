# 脚本使用指南

本目录包含用于 Subnet 开发、测试和部署的实用工具脚本。

## 概述

`scripts/` 目录提供了使用 PinAI Subnet 的核心工具:

- **区块链操作**: 创建子网并在链上注册参与者
- **测试与验证**: 端到端测试和测试代理
- **服务管理**: 注册中心 CLI 和监控工具
- **实用工具**: 密钥派生和意图提交工具

## 脚本参考

### 1. create-subnet.sh

**用途**: 在区块链上创建新的子网

**位置**: `scripts/create-subnet.sh` + `scripts/create-subnet.go`

**描述**:
此脚本使用 intent-protocol-contract-sdk 将新子网部署到区块链。它处理子网工厂交互、质押治理配置,并保存子网信息供后续使用。

**用法**:
```bash
# 使用默认配置
./scripts/create-subnet.sh

# 自定义子网名称
./scripts/create-subnet.sh --name "生产环境子网"

# 手动审批参与者
./scripts/create-subnet.sh --auto-approve false

# 完整配置
./scripts/create-subnet.sh \
  --rpc https://sepolia.base.org \
  --key 0x你的私钥 \
  --name "我的测试子网" \
  --auto-approve true
```

**选项**:
- `--config FILE` - 配置文件路径(默认: config/config.yaml)
- `--network NAME` - 网络名称(默认: base_sepolia)
- `--rpc URL` - RPC URL(覆盖配置)
- `--key HEX` - 私钥十六进制(覆盖配置)
- `--name NAME` - 子网名称(默认: "My Test Subnet")
- `--auto-approve BOOL` - 自动批准参与者(默认: true)
- `--help` - 显示帮助信息

**环境变量**:
- `NETWORK` - 网络名称
- `RPC_URL` - RPC URL
- `PRIVATE_KEY` - 私钥十六进制
- `SUBNET_NAME` - 子网名称

**输出**:
- 在区块链上创建子网
- 显示子网 ID 和合约地址
- 保存配置供验证器/匹配器设置使用

---

### 2. register.sh

**用途**: 在子网上注册验证器、匹配器和代理

**位置**: `scripts/register.sh` + `scripts/register-participants.go`

**描述**:
此脚本处理在现有子网上注册参与者。支持注册所有三种参与者类型(验证器、匹配器、代理),可配置质押金额、端点和元数据。脚本可以检查当前注册状态、执行演练运行,以及选择性地注册特定参与者类型。

**用法**:
```bash
# 使用配置文件注册所有参与者
./scripts/register.sh

# 仅检查注册状态
./scripts/register.sh --check

# 使用自定义参数注册
./scripts/register.sh \
  --rpc https://sepolia.base.org \
  --subnet 0x123... \
  --key 0xabc... \
  --domain my-subnet.com

# 演练运行查看会发生什么
./scripts/register.sh --dry-run

# 仅注册验证器
./scripts/register.sh --skip-matcher --skip-agent
```

**选项**:
- `--config FILE` - 配置文件路径(默认: config/config.yaml)
- `--network NAME` - 网络名称(默认: base_sepolia)
- `--rpc URL` - RPC URL(覆盖配置)
- `--subnet ADDRESS` - 子网合约地址(覆盖配置)
- `--key HEX` - 私钥十六进制(覆盖配置)
- `--domain DOMAIN` - 参与者域名(默认: subnet.example.com)
- `--validator-port PORT` - 验证器端点端口(默认: 9090)
- `--matcher-port PORT` - 匹配器端点端口(默认: 8090)
- `--agent-port PORT` - 代理端点端口(默认: 7070)
- `--validator-stake AMOUNT` - 验证器质押 ETH 数量(默认: 0.1)
- `--matcher-stake AMOUNT` - 匹配器质押 ETH 数量(默认: 0.05)
- `--agent-stake AMOUNT` - 代理质押 ETH 数量(默认: 0.05)
- `--check` - 仅检查注册状态
- `--dry-run` - 演练运行(不提交交易)
- `--skip-validator` - 跳过验证器注册
- `--skip-matcher` - 跳过匹配器注册
- `--skip-agent` - 跳过代理注册
- `--erc20` - 使用 ERC20 质押而非 ETH
- `--metadata URI` - 元数据 URI(可选)

**功能特性**:
- 查询子网信息和质押要求
- 检查每种参与者类型的当前注册状态
- 验证质押金额是否满足子网最低要求
- 支持 ETH 和 ERC20 质押
- 显示交易哈希和确认状态
- 根据子网配置自动批准或需要所有者批准

**示例输出**:
```
🚀 开始参与者注册脚本
   网络: base_sepolia
   子网合约: 0x123...
   签名者地址: 0xabc...
   余额: 1.234567 ETH

📊 质押要求:
   最低验证器质押: 0.100000 ETH
   最低匹配器质押: 0.050000 ETH
   最低代理质押: 0.050000 ETH
   自动批准: true

🔍 检查当前注册状态...
   ❌ 未注册为验证器
   ❌ 未注册为匹配器
   ❌ 未注册为代理

📝 注册为验证器...
   域名: subnet.example.com
   端点: https://subnet.example.com:9090
   质押: 0.100000 ETH
   📤 交易已提交: 0xtx123...
   ✅ 验证器注册完成

[... 匹配器和代理的类似输出 ...]

🎉 注册流程完成!
```

---

### 3. registry-cli.sh

**用途**: 与注册中心服务交互的命令行接口

**位置**: `scripts/registry-cli.sh`

**描述**:
此工具提供了查询和监控注册中心服务的 CLI,注册中心维护活动代理和验证器列表。支持列出参与者、获取详情、发送心跳和实时监控。

**用法**:
```bash
# 列出所有已注册的验证器
./scripts/registry-cli.sh list-validators

# 列出所有已注册的代理
./scripts/registry-cli.sh list-agents

# 获取特定验证器的详情
./scripts/registry-cli.sh validator validator-1

# 获取特定代理的详情
./scripts/registry-cli.sh agent agent-001

# 为验证器发送心跳
./scripts/registry-cli.sh validator-heartbeat validator-1

# 监控验证器(每2秒更新)
./scripts/registry-cli.sh watch-validators

# 监控所有服务
./scripts/registry-cli.sh watch-all

# 检查注册中心健康状况
./scripts/registry-cli.sh health
```

**命令**:
- `list-agents` - 列出所有已注册的代理
- `list-validators` - 列出所有已注册的验证器
- `agent <id>` - 获取特定代理的详情
- `validator <id>` - 获取特定验证器的详情
- `agent-heartbeat <id>` - 为代理发送心跳
- `validator-heartbeat <id>` - 为验证器发送心跳
- `watch-agents` - 监控代理数量(每2秒更新)
- `watch-validators` - 监控验证器数量(每2秒更新)
- `watch-all` - 监控所有服务(每2秒更新)
- `health` - 检查注册中心健康状况

**环境变量**:
- `REGISTRY_URL` - 注册中心端点(默认: http://localhost:8092)

**依赖**:
- `curl` - HTTP 请求必需
- `jq` - 可选,用于格式化 JSON 输出

**示例输出**:
```bash
$ ./scripts/registry-cli.sh list-validators
正在从 http://localhost:8092 获取验证器
{
  "validators": [
    {
      "id": "validator-1",
      "endpoint": "localhost:9090",
      "status": "active",
      "last_heartbeat": "2025-10-14T10:30:45Z"
    }
  ]
}

$ ./scripts/registry-cli.sh watch-all
=== 注册中心状态 2025年10月14日 10:30:50 ===

验证器:
  数量: 4
  - validator-1 [active] localhost:9200
  - validator-2 [active] localhost:9201
  - validator-3 [active] localhost:9202
  - validator-4 [active] localhost:9203

代理:
  数量: 2
  - agent-001 [active] 能力: code-execution, data-analysis
  - agent-002 [active] 能力: image-generation
```

---

### 4. derive-pubkey.go

**用途**: 从以太坊私钥派生公钥

**位置**: `scripts/derive-pubkey.go`

**描述**:
简单的实用工具,从以太坊私钥派生未压缩的公钥。用于验证器配置和调试身份验证问题。

**用法**:
```bash
# 编译并运行
go run scripts/derive-pubkey.go <私钥十六进制>

# 或先编译
go build -o bin/derive-pubkey scripts/derive-pubkey.go
./bin/derive-pubkey 0x你的私钥
```

**示例**:
```bash
$ go run scripts/derive-pubkey.go 0xabc123...
0482ea12c5481d481c7f9d7c1a2047401c6e2f855e4cee4d8df0aa197514f3456528ba6c55092b20b51478fd8cf62cde37f206621b3dd47c2be3d5c35e4889bf94
```

**输出格式**:
返回十六进制格式的未压缩公钥(130个字符, 65字节)。

---

### 5. e2e-test.sh

**用途**: 完整的端到端集成测试,测试整个子网流程

**位置**: `scripts/e2e-test.sh`

**描述**:
全面的 E2E 测试脚本,模拟完整的意图生命周期:
1. 将意图提交到 RootLayer(双重提交: 区块链 + RootLayer HTTP)
2. 匹配器从 RootLayer gRPC 拉取意图
3. 匹配器开启竞标窗口
4. 测试代理通过 SDK 提交出价
5. 匹配器关闭竞标并创建分配
6. 代理执行任务并向验证器提交执行报告
7. 验证器验证报告并广播给所有验证器
8. 验证器收集签名并达到阈值
9. 领导验证器构建验证包
10. 验证包提交到 RootLayer(HTTP API 和区块链)

脚本启动所有必需的服务(匹配器、验证器、测试代理),监控日志进度,并提供详细的状态更新。

**用法**:
```bash
# 使用真实 RootLayer 运行 E2E 测试
export SUBNET_ID="0x0000000000000000000000000000000000000000000000000000000000000002"
export ROOTLAYER_GRPC="3.17.208.238:9001"
export ROOTLAYER_HTTP="http://3.17.208.238:8081"
./scripts/start-subnet.sh

# 非交互式运行(CI/CD 模式)
./scripts/start-subnet.sh --no-interactive

# 启用区块链提交
export ENABLE_CHAIN_SUBMIT=true
./scripts/start-subnet.sh

# 自定义 RootLayer 端点
./scripts/start-subnet.sh \
  --rootlayer-grpc 3.17.208.238:9001 \
  --rootlayer-http http://3.17.208.238:8081
```

**选项**:
- `--rootlayer-grpc <addr>` - RootLayer gRPC 端点(默认: 3.17.208.238:9001)
- `--rootlayer-http <url>` - RootLayer HTTP 端点(默认: http://3.17.208.238:8081)
- `--no-interactive` - 运行测试后退出,不进入交互模式
- `--help, -h` - 显示帮助信息

**环境变量**:
- `ROOTLAYER_GRPC` - --rootlayer-grpc 的替代方式
- `ROOTLAYER_HTTP` - --rootlayer-http 的替代方式
- `SUBNET_ID` - 用于测试的子网 ID
- `ENABLE_CHAIN_SUBMIT` - 启用区块链提交(true/false,默认: false)
- `CHAIN_RPC_URL` - 区块链 RPC URL(默认: https://sepolia.base.org)
- `CHAIN_NETWORK` - 网络名称(默认: base_sepolia)
- `INTENT_MANAGER_ADDR` - IntentManager 合约地址
- `MATCHER_PRIVATE_KEY` - 匹配器私钥(生产环境请勿使用!)

**启动的服务**:
- 匹配器(端口 8090)
- 验证器 1(端口 9200) - 单验证器模式以加快测试
- 测试代理(连接到匹配器和验证器)

**测试流程**:
```
1. 提交意图 → RootLayer(双重: 区块链交易 + HTTP API)
2. 匹配器拉取意图 ← RootLayer gRPC
3. 匹配器开启竞标窗口(10秒)
4. 代理向匹配器提交出价
5. 匹配器关闭窗口 → 创建分配
6. 代理接收分配
7. 代理执行任务
8. 代理提交执行报告 → 验证器
9. 验证器验证并广播报告
10. 验证器收集签名(阈值签名)
11. 领导者构建验证包
12. 验证包 → RootLayer(HTTP API + 区块链)
```

**交互式命令**(如未使用 --no-interactive):
```
> logs matcher          - 显示匹配器日志
> logs validator-1      - 显示验证器日志
> logs agent           - 显示测试代理日志
> stats                - 显示快速统计
> quit                 - 停止服务并退出
```

**日志文件**:
- `e2e-test-logs/matcher.log` - 匹配器服务日志
- `e2e-test-logs/validator-1.log` - 验证器日志
- `e2e-test-logs/test-agent.log` - 测试代理日志
- `e2e-test-logs/pids.txt` - 进程 ID 用于清理

**前置条件**:
- 所有二进制文件已构建(`make build`)
- RootLayer 在配置的端点可访问

**退出码**:
- `0` - 测试通过,验证包成功提交
- `1` - 测试失败或服务启动失败

---

### 6. submit-intent-signed.go

**用途**: 提交带签名的意图,使用双重提交(区块链 + RootLayer HTTP)

**位置**: `scripts/submit-intent-signed.go`

**描述**:
高级意图提交工具,实现了正确的 EIP-191 签名创建和双重提交策略。它首先将意图提交到区块链(IntentManager 合约),然后将相同的意图提交到 RootLayer HTTP API 并进行签名验证。这确保了链上记录和快速的链下传播。

**用法**:
```bash
# 构建工具
go build -o bin/submit-intent-signed scripts/submit-intent-signed.go

# 通过环境变量完整配置提交
export PIN_BASE_SEPOLIA_INTENT_MANAGER="0xD04d23775D3B8e028e6104E31eb0F6c07206EB46"
export RPC_URL="https://sepolia.base.org"
export PRIVATE_KEY="0x你的私钥"
export PIN_NETWORK="base_sepolia"
export ROOTLAYER_HTTP="http://3.17.208.238:8081/api/v1"
export SUBNET_ID="0x0000000000000000000000000000000000000000000000000000000000000002"
export INTENT_TYPE="my-task"
export PARAMS_JSON='{"task":"处理这些数据","priority":"high"}'
export AMOUNT_WEI="100000000000000"

./bin/submit-intent-signed
```

**环境变量**(全部必需):
- `PIN_BASE_SEPOLIA_INTENT_MANAGER` - IntentManager 合约地址
- `RPC_URL` - 区块链 RPC URL
- `PRIVATE_KEY` - 提交者的私钥(可选 0x 前缀)
- `PIN_NETWORK` - 网络名称(base_sepolia, base, local)
- `ROOTLAYER_HTTP` - RootLayer HTTP API 基础 URL
- `SUBNET_ID` - 目标子网 ID(32字节十六进制)
- `INTENT_TYPE` - 意图类型标识符
- `PARAMS_JSON` - 意图参数 JSON 字符串
- `AMOUNT_WEI` - 预算金额(Wei)

**功能特性**:
- **EIP-191 签名**: 创建正确的以太坊签名消息
- **双重提交**: 区块链交易 + RootLayer HTTP API
- **本地验证**: 提交前在本地验证签名
- **签名格式**: 使用无填充的 base64url 编码
- **正确的哈希**: 使用 Keccak256(PARAMS_JSON) 进行签名
- **交易追踪**: 返回区块链交易哈希和意图 ID

**示例输出**:
```
🚀 使用双重提交提交意图(区块链 + RootLayer)

📋 意图详情:
   子网 ID: 0x0000...0002
   意图类型: e2e-test
   预算: 100000000000000 wei (0.0001 ETH)
   RootLayer HTTP: http://3.17.208.238:8081/api/v1

🔐 创建 EIP-191 签名...
   提交者: 0xfc5A111b714547fc2D1D796EAAbb68264ed4A132
   参数哈希: 0x5f3d8aa4...
   签名: vyEpmvTKK-Bhze...

✓ 本地签名验证通过

📤 步骤 1: 提交到区块链(IntentManager)...
   合约: 0xD04d23775D3B8e028e6104E31eb0F6c07206EB46
   交易: 0x7b3f9e1d...
   ⏳ 等待确认...
   ✅ 区块链交易已确认!

📤 步骤 2: 提交到 RootLayer HTTP API...
   端点: http://3.17.208.238:8081/api/v1/intents
   ✅ RootLayer 接受意图

✅ 双重提交成功完成!
   意图 ID: intent_abc123...
   区块链交易: 0x7b3f9e1d...

意图将:
1. 记录在 IntentManager 合约的链上
2. 通过 RootLayer gRPC 对匹配器可用
```

**签名详情**:
- 消息格式: `0x19Ethereum Signed Message:\n32` + keccak256(PARAMS_JSON)
- 编码: 无填充的 base64url(RFC 4648 第5节)
- 恢复: V 值针对以太坊调整(27/28)

---

### 7. test-agent/

**用途**: 用于 E2E 测试的测试代理实现

**位置**: `scripts/test-agent/`

**描述**:
包含一个简单的测试代理(`validator_test_agent.go`),实现了完整的代理工作流:
- 向匹配器注册
- 为意图提交出价
- 接收分配
- 执行任务(模拟)
- 向验证器提交执行报告
- 处理收据

这被 `e2e-test.sh` 用于在测试期间模拟真实代理。

**文件**:
- `validator_test_agent.go` - 主测试代理实现
- `test-agent` - 编译的二进制文件(由 e2e-test.sh 创建)

**用法**:
测试代理由 `e2e-test.sh` 自动构建和启动。也可以手动运行:

```bash
cd scripts/test-agent
go build -o test-agent validator_test_agent.go

./test-agent \
  --agent-id "test-agent-001" \
  --matcher "localhost:8090" \
  --validator "localhost:9200" \
  --subnet-id "0x1111..."
```

---

### 8. intent-test/

**用途**: 遗留的意图测试实用程序

**位置**: `scripts/intent-test/`

**描述**:
包含较旧的意图提交和测试实用程序。大部分功能已被 `submit-intent-signed.go` 和 E2E 测试脚本取代。

**文件**:
- 各种测试脚本和意图提交实用程序

**状态**: 遗留 - 使用 `submit-intent-signed.go` 和 `e2e-test.sh` 进行当前测试

---

## 常见工作流程

### 初始子网设置

1. **创建子网**:
   ```bash
   ./scripts/create-subnet.sh --name "我的子网" --auto-approve true
   # 记录输出中的子网 ID
   ```

2. **注册参与者**:
   ```bash
   export SUBNET_CONTRACT="0x你的子网地址"
   ./scripts/register.sh --subnet $SUBNET_CONTRACT
   ```

3. **验证注册**:
   ```bash
   ./scripts/register.sh --check
   ```

### 开发与测试

1. **构建所有组件**:
   ```bash
   make build
   ```

2. **运行 E2E 测试**:
   ```bash
   export SUBNET_ID="0x你的子网ID"
   export ROOTLAYER_GRPC="localhost:9001"
   export ROOTLAYER_HTTP="http://localhost:8080"
   ./scripts/start-subnet.sh
   ```

3. **监控服务**:
   ```bash
   # 在不同的终端
   tail -f e2e-test-logs/matcher.log
   tail -f e2e-test-logs/validator-1.log
   tail -f e2e-test-logs/test-agent.log
   ```

### 生产部署

1. **检查注册状态**:
   ```bash
   ./scripts/registry-cli.sh health
   ./scripts/registry-cli.sh list-validators
   ./scripts/registry-cli.sh list-agents
   ```

2. **监控注册中心**:
   ```bash
   ./scripts/registry-cli.sh watch-all
   ```

3. **提交生产意图**:
   ```bash
   export SUBNET_ID="0x生产子网ID"
   export INTENT_TYPE="production-task"
   export PARAMS_JSON='{"task":"真实工作负载","priority":"high"}'
   ./bin/submit-intent-signed
   ```

---

## 环境变量参考

### 通用变量

- `RPC_URL` - 区块链 RPC 端点
- `PRIVATE_KEY` - 以太坊私钥(可带或不带 0x 前缀)
- `SUBNET_ID` - 目标子网 ID(32字节, 0x前缀)
- `PIN_NETWORK` - 网络名称(base_sepolia, base, local)

### RootLayer 配置

- `ROOTLAYER_GRPC` - RootLayer gRPC 端点(主机:端口)
- `ROOTLAYER_HTTP` - RootLayer HTTP API 基础 URL

### 区块链合约

- `PIN_BASE_SEPOLIA_INTENT_MANAGER` - IntentManager 合约地址
- `PIN_BASE_SEPOLIA_SUBNET_FACTORY` - SubnetFactory 合约地址
- `PIN_BASE_SEPOLIA_STAKING_MANAGER` - StakingManager 合约地址
- `PIN_BASE_SEPOLIA_CHECKPOINT_MANAGER` - CheckpointManager 合约地址

### 注册中心服务

- `REGISTRY_URL` - 注册中心 HTTP 端点(默认: http://localhost:8092)

### E2E 测试配置

- `ENABLE_CHAIN_SUBMIT` - 启用区块链提交(true/false)
- `CHAIN_RPC_URL` - E2E 测试的区块链 RPC
- `CHAIN_NETWORK` - E2E 测试的网络名称
- `MATCHER_PRIVATE_KEY` - 测试匹配器的私钥(生产环境请勿使用!)

---

## 安全注意事项

### 私钥

- **永远不要将私钥提交**到版本控制
- 使用环境变量或安全配置管理
- 对于测试,使用专门的测试账户和最少的资金
- E2E 测试中的 `MATCHER_PRIVATE_KEY` 仅用于本地测试

### 配置文件

- 配置文件可能包含敏感数据(私钥)
- 使用限制性文件权限: `chmod 600 config/config.yaml`
- 对于生产环境,考虑使用环境变量而非配置文件

### 注册中心服务

- 默认情况下,注册中心 CLI 工具是只读的
- 生产环境中心跳端点应该经过身份验证
- 使用防火墙规则限制注册中心访问

---

## 故障排除

### 脚本构建失败

**问题**: `构建 X 脚本失败`

**解决方案**:
```bash
# 确保 Go 已安装并在 PATH 中
go version

# 更新依赖
go mod tidy

# 尝试手动构建
go build -o bin/X scripts/X.go
```

### E2E 测试服务无法启动

**问题**: 端口已被使用

**解决方案**:
```bash
# 检查什么正在使用端口
lsof -i :8090  # 匹配器
lsof -i :9200  # 验证器

# 终止旧进程
pkill -f "bin/matcher"
pkill -f "bin/validator"

# 或在 e2e-test.sh 中更改端口
```

### 注册中心 CLI 返回空结果

**问题**: `curl: (7) 连接失败`

**解决方案**:
```bash
# 检查注册中心是否运行
./scripts/registry-cli.sh health

# 启用注册中心启动验证器
./bin/validator --registry-grpc :8092 --registry-http :8093 ...

# 或设置 REGISTRY_URL
export REGISTRY_URL="http://your-registry:8092"
```

### 意图提交失败

**问题**: 签名验证失败

**解决方案**:
```bash
# 验证参数 JSON 是否有效
echo $PARAMS_JSON | jq .

# 使用详细输出检查签名创建
# submit-intent-signed 显示签名详情

# 验证私钥格式
# 应该是64个十六进制字符(可选 0x 前缀)
```

### 验证包未提交

**问题**: E2E 测试完成但没有验证包

**解决方案**:
- 等待更长时间 - 验证包提交在执行报告到达后的下一个检查点发生
- 检查验证器日志: `grep -i "validation bundle" e2e-test-logs/validator-*.log`
- 验证 RootLayer 是否可访问
- 检查 ENABLE_CHAIN_SUBMIT 是否正确设置

---

## 其他资源

- **子网部署指南**: `docs/subnet_deployment_guide.md`
- **注册指南**: `scripts/REGISTRATION_GUIDE.md`
- **架构概述**: `docs/ARCHITECTURE_OVERVIEW.md`
- **API 文档**: 见 `proto/` 中的 proto 定义

---

## 贡献

添加新脚本时:

1. 遵循命名约定: shell 脚本使用 `kebab-case.sh`,Go 使用 `kebab-case.go`
2. 添加全面的 `--help` 文档
3. 同时支持 CLI 标志和环境变量
4. 在本指南中包含示例
5. 添加错误处理和用户友好的消息
6. 使用颜色输出助手以保持一致性

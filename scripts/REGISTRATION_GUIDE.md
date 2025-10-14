# 参与者注册指南 (Participant Registration Guide)

本指南说明如何使用注册脚本在链上注册 Validator、Matcher 和 Agent。

## 功能特性

✅ **统一注册** - 使用同一个私钥同时注册三个角色
✅ **自动查询** - 自动查询并显示最小质押要求
✅ **状态检查** - 检查当前注册状态，避免重复注册
✅ **灵活配置** - 支持配置文件和命令行参数
✅ **干运行模式** - 支持 dry-run，预览执行结果
✅ **ETH/ERC20** - 支持 ETH 和 ERC20 代币质押

## 前置要求

1. **Go 环境** - 需要 Go 1.24+
2. **SDK 依赖** - 需要 `intent-protocol-contract-sdk`
3. **RPC 节点** - Base Sepolia 或其他网络的 RPC URL
4. **私钥** - 有足够 ETH 的以太坊私钥
5. **Subnet 合约地址** - 已部署的 Subnet 合约地址

## 配置文件设置

### 方式 1：使用 config.yaml（推荐）

编辑 `config/config.yaml`，确保包含以下配置：

```yaml
# 私钥配置（使用同一个私钥注册所有角色）
agent:
  matcher:
    signer:
      private_key: "EXAMPLE_PRIVATE_KEY_DO_NOT_USE_1234567890ABCDEF1234567890ABCDEF"

# 区块链配置
blockchain:
  enabled: true
  rpc_url: "https://sepolia.base.org"
  subnet_contract: "0xYourSubnetContractAddress"
```

### 方式 2：使用环境变量

```bash
export NETWORK="base_sepolia"
export RPC_URL="https://sepolia.base.org"
export SUBNET_CONTRACT="0xYourSubnetContractAddress"
export PRIVATE_KEY="0xYourPrivateKey"
export DOMAIN="my-subnet.example.com"
```

## 使用方法

### 1. 检查注册状态

首先检查地址是否已经注册：

```bash
cd /Users/ty/pinai/protocol/Subnet
./scripts/register.sh --check
```

输出示例：
```
🔍 Checking current registration status...
   ✅ Already registered as Validator
   ❌ Not registered as Matcher
   ❌ Not registered as Agent
```

### 2. 完整注册（推荐）

使用配置文件中的设置注册所有角色：

```bash
./scripts/register.sh
```

### 3. 干运行模式

预览将要执行的操作，不提交实际交易：

```bash
./scripts/register.sh --dry-run
```

### 4. 使用命令行参数

覆盖配置文件的设置：

```bash
./scripts/register.sh \
  --rpc "https://sepolia.base.org" \
  --subnet "0x1234567890abcdef..." \
  --key "0xYourPrivateKeyHex" \
  --domain "my-validator.com"
```

### 5. 选择性注册

只注册特定角色：

```bash
# 只注册 Validator
./scripts/register.sh --skip-matcher --skip-agent

# 只注册 Matcher 和 Agent
./scripts/register.sh --skip-validator

# 只注册 Agent
./scripts/register.sh --skip-validator --skip-matcher
```

### 6. 自定义质押金额

指定每个角色的质押金额（单位：ETH）：

```bash
./scripts/register.sh \
  --validator-stake "1.0" \
  --matcher-stake "0.5" \
  --agent-stake "0.3"
```

### 7. 使用 ERC20 质押

使用 ERC20 代币代替 ETH 质押：

```bash
# 注意：需要先 approve 代币
./scripts/register.sh --erc20
```

## 完整参数说明

| 参数 | 说明 | 默认值 |
|-----|------|--------|
| `--config FILE` | 配置文件路径 | `config/config.yaml` |
| `--network NAME` | 网络名称 | `base_sepolia` |
| `--rpc URL` | RPC URL | 从配置读取 |
| `--subnet ADDRESS` | Subnet 合约地址 | 从配置读取 |
| `--key HEX` | 私钥（0x 前缀可选） | 从配置读取 |
| `--domain DOMAIN` | 参与者域名 | `validator.example.com` |
| `--validator-port` | Validator 端口 | `9090` |
| `--matcher-port` | Matcher 端口 | `8090` |
| `--agent-port` | Agent 端口 | `7070` |
| `--metadata URI` | 元数据 URI（可选） | 空 |
| `--validator-stake` | Validator 质押金额（ETH） | `0.1` |
| `--matcher-stake` | Matcher 质押金额（ETH） | `0.05` |
| `--agent-stake` | Agent 质押金额（ETH） | `0.05` |
| `--erc20` | 使用 ERC20 质押 | `false` |
| `--check` | 仅检查注册状态 | `false` |
| `--dry-run` | 干运行模式 | `false` |
| `--skip-validator` | 跳过 Validator 注册 | `false` |
| `--skip-matcher` | 跳过 Matcher 注册 | `false` |
| `--skip-agent` | 跳过 Agent 注册 | `false` |
| `--help` | 显示帮助信息 | - |

## 输出示例

### 成功注册

```
🚀 Starting participant registration script
   Network: base_sepolia
   RPC URL: https://sepolia.base.org
   Subnet Contract: 0x1234567890abcdef...
   Domain: my-subnet.example.com
   Chain ID: 84532
   Signer Address: 0xabcdef1234567890...
   Balance: 1.234567 ETH
   Subnet Name: My Subnet
   Subnet Owner: 0x9876543210fedcba...

📊 Stake Requirements:
   Min Validator Stake: 0.100000 ETH
   Min Matcher Stake: 0.050000 ETH
   Min Agent Stake: 0.050000 ETH
   Auto Approve: true
   Unstake Lock Period: 7d

🔍 Checking current registration status...
   ❌ Not registered as Validator
   ❌ Not registered as Matcher
   ❌ Not registered as Agent

🔐 Starting registration process...

📝 Registering as Validator...
   Domain: my-subnet.example.com
   Endpoint: https://my-subnet.example.com:9090
   Stake: 0.100000 ETH
   📤 Transaction submitted: 0x1a2b3c4d5e6f...
   ⏳ Waiting for confirmation...
   ✅ Validator registration completed

📝 Registering as Matcher...
   Domain: my-subnet.example.com
   Endpoint: https://my-subnet.example.com:8090
   Stake: 0.050000 ETH
   📤 Transaction submitted: 0x7f8e9d0c1b2a...
   ⏳ Waiting for confirmation...
   ✅ Matcher registration completed

📝 Registering as Agent...
   Domain: my-subnet.example.com
   Endpoint: https://my-subnet.example.com:7070
   Stake: 0.050000 ETH
   📤 Transaction submitted: 0x3c4d5e6f7a8b...
   ⏳ Waiting for confirmation...
   ✅ Agent registration completed

🎉 Registration process completed!
```

## 常见问题

### Q1: 可以使用同一个私钥注册所有角色吗？

**可以！** 一个以太坊地址可以同时注册为 Validator、Matcher 和 Agent。这是完全允许的。

### Q2: 注册需要多少 ETH？

查询最小质押要求：
```bash
./scripts/register.sh --check
```

通常：
- Validator: 0.1 - 1 ETH
- Matcher: 0.05 - 0.5 ETH
- Agent: 0.05 - 0.5 ETH
- Gas 费用: ~0.01 ETH per transaction

### Q3: 如果质押金额不足会怎样？

交易会失败并显示错误信息。使用 `--check` 查看最小质押要求。

### Q4: 注册后如何验证？

```bash
# 再次检查状态
./scripts/register.sh --check

# 或查询链上数据
go run scripts/register-participants.go --check
```

### Q5: autoApprove=false 时需要等待批准吗？

是的，如果 `autoApprove=false`，注册后需要等待 Subnet 管理员批准。

检查批准状态：
```bash
# Status 会显示 Pending (等待批准) 或 Active (已批准)
./scripts/register.sh --check
```

### Q6: 如何使用 ERC20 代币质押？

```bash
# 1. 先授权（approve）Subnet 合约
# 使用 SDK 或 Etherscan 调用 token.approve(subnetAddress, amount)

# 2. 使用 --erc20 参数注册
./scripts/register.sh --erc20 --validator-stake "1000"
```

### Q7: 注册失败如何排查？

1. **检查余额**：`--check` 查看余额是否足够
2. **检查网络**：确认 RPC URL 正确
3. **检查合约地址**：确认 Subnet 合约地址正确
4. **查看日志**：查看详细错误信息
5. **使用 dry-run**：`--dry-run` 预检查

### Q8: 如何解除注册？

需要调用 Unstake 相关接口：

```go
// 申请解押
stakingSvc.RequestUnstake(ctx, tokenAddr, sdk.ParticipantValidator, amount)

// 等待解锁期后提取
stakingSvc.Withdraw(ctx, tokenAddr, sdk.ParticipantValidator)
```

## 安全建议

⚠️ **生产环境注意事项：**

1. **私钥安全**
   - 不要在公共仓库提交私钥
   - 使用环境变量或加密存储
   - 生产环境使用 HSM/KMS

2. **质押金额**
   - 先在测试网验证
   - 确认最小质押要求
   - 保留足够的 gas 费用

3. **网络选择**
   - 测试网：Base Sepolia
   - 主网：Base Mainnet
   - 确认 Chain ID 正确

4. **合约验证**
   - 确认 Subnet 合约地址正确
   - 验证合约是否已部署
   - 检查合约所有者

## 故障排除

### 错误：Failed to connect to Ethereum node

```bash
# 检查 RPC URL 是否可访问
curl -X POST $RPC_URL \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'

# 尝试其他 RPC 端点
--rpc "https://base-sepolia.g.alchemy.com/v2/YOUR_API_KEY"
```

### 错误：insufficient funds for gas * price + value

```bash
# 检查余额
./scripts/register.sh --check

# 获取测试网 ETH（Base Sepolia）
# https://www.alchemy.com/faucets/base-sepolia
```

### 错误：Invalid subnet contract

```bash
# 验证合约地址
cast code $SUBNET_CONTRACT --rpc-url $RPC_URL

# 或使用 Basescan
# https://sepolia.basescan.org/address/$SUBNET_CONTRACT
```

## 相关资源

- [SDK 文档](../intent-protocol-contract-sdk/README.md)
- [智能合约 ABI](../intent-protocol-contract-sdk/contracts/)
- [配置文件示例](../config/config.yaml)
- [Base Sepolia Faucet](https://www.alchemy.com/faucets/base-sepolia)
- [Basescan Testnet](https://sepolia.basescan.org/)

## 支持

如有问题，请提交 Issue 或联系开发团队。

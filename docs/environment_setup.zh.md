# 环境配置指南

PinAI Subnet 开发环境完整配置指南。

## 目录

- [系统要求](#系统要求)
- [核心依赖](#核心依赖)
- [可选工具](#可选工具)
- [安装说明](#安装说明)
- [项目配置](#项目配置)
- [配置文件](#配置文件)
- [验证安装](#验证安装)
- [故障排除](#故障排除)

---

## 系统要求

### 支持的操作系统

- **Linux** (Ubuntu 20.04+, Debian 11+, CentOS 8+)
- **macOS** (11.0 Big Sur 或更高版本)
- **Windows** (推荐使用 WSL2)

### 硬件要求

**最低配置**:
- CPU: 2核
- 内存: 4 GB
- 磁盘: 20 GB 可用空间
- 网络: 稳定的互联网连接

**生产环境推荐配置**:
- CPU: 4核或以上
- 内存: 8 GB 或以上
- 磁盘: 100+ GB SSD
- 网络: 低延迟连接 (< 100ms 到区块链 RPC)

---

## 核心依赖

### 1. Go 编程语言

**所需版本**: Go 1.24.0 或更高

Go 是 Subnet 组件(匹配器、验证器、注册中心)的主要语言。

**为什么需要**:
- 编译 Subnet 二进制文件
- 运行基于 Go 的脚本
- 构建自定义代理

### 2. Protocol Buffers 编译器 (protoc)

**所需版本**: protoc 3.20.0 或更高

用于生成 gRPC 服务定义。

**为什么需要**:
- 生成 protocol buffer 代码
- 将 `.proto` 文件编译为 Go/Python

### 3. NATS 消息代理

**所需版本**: NATS 2.9.0 或更高

用于验证器共识的分布式消息系统。

**为什么需要**:
- 验证器间通信
- 共识消息广播
- 检查点协调

### 4. Git

**所需版本**: Git 2.30.0 或更高

版本控制系统。

**为什么需要**:
- 克隆仓库
- 管理代码版本
- 贡献代码

---

## 可选工具

### 用于测试和开发

1. **Docker 和 Docker Compose**
   - 快速部署 NATS
   - 容器化测试
   - 版本: Docker 20.10+ / Docker Compose 2.0+

2. **curl**
   - HTTP API 测试
   - RootLayer 交互
   - 大多数系统预装

3. **jq**
   - 脚本中的 JSON 解析
   - Registry CLI 格式化
   - 非必需但强烈推荐

### 用于区块链交互

4. **以太坊节点访问**
   - 公共 RPC: Infura, Alchemy, QuickNode
   - 或自建节点
   - 链上操作必需

5. **以太坊钱包**
   - 带有 ETH 的私钥用于支付 gas 费
   - 测试网 ETH 用于开发
   - 推荐使用 Sepolia 测试网

### 用于 Python SDK 开发

6. **Python 3.9+**
   - 用于 Python SDK 开发
   - pip 包管理器
   - 推荐使用 virtualenv

---

## 安装说明

### macOS

#### 使用 Homebrew (推荐)

```bash
# 如果尚未安装 Homebrew,先安装
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# 安装 Go
brew install go

# 验证 Go 安装
go version  # 应显示 go1.24.0 或更高

# 安装 Protocol Buffers
brew install protobuf

# 验证 protoc 安装
protoc --version  # 应显示 libprotoc 3.20.0 或更高

# 安装 NATS server
brew install nats-server

# 安装可选工具
brew install curl jq git

# 安装 Docker Desktop (包含 Docker Compose)
# 从以下网址下载: https://www.docker.com/products/docker-desktop
```

#### 手动安装

**Go**:
```bash
# 从 https://go.dev/dl/ 下载 Go
curl -OL https://go.dev/dl/go1.24.0.darwin-arm64.tar.gz  # Apple Silicon
# 或
curl -OL https://go.dev/dl/go1.24.0.darwin-amd64.tar.gz  # Intel

# 解压并安装
sudo tar -C /usr/local -xzf go1.24.0.darwin-*.tar.gz

# 添加到 PATH (添加到 ~/.zshrc 或 ~/.bash_profile)
export PATH=$PATH:/usr/local/go/bin
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin

# 重新加载 shell 配置
source ~/.zshrc  # 或 source ~/.bash_profile
```

**Protocol Buffers**:
```bash
# 从 https://github.com/protocolbuffers/protobuf/releases 下载
curl -OL https://github.com/protocolbuffers/protobuf/releases/download/v24.4/protoc-24.4-osx-universal_binary.zip

# 解压
unzip protoc-24.4-osx-universal_binary.zip -d $HOME/protoc

# 添加到 PATH
export PATH=$PATH:$HOME/protoc/bin
```

**NATS**:
```bash
# 从 https://github.com/nats-io/nats-server/releases 下载
curl -OL https://github.com/nats-io/nats-server/releases/download/v2.10.7/nats-server-v2.10.7-darwin-arm64.zip

# 解压
unzip nats-server-v2.10.7-darwin-arm64.zip

# 移动到 /usr/local/bin
sudo mv nats-server-v2.10.7-darwin-arm64/nats-server /usr/local/bin/

# 验证
nats-server --version
```

---

### Linux (Ubuntu/Debian)

```bash
# 更新包索引
sudo apt update

# 安装基本工具
sudo apt install -y curl git build-essential unzip

# 安装 Go
wget https://go.dev/dl/go1.24.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.24.0.linux-amd64.tar.gz

# 添加到 PATH (添加到 ~/.bashrc 或 ~/.zshrc)
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
echo 'export GOPATH=$HOME/go' >> ~/.bashrc
echo 'export PATH=$PATH:$GOPATH/bin' >> ~/.bashrc
source ~/.bashrc

# 验证 Go
go version

# 安装 Protocol Buffers
sudo apt install -y protobuf-compiler

# 或手动安装最新版本
PB_REL="https://github.com/protocolbuffers/protobuf/releases"
curl -LO $PB_REL/download/v24.4/protoc-24.4-linux-x86_64.zip
unzip protoc-24.4-linux-x86_64.zip -d $HOME/.local
export PATH="$PATH:$HOME/.local/bin"

# 验证 protoc
protoc --version

# 安装 NATS server
curl -L https://github.com/nats-io/nats-server/releases/download/v2.10.7/nats-server-v2.10.7-linux-amd64.tar.gz -o nats-server.tar.gz
tar -xzf nats-server.tar.gz
sudo mv nats-server-v2.10.7-linux-amd64/nats-server /usr/local/bin/
rm -rf nats-server*

# 验证 NATS
nats-server --version

# 安装 Docker (可选)
sudo apt install -y docker.io docker-compose
sudo systemctl start docker
sudo systemctl enable docker
sudo usermod -aG docker $USER  # 将用户添加到 docker 组

# 安装 jq (可选但推荐)
sudo apt install -y jq
```

---

### Linux (CentOS/RHEL/Fedora)

```bash
# 安装基本工具
sudo yum install -y curl git gcc make unzip

# 安装 Go
wget https://go.dev/dl/go1.24.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.24.0.linux-amd64.tar.gz

# 添加到 PATH
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
echo 'export GOPATH=$HOME/go' >> ~/.bashrc
echo 'export PATH=$PATH:$GOPATH/bin' >> ~/.bashrc
source ~/.bashrc

# 安装 Protocol Buffers
sudo yum install -y protobuf-compiler

# 安装 NATS
curl -L https://github.com/nats-io/nats-server/releases/download/v2.10.7/nats-server-v2.10.7-linux-amd64.tar.gz -o nats-server.tar.gz
tar -xzf nats-server.tar.gz
sudo mv nats-server-v2.10.7-linux-amd64/nats-server /usr/local/bin/

# 安装 Docker (Fedora/CentOS 8+)
sudo dnf install -y docker docker-compose
sudo systemctl start docker
sudo systemctl enable docker
sudo usermod -aG docker $USER
```

---

### Windows (WSL2)

**推荐**: 使用 Windows Subsystem for Linux 2 (WSL2) 以获得最佳兼容性。

1. **安装 WSL2**:
   ```powershell
   # 在 PowerShell (管理员权限)
   wsl --install
   # 或指定 Ubuntu
   wsl --install -d Ubuntu-22.04
   ```

2. **在 WSL2 终端中遵循 Linux (Ubuntu) 说明**

3. **安装 Docker Desktop for Windows**:
   - 从 https://www.docker.com/products/docker-desktop 下载
   - 在 Docker Desktop 设置中启用 WSL2 集成

---

## 项目配置

### 1. 克隆仓库

```bash
# 克隆 Subnet 仓库
git clone https://github.com/PIN-AI/Subnet.git
cd Subnet
```

### 2. 安装 Go 依赖

```bash
# 下载并安装 Go 模块依赖
go mod download

# 验证依赖
go mod verify
```

### 3. 安装 Go Protocol Buffer 插件

```bash
# 安装 protoc-gen-go (用于 Protocol Buffers)
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

# 安装 protoc-gen-go-grpc (用于 gRPC)
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# 验证安装
which protoc-gen-go
which protoc-gen-go-grpc
```

### 4. 构建所有二进制文件

```bash
# 使用 Makefile 构建所有组件
make build

# 这将在 ./bin/ 中创建二进制文件:
# - matcher
# - validator
# - registry
# - mock-rootlayer
# - simple-agent
```

### 5. 验证构建

```bash
# 检查已构建的二进制文件
ls -lh bin/

# 测试运行二进制文件
./bin/matcher --help
./bin/validator --help
```

---

## 配置文件

### 1. 启动 NATS Server

```bash
# 使用默认配置启动 NATS
nats-server

# 或在后台启动
nats-server -D

# 或使用 Docker
docker run -d --name nats -p 4222:4222 nats:latest

# 验证 NATS 正在运行
curl http://localhost:8222/varz  # NATS 监控端点
```

### 2. 获取测试网 ETH

在 Base Sepolia 上测试:

1. **获取钱包**:
   - 使用 MetaMask 创建或使用现有钱包
   - 安全保存您的私钥

2. **获取测试网 ETH**:
   - Base Sepolia 水龙头: https://www.coinbase.com/faucets/base-ethereum-goerli-faucet
   - 或从 Sepolia 桥接到 Base Sepolia

3. **保存配置**:
   ```bash
   # 创建 .env 文件 (请勿提交此文件!)
   cat > .env << 'EOF'
   PRIVATE_KEY=你的私钥
   RPC_URL=https://sepolia.base.org
   NETWORK=base_sepolia
   EOF

   # 保护文件
   chmod 600 .env
   ```

### 3. 配置 RootLayer 连接

```bash
# 设置 RootLayer 端点 (示例 - 根据您的部署调整)
export ROOTLAYER_GRPC="3.17.208.238:9001"
export ROOTLAYER_HTTP="http://3.17.208.238:8081"

# 或添加到您的 .env 文件
echo "ROOTLAYER_GRPC=3.17.208.238:9001" >> .env
echo "ROOTLAYER_HTTP=http://3.17.208.238:8081" >> .env
```

### 4. 创建配置文件

```bash
# 复制示例配置
cp config/config.example.yaml config/config.yaml

# 使用您的值编辑
nano config/config.yaml
# 或
vim config/config.yaml
```

**最小化 config/config.yaml**:
```yaml
subnet_id: "0x0000000000000000000000000000000000000000000000000000000000000002"

identity:
  matcher_id: "matcher-001"
  subnet_id: "0x0000000000000000000000000000000000000000000000000000000000000002"

network:
  nats_url: "nats://127.0.0.1:4222"
  matcher_grpc_port: ":8090"

rootlayer:
  http_url: "http://3.17.208.238:8081"
  grpc_endpoint: "3.17.208.238:9001"

blockchain:
  rpc_url: "https://sepolia.base.org"
  subnet_contract: "0x你的子网合约地址"

agent:
  matcher:
    signer:
      type: "ecdsa"
      private_key: "你的私钥"
```

---

## 验证安装

### 测试您的配置

```bash
# 1. 验证 Go 工作正常
go version
go env GOPATH

# 2. 验证 protoc 工作正常
protoc --version

# 3. 验证 NATS 可访问
nc -zv localhost 4222
# 或
telnet localhost 4222

# 4. 验证二进制文件运行
./bin/matcher --help
./bin/validator --help

# 5. 运行单元测试
make test
# 或
go test ./...

# 6. 构建 protocol buffers
make proto

# 7. 检查 Go 模块状态
go mod verify
go mod tidy
```

### 运行快速 E2E 测试

```bash
# 设置必需的环境变量
export SUBNET_ID="0x0000000000000000000000000000000000000000000000000000000000000002"
export ROOTLAYER_GRPC="3.17.208.238:9001"
export ROOTLAYER_HTTP="http://3.17.208.238:8081"

# 运行 E2E 测试 (非交互模式)
./scripts/e2e-test.sh --no-interactive

# 检查成功
echo $?  # 应该是 0
```

---

## 故障排除

### Go 问题

**问题**: `go: command not found`

**解决方案**:
```bash
# 验证 Go 已安装
which go

# 如果未找到,检查 PATH
echo $PATH | grep go

# 添加 Go 到 PATH
export PATH=$PATH:/usr/local/go/bin
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin

# 添加到 ~/.bashrc 或 ~/.zshrc 使其永久生效
```

---

**问题**: `cannot find package`

**解决方案**:
```bash
# 清理模块缓存
go clean -modcache

# 重新下载依赖
go mod download

# 验证模块
go mod verify

# 更新依赖
go mod tidy
```

---

### NATS 问题

**问题**: NATS `connection refused`

**解决方案**:
```bash
# 检查 NATS 是否运行
ps aux | grep nats-server

# 检查端口 4222
lsof -i :4222
# 或
netstat -an | grep 4222

# 如果未运行则启动 NATS
nats-server -D

# 或使用 Docker
docker run -d --name nats -p 4222:4222 nats:latest

# 检查 NATS 日志
docker logs nats
```

---

**问题**: NATS 连接超时

**解决方案**:
```bash
# 测试连接
nc -zv localhost 4222

# 检查防火墙
sudo ufw status  # Ubuntu/Debian
sudo firewall-cmd --list-all  # CentOS/RHEL

# 允许 NATS 端口
sudo ufw allow 4222
# 或
sudo firewall-cmd --add-port=4222/tcp --permanent
sudo firewall-cmd --reload
```

---

### 构建问题

**问题**: `make: command not found`

**解决方案**:
```bash
# 安装 build-essential (Ubuntu/Debian)
sudo apt install build-essential

# 或仅安装 make
sudo apt install make

# macOS
xcode-select --install
```

---

**问题**: Protocol buffer 编译失败

**解决方案**:
```bash
# 检查 protoc 版本
protoc --version  # 需要 3.20.0+

# 验证 protoc 插件已安装
which protoc-gen-go
which protoc-gen-go-grpc

# 重新安装插件
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# 验证 PATH 包含 $GOPATH/bin
echo $PATH | grep $GOPATH/bin

# 如果缺失则添加
export PATH=$PATH:$GOPATH/bin
```

---

**问题**: 二进制文件执行失败: "permission denied"

**解决方案**:
```bash
# 使二进制文件可执行
chmod +x bin/*

# 或重新构建
make clean
make build
```

---

### 网络/区块链问题

**问题**: 无法连接到 RPC 端点

**解决方案**:
```bash
# 测试 RPC 连接
curl -X POST \
  -H "Content-Type: application/json" \
  --data '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' \
  https://sepolia.base.org

# 尝试替代 RPC
export RPC_URL="https://base-sepolia.g.alchemy.com/v2/YOUR_API_KEY"

# 检查 RPC 是否需要 API 密钥
# 从以下网站获取免费 API 密钥:
# - Alchemy: https://www.alchemy.com/
# - Infura: https://infura.io/
# - QuickNode: https://www.quicknode.com/
```

---

**问题**: gas 费用资金不足

**解决方案**:
```bash
# 检查余额
# 使用区块浏览器: https://sepolia.basescan.org/

# 获取测试网 ETH
# - Base Sepolia 水龙头: https://www.coinbase.com/faucets
# - 或从 Sepolia ETH 桥接

# 验证钱包地址
go run scripts/derive-pubkey.go $PRIVATE_KEY
```

---

### Docker 问题

**问题**: Docker 命令 `permission denied`

**解决方案**:
```bash
# 将用户添加到 docker 组
sudo usermod -aG docker $USER

# 注销并重新登录以使组生效
# 或运行
newgrp docker

# 测试
docker ps
```

---

**问题**: 端口已被使用

**解决方案**:
```bash
# 查找正在使用端口的进程
lsof -i :4222  # NATS
lsof -i :8090  # 匹配器
lsof -i :9200  # 验证器

# 终止进程
kill -9 <PID>

# 或停止 Docker 容器
docker ps
docker stop <container_name>

# 或在配置中更改端口
```

---

## 环境变量参考

### 开发所需

```bash
# Go
export PATH=$PATH:/usr/local/go/bin
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin

# Protocol Buffers
export PATH=$PATH:$HOME/.local/bin  # 如果本地安装

# 项目特定
export SUBNET_ID="0x..."
export ROOTLAYER_GRPC="主机:端口"
export ROOTLAYER_HTTP="http://主机:端口"
```

### 区块链操作所需

```bash
# 钱包
export PRIVATE_KEY="0x..."
export RPC_URL="https://sepolia.base.org"
export NETWORK="base_sepolia"

# 合约
export PIN_BASE_SEPOLIA_INTENT_MANAGER="0x..."
export PIN_BASE_SEPOLIA_SUBNET_FACTORY="0x..."
```

### 可选

```bash
# 注册中心
export REGISTRY_URL="http://localhost:8092"

# 测试
export ENABLE_CHAIN_SUBMIT="true"
export CHAIN_RPC_URL="https://sepolia.base.org"
```

---

## 下一步

完成环境配置后:

1. **创建子网**:
   ```bash
   ./scripts/create-subnet.sh --help
   ```

2. **注册参与者**:
   ```bash
   ./scripts/register.sh --help
   ```

3. **运行 E2E 测试**:
   ```bash
   ./scripts/e2e-test.sh --help
   ```

4. **阅读文档**:
   - [子网部署指南](./subnet_deployment_guide.zh.md)
   - [脚本指南](./scripts_guide.zh.md)
   - [架构概述](./architecture.md)

5. **加入社区**:
   - GitHub: https://github.com/PIN-AI/Subnet
   - Discord: [链接]
   - 文档: [链接]

---

## 开发工作流程

配置完成后的典型开发工作流程:

```bash
# 1. 进行代码更改
vim internal/matcher/server.go

# 2. 重新构建受影响的组件
make build

# 3. 运行测试
go test ./internal/matcher/...

# 4. 本地测试
./scripts/e2e-test.sh

# 5. 检查问题
go vet ./...
go fmt ./...

# 6. 提交更改
git add .
git commit -m "feat: 改进匹配器竞标逻辑"
```

---

## 性能调优

### 用于生产部署

1. **增加文件描述符限制**:
   ```bash
   ulimit -n 65536
   # 在 /etc/security/limits.conf 中永久设置
   ```

2. **优化 Go 运行时**:
   ```bash
   export GOMAXPROCS=$(nproc)  # 使用所有 CPU 核心
   export GOGC=50  # 更激进的 GC
   ```

3. **NATS 调优**:
   ```bash
   # 创建 nats-server.conf
   max_connections: 10000
   max_payload: 10MB
   write_deadline: "10s"
   ```

4. **监控资源**:
   ```bash
   # 安装监控工具
   sudo apt install htop iotop nethogs

   # 监控进程
   htop
   ```

---

## 安全最佳实践

1. **永远不要将私钥提交**到版本控制
2. **使用环境变量**存储敏感数据
3. **设置限制性文件权限**: `chmod 600 config/config.yaml`
4. **使用单独的钱包**用于测试和生产
5. **保持依赖更新**: `go get -u ./...`
6. **使用防火墙规则**限制访问
7. **启用 TLS** 用于生产 gRPC 端点

---

## 获取帮助

如果遇到问题:

1. 检查[故障排除](#故障排除)部分
2. 查阅[脚本指南](./scripts_guide.zh.md)
3. 搜索 GitHub Issues: https://github.com/PIN-AI/Subnet/issues
4. 在 Discord 提问: [链接]
5. 创建新 issue 并包含:
   - 操作系统和版本
   - Go 版本 (`go version`)
   - 错误消息 (完整输出)
   - 重现步骤

---

## 贡献

欢迎贡献! 请:

1. Fork 仓库
2. 创建功能分支
3. 进行更改
4. 运行测试: `make test`
5. 格式化代码: `go fmt ./...`
6. 提交 pull request

详细指南请参见 [CONTRIBUTING.md](../CONTRIBUTING.md)。

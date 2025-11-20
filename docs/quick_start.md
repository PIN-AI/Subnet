# 🚀 PinAI Subnet - Quick Start

**📍 Prerequisites:** Complete [Environment Setup](environment_setup.md) first (install Go, Docker, protoc, etc.)

**Time to complete:** ~15 minutes (after environment setup)

---

## 🤔 Which Guide Should I Follow?

**Quick decision table:**

| I want to... | Use this guide | Time |
|-------------|----------------|------|
| 🚀 **Test quickly (1 node)** | [Single Node Quick Start](#single-node-quick-start-5-minutes) ⭐ **Recommended for first-time users** | 5 min |
| 🐳 Docker deployment (3 nodes) | [Docker Deployment](../deployment/README.md) | 5 min |
| 🔧 Multi-node setup (3+ nodes) | [Manual Deployment](#option-2-traditional-deployment) | 15 min |
| 📦 Build custom agent | [Agent SDK Docs](https://github.com/PIN-AI/subnet-sdk) | - |
| 🏭 Deploy to production | [Production Guide](subnet_deployment_guide.md#production-deployment) | - |
| 🔍 Fix issues | [Troubleshooting](subnet_deployment_guide.md#troubleshooting) | - |

> 💡 **New users**: Start with the [Single Node Quick Start](#single-node-quick-start-5-minutes) below for fastest testing!

---

## 📦 Project Setup

Before starting deployment, clone the repository and build the project:

### 1. Clone the Repository

```bash
# Clone Subnet repository
git clone https://github.com/PIN-AI/Subnet.git
cd Subnet
```

### 2. Install Go Dependencies

```bash
# Download and install Go module dependencies
go mod download

# Verify dependencies
go mod verify
```

### 3. Install Go Protocol Buffer Plugins

```bash
# Install protoc-gen-go (for Protocol Buffers)
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

# Install protoc-gen-go-grpc (for gRPC)
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Verify installations
which protoc-gen-go
which protoc-gen-go-grpc
```

### 4. Build All Binaries

```bash
# Build all components using Makefile
make build

# This creates binaries in ./bin/:
# - matcher
# - validator
# - mock-rootlayer
# - simple-agent
# - derive-pubkey
# - create-subnet
# - register-participants
```

### 5. Verify Build

```bash
# Check built binaries
ls -lh bin/

# Test running a binary
./bin/matcher --help
./bin/validator --help
```

---

## 🚀 Single Node Quick Start (5 minutes)

**Want to test quickly? Copy-paste this complete workflow:**

This creates a single-node subnet with threshold 1/1 - perfect for testing and development.

### ⚠️ **Prerequisites: Get Testnet ETH First!**

**You need Base Sepolia testnet ETH before proceeding:**

- **Required**: At least **0.05 ETH** on Base Sepolia testnet
- **Why**:
  - Create subnet: ~0.005 ETH (gas)
  - Register components: 3 × 0.01 ETH stake + gas = ~0.033 ETH

**Get testnet ETH:**

1. **Option 1: Coinbase Faucet** (Recommended)
   - Visit: https://www.coinbase.com/faucets/base-ethereum-goerli-faucet
   - Connect wallet and request testnet ETH

2. **Option 2: Bridge from Sepolia**
   - Get Sepolia ETH from https://sepoliafaucet.com
   - Bridge to Base Sepolia: https://bridge.base.org

3. **Verify your balance:**
   ```bash
   # Check on block explorer (replace with your address)
   open https://sepolia.basescan.org/address/0xYourAddress
   ```

> 💡 **Tip**: Generate your private key first, derive the address, then fund it before proceeding.

---

```bash
# Navigate to project directory
cd Subnet

# 1. Build tools (if not already done)
make build

# 2. Generate validator key
PRIVKEY=$(openssl rand -hex 32)
PUBKEY=$(./bin/derive-pubkey "$PRIVKEY")

# Get your wallet address
ADDRESS=$(cast wallet address $PRIVKEY 2>/dev/null || echo "Install foundry to see address")
echo "Your Address: $ADDRESS"
echo "⚠️  FUND THIS ADDRESS WITH 0.05+ ETH ON BASE SEPOLIA BEFORE CONTINUING!"
echo "   Check balance: https://sepolia.basescan.org/address/$ADDRESS"
read -p "Press Enter after funding..."

echo "Generated validator key:"
echo "  Private: $PRIVKEY"
echo "  Public:  $PUBKEY"
echo ""

# 3. Create subnet with threshold 1/1 (for single node)
PRIVATE_KEY=$PRIVKEY ./scripts/create-subnet.sh \
  --name "Test Subnet Single Node" \
  --threshold-num 1 \
  --threshold-denom 1

# 📝 SAVE the output: Subnet ID and Contract Address
# Example output:
#   Subnet ID: 0x0000000000000000000000000000000000000000000000000000000000000008
#   Contract Address: 0x340B34AeE852A64360596eBf14039f76419e0bA7

# 4. Register components (replace with your values from step 3)
SUBNET_CONTRACT="<paste_contract_address_here>"
PRIVATE_KEY="$PRIVKEY" ./scripts/register.sh --subnet "$SUBNET_CONTRACT"

# 5. Configure .env
# Copy from template (contains all fixed network settings)
cp .env.example .env

# Edit .env and update these fields:
#   NUM_VALIDATORS=1
#   VALIDATOR_KEYS=<paste from step 2>
#   VALIDATOR_PUBKEYS=<paste from step 2>
#   TEST_PRIVATE_KEY=<paste from step 2>
#   SUBNET_ID=<paste from step 3>
#
# Quick edit: nano .env

# 6. Start subnet
./scripts/start-subnet.sh
```

**What happens:**
- ✅ Creates subnet with 1/1 threshold (1 signature needed from 1 validator)
- ✅ Registers Validator, Matcher, and Agent
- ✅ Configures single-node environment
- ✅ Starts all services

**Verify it's running:**
```bash
# Check processes
ps aux | grep -E "bin/(matcher|validator|simple-agent)" | grep -v grep

# View logs
tail -f subnet-logs/*.log
```

**Stop services:**
```bash
pkill -f 'bin/matcher|bin/validator|bin/simple-agent'
```

> 💡 **Next steps:** Once you verify single-node works, see [Multi-Node Setup](#option-2-traditional-deployment) for production deployment.

---
## 🚀 Single Node Quick Start (5 minutes)

**Want to test quickly? Copy-paste this complete workflow:**

This creates a single-node subnet with threshold 1/1 - perfect for testing and development.

### ⚠️ **Prerequisites: Get Testnet ETH First!**

**You need Base Sepolia testnet ETH before proceeding:**

- **Required**: At least **0.05 ETH** on Base Sepolia testnet
- **Why**:
  - Create subnet: ~0.005 ETH (gas)
  - Register components: 3 × 0.01 ETH stake + gas = ~0.033 ETH

**Get testnet ETH:**

1. **Option 1: Coinbase Faucet** (Recommended)
   - Visit: https://www.coinbase.com/faucets/base-ethereum-goerli-faucet
   - Connect wallet and request testnet ETH

2. **Option 2: Bridge from Sepolia**
   - Get Sepolia ETH from https://sepoliafaucet.com
   - Bridge to Base Sepolia: https://bridge.base.org

3. **Verify your balance:**
   ```bash
   # Check on block explorer (replace with your address)
   open https://sepolia.basescan.org/address/0xYourAddress
   ```

> 💡 **Tip**: Generate your private key first, derive the address, then fund it before proceeding.

---

```bash
# Navigate to project directory
cd /path/to/Subnet

# 1. Build tools
make build

# 2. Generate validator key
PRIVKEY=$(openssl rand -hex 32)
PUBKEY=$(./bin/derive-pubkey "$PRIVKEY")

# Get your wallet address
ADDRESS=$(cast wallet address $PRIVKEY 2>/dev/null || echo "Install foundry to see address")
echo "Your Address: $ADDRESS"
echo "⚠️  FUND THIS ADDRESS WITH 0.05+ ETH ON BASE SEPOLIA BEFORE CONTINUING!"
echo "   Check balance: https://sepolia.basescan.org/address/$ADDRESS"
read -p "Press Enter after funding..."

echo "Generated validator key:"
echo "  Private: $PRIVKEY"
echo "  Public:  $PUBKEY"
echo ""

# 3. Create subnet with threshold 1/1 (for single node)
./scripts/create-subnet.sh \
  --name "Test Subnet Single Node" \
  --threshold-num 1 \
  --threshold-denom 1

# 📝 SAVE the output: Subnet ID and Contract Address
# Example output:
#   Subnet ID: 0x0000000000000000000000000000000000000000000000000000000000000008
#   Contract Address: 0x340B34AeE852A64360596eBf14039f76419e0bA7

# 4. Register components (replace with your values from step 3)
SUBNET_CONTRACT="<paste_contract_address_here>"
PRIVATE_KEY="$PRIVKEY" ./scripts/register.sh --subnet "$SUBNET_CONTRACT"

# 5. Configure .env
# Copy from template (contains all fixed network settings)
cp .env.example .env

# Edit .env and update these fields:
#   NUM_VALIDATORS=1
#   VALIDATOR_KEYS=<paste from step 2>
#   VALIDATOR_PUBKEYS=<paste from step 2>
#   TEST_PRIVATE_KEY=<paste from step 2>
#   SUBNET_ID=<paste from step 3>
#
# Quick edit: nano .env

# 6. Start subnet
./scripts/start-subnet.sh
```

**What happens:**
- ✅ Creates subnet with 1/1 threshold (1 signature needed from 1 validator)
- ✅ Registers Validator, Matcher, and Agent
- ✅ Configures single-node environment
- ✅ Starts all services

**Verify it's running:**
```bash
# Check processes
ps aux | grep -E "bin/(matcher|validator|simple-agent)" | grep -v grep

# View logs
tail -f subnet-logs/*.log
```

**Stop services:**
```bash
pkill -f 'bin/matcher|bin/validator|bin/simple-agent'
```

> 💡 **Next steps:** Once you verify single-node works, see [Multi-Node Setup](#option-2-traditional-deployment) for production deployment.

---

## 🎯 Understanding Signature Threshold

**What is signature threshold?**
The minimum number of validator signatures required to finalize a checkpoint.

**Format:** `numerator/denominator`
Example: `2/3` means at least `ceil(2/3 * total_validators)` signatures are needed.

| Scenario | Validators | Threshold | Signatures Needed | Use Case |
|----------|-----------|-----------|-------------------|----------|
| **Testing** | 1 | **1/1** | 1 (100%) | Development, debugging |
| **Small Team** | 3 | **2/3** | 2 (67%) | Small production, tolerates 1 failure |
| **Production** | 3-5 | **3/4** | 3 (75%) | Higher security, Byzantine fault tolerance |
| **Enterprise** | 7+ | **5/7** | 5 (71%) | Large-scale production |

**Key Rules:**
- ✅ **Single node (1 validator)**: Must use **1/1** threshold
- ✅ **Multi-node**: Use **2/3** or **3/4** for fault tolerance
- ⚠️ Using default 3/4 with 1 validator works mathematically but is misleading
- ⚠️ Higher threshold = more security but needs more online validators

**Formula:**
```
Required signatures = ceil(threshold_num / threshold_denom * total_validators)

Examples:
- 1/1 with 1 validator = ceil(1/1 * 1) = 1 signature ✅
- 2/3 with 3 validators = ceil(2/3 * 3) = 2 signatures ✅
- 3/4 with 4 validators = ceil(3/4 * 4) = 3 signatures ✅
- 5/7 with 7 validators = ceil(5/7 * 7) = 5 signatures ✅
```

---

## ⚠️ Important: Complete Blockchain Registration First!

**Before starting any services, you must:**

1. **Create Subnet** → Get Subnet ID and Contract Address
2. **Register Components** → Register Validator, Matcher, Agent to Subnet
3. **Configure .env** → Fill in registration information
4. **Start Services** → Use any method below

### 📝 Important: Environment Variables

The project uses environment variables for configuration. There are two types:

**1. Fixed Values (Protocol-wide, already configured):**
- Smart contract addresses (`PIN_BASE_SEPOLIA_*`)
- Network endpoints (`ROOTLAYER_GRPC`, `ROOTLAYER_HTTP`)
- RPC URL (`CHAIN_RPC_URL`)
- These are **automatically loaded from `.env`** or use **built-in defaults**

**2. User-Specific Values (YOU must configure):**
- Private keys (`TEST_PRIVATE_KEY`, `VALIDATOR_KEYS`)
- Public keys (`VALIDATOR_PUBKEYS`)
- Subnet information (`SUBNET_ID` - after creation)

> ✅ **Good news**: Scripts now automatically load `.env` and provide defaults for fixed addresses!
> ⚠️ **Action required**: You only need to fill in your **private keys and subnet info** in `.env`

### 🔑 Step 1: Generate Validator Keys

**What are these keys?**
- Each validator needs an **ECDSA key pair** (private key + public key) for signing consensus messages
- The private key (64 hex characters) is kept secret and used for signing
- The public key (130 hex characters) is derived from the private key and shared with other validators

> ⚠️ **IMPORTANT**: Before generating keys, decide if you want to use your own funded account or generate new keys and fund them later. You'll need **at least 0.05 ETH on Base Sepolia** to create subnet and register components.

**Generate keys for 3 validators:**

```bash
# 1. Build the derive-pubkey tool
make build

# 2. Generate private keys (using OpenSSL)
PRIVKEY1=$(openssl rand -hex 32)
PRIVKEY2=$(openssl rand -hex 32)
PRIVKEY3=$(openssl rand -hex 32)

# 3. Derive public keys from private keys
PUBKEY1=$(./bin/derive-pubkey "$PRIVKEY1")
PUBKEY2=$(./bin/derive-pubkey "$PRIVKEY2")
PUBKEY3=$(./bin/derive-pubkey "$PRIVKEY3")

# 4. Display the keys for verification
echo "Validator 1:"
echo "  Private: $PRIVKEY1"
echo "  Public:  $PUBKEY1"
echo ""
echo "Validator 2:"
echo "  Private: $PRIVKEY2"
echo "  Public:  $PUBKEY2"
echo ""
echo "Validator 3:"
echo "  Private: $PRIVKEY3"
echo "  Public:  $PUBKEY3"

# 5. Create .env file and save keys
cp .env.example .env
echo "VALIDATOR_KEYS=$PRIVKEY1,$PRIVKEY2,$PRIVKEY3" >> .env
echo "VALIDATOR_PUBKEYS=$PUBKEY1,$PUBKEY2,$PUBKEY3" >> .env

# 6. Generate separate matcher key (SECURITY: never reuse validator keys!)
MATCHER_KEY=$(openssl rand -hex 32)
echo "TEST_PRIVATE_KEY=$MATCHER_KEY" >> .env
```

**Example output:**
```
Validator 1:
  Private: 1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef
  Public:  04abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab

Validator 2:
  Private: fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321
  Public:  04123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef012

Validator 3:
  Private: 0011223344556677889900112233445566778899001122334455667788990011
  Public:  04ffeeddccbbaa998877665544332211ffeeddccbbaa998877665544332211ffeeddccbbaa998877665544332211ffeeddccbbaa998877665544332211ff
```

**Key Format Requirements:**
- ✅ Private key: Exactly **64 hexadecimal characters** (0-9, a-f)
- ✅ Public key: Exactly **130 hexadecimal characters** starting with `04`
- ✅ No `0x` prefix needed
- ❌ Don't use keys with real funds - these are test-only keys!

**Troubleshooting:**
- If `derive-pubkey` says "command not found": Run `make build` first
- If you get "Error decoding private key": Check that your private key is exactly 64 hex characters
- If public key doesn't start with `04`: The key derivation failed, regenerate the private key

**Security Notes:**
- ⚠️ **NEVER commit private keys to git** (`.env` is in `.gitignore`)
- ⚠️ **Use test-only keys with NO real funds**
- ⚠️ Store keys securely - losing them means losing validator access
- ⚠️ Each validator must have a **unique** private key

---

### 📦 Step 2: Create Subnet and Save Information

> ⚠️ **COST**: Creating a subnet costs ~0.001-0.005 ETH in gas fees. Ensure your account (from Step 1) has sufficient Base Sepolia ETH.

**Create your subnet on the blockchain:**

```bash
# For single-node testing (threshold 1/1)
./scripts/create-subnet.sh \
  --name "Test Subnet Single Node" \
  --threshold-num 1 \
  --threshold-denom 1

# For 3-node deployment (threshold 2/3)
./scripts/create-subnet.sh \
  --name "Dev Subnet 3 Nodes" \
  --threshold-num 2 \
  --threshold-denom 3

# For production (threshold 3/4) - this is the default
./scripts/create-subnet.sh --name "Production Subnet"
```

> 💡 **Threshold guideline**: Use 1/1 for single node, 2/3 for 3 nodes, 3/4 for production. See [Understanding Threshold](#understanding-signature-threshold) for details.

**What happens:**
1. Script connects to Base Sepolia blockchain
2. Deploys a new subnet contract with specified threshold
3. Returns Subnet ID and Contract Address
4. **Automatically saves info to `subnet-info-YYYYMMDD-HHMMSS.txt`**

**Example output:**
```
🎉 Subnet created successfully!
   Subnet ID: 0x0000000000000000000000000000000000000000000000000000000000000006
   Contract Address: 0x4cA582Ef4D2B9a474cf3fEf91231d373DeE5cA87
   Transaction: 0xabc123...
   View on Basescan: https://sepolia.basescan.org/tx/0xabc123...

📄 Subnet info saved to: ./subnet-info-20250117-143025.txt
```

**⚠️ IMPORTANT - Save This File!**

The `subnet-info-*.txt` file contains **critical information** you'll need later:
- ✅ Subnet ID (for .env configuration)
- ✅ Contract Address (for participant registration)
- ✅ Registration command template
- ✅ Transaction hash (for verification)

**Managing subnet info files:**

```bash
# View the most recent subnet info
cat subnet-info-*.txt | tail -20

# List all subnet info files
ls -lt subnet-info-*.txt

# Copy to .env for reference
SUBNET_INFO=$(ls -t subnet-info-*.txt | head -1)
grep "Subnet ID:" "$SUBNET_INFO"
grep "Contract Address:" "$SUBNET_INFO"

# Organize multiple subnets
mkdir -p subnets/
mv subnet-info-*.txt subnets/
ls subnets/
```

**Best practices:**
- 📌 Don't delete these files - you'll need them for registration
- 📌 Back up to a safe location (not in git!)
- 📌 Name your subnets clearly during creation for easy identification
- 📌 Keep one file per subnet deployment

---

### Quick Registration Process:

> ⚠️ **COST**: Registration requires **0.03 ETH** (3 components × 0.01 ETH stake each) plus gas fees (~0.003 ETH). Total: **~0.033 ETH**

**Complete workflow from keys to running subnet:**

```bash
# 1. Build tools
make build

# 2. Generate validator keys and create .env
cp .env.example .env

PRIVKEY1=$(openssl rand -hex 32)
PRIVKEY2=$(openssl rand -hex 32)
PRIVKEY3=$(openssl rand -hex 32)
PUBKEY1=$(./bin/derive-pubkey "$PRIVKEY1")
PUBKEY2=$(./bin/derive-pubkey "$PRIVKEY2")
PUBKEY3=$(./bin/derive-pubkey "$PRIVKEY3")

# Generate separate matcher key (SECURITY: never reuse validator keys!)
MATCHER_KEY=$(openssl rand -hex 32)

# Save keys to .env
echo "TEST_PRIVATE_KEY=$MATCHER_KEY" >> .env
echo "VALIDATOR_KEYS=$PRIVKEY1,$PRIVKEY2,$PRIVKEY3" >> .env
echo "VALIDATOR_PUBKEYS=$PUBKEY1,$PUBKEY2,$PUBKEY3" >> .env

# 3. Create Subnet
./scripts/create-subnet.sh --name "My Test Subnet"
# 📝 Save the output: Subnet ID and Contract Address
# Example output:
#   Subnet ID: 0x0000...0006
#   Contract Address: 0x4cA582Ef4D2B9a474cf3fEf91231d373DeE5cA87

# 4. Update .env with subnet information
nano .env
# Add these lines from step 3 output:
#   SUBNET_ID=0x0000...0006
#   SUBNET_CONTRACT=0x4cA582Ef4D2B9a474cf3fEf91231d373DeE5cA87

# 5. Register all components (Validator, Matcher, Agent)
# Note: This single command registers ALL components at once
PRIVATE_KEY="$MATCHER_KEY" ./scripts/register.sh

# Or use environment from .env:
# ./scripts/register.sh  # Auto-loads SUBNET_CONTRACT from .env

# 6. Start services
./scripts/start-subnet.sh
```

**What register.sh does:**
- ✅ Registers **Validator** (stakes 0.01 ETH)
- ✅ Registers **Matcher** (stakes 0.01 ETH)
- ✅ Registers **Agent** (stakes 0.01 ETH)
- All in a **single execution** using the same private key

**Alternative: Manual registration with specific key**
```bash
# If you want to use a different key or subnet contract:
./scripts/register.sh \
  --subnet 0x4cA582Ef... \
  --key your_private_key_here \
  --domain your-subnet.example.com
```

📚 **Detailed Registration Docs**: [`docs/scripts_guide.md`](docs/scripts_guide.md)

---

## Option 1: Docker Deployment (Recommended! ⭐)

**Simplest approach, production-ready 3-node cluster by default**

```bash
# IMPORTANT: macOS users must compile Linux binaries first!
# For Apple Silicon (M1/M2/M3):
make build-linux-arm64

# For Intel Mac or x86_64 servers:
make build-linux-amd64

# Then deploy
cd deployment
./scripts/build-images.sh
./scripts/deploy.sh

# Stop services
docker compose -f docker/docker-compose.yml down
```

📚 **Detailed Documentation**: [`deployment/README.md`](../deployment/README.md) - Complete configuration, testing, and troubleshooting

**Benefits**:
- ✅ 5-minute deployment
- ✅ 3-node by default, production-ready
- ✅ Automatic fault tolerance (tolerates 1 node failure)
- ✅ No need to install Go or other dependencies
- ✅ Fully isolated, clean system environment

**Important for macOS users**:
- ⚠️ **MUST use `make build-linux-*`** instead of `make build`
- `make build` produces Mach-O format (macOS only), which cannot run in Docker Linux containers
- `make build-linux-*` produces ELF format (Linux), which works in Docker

---

## Option 2: Traditional Deployment

**Better performance and control**

### Node Cluster
```bash
# Set environment variables
export NUM_VALIDATORS=3
export VALIDATOR_KEYS=key1,key2,key3
export VALIDATOR_PUBKEYS=pubkey1,pubkey2,pubkey3

# Start
./scripts/start-subnet.sh
```

📚 **Detailed Documentation**: [`docs/scripts_guide.md`](docs/scripts_guide.md)

---

## Option 3: AWS EC2 Deployment

**Production cloud deployment**

```bash
# SSH to EC2 instance
ssh -i your-key.pem ubuntu@YOUR_IP

# Clone and enter project
git clone https://github.com/PIN-AI/Subnet.git
cd Subnet

# Choose Docker or traditional method (see above)
```

📚 **Detailed Documentation**: [`docs/environment_setup.md`](docs/environment_setup.md)

---

## 📁 Project Structure

```
Subnet/
├── deployment/               # Production deployment files (Docker)
│   ├── Dockerfile            # Docker image definition
│   ├── docker-compose.yml    # 3-node cluster config
│   ├── docker-compose-dev.yml# Single-node config
│   ├── docker-start.sh       # 3-node startup script
│   ├── docker-start-dev.sh   # Single-node startup script
│   └── README.md             # Complete Docker docs (with 3-node details)
├── scripts/                   # Traditional deployment scripts
│   └── start-subnet.sh       # Startup script
├── docs/                      # Documentation
│   ├── scripts_guide.md      # Scripts usage guide
│   └── environment_setup.md  # Environment configuration
└── .env                      # Configuration file (create this)
```

---

## 🎯 Recommended Choice

| Scenario | Recommended Method | Command |
|----------|-------------------|---------|
| **Quick Testing** | Docker single-node | `cd docker && ./docker-start-dev.sh` |
| **Production** | Docker 3-node | `cd docker && ./docker-start.sh` |
| **Best Performance** | Traditional 3-node | `./scripts/start-subnet.sh` |

---

## 📋 Configuration Requirements

### Required Environment Variables (.env)

**What you MUST configure in .env:**

```bash
# ============================================================
# USER-SPECIFIC VALUES (REQUIRED - FILL IN)
# ============================================================
# Matcher/Test private key
TEST_PRIVATE_KEY=your_key_here

# Validator keys (comma-separated for multi-node, or single key for single-node)
VALIDATOR_KEYS=key1,key2,key3
VALIDATOR_PUBKEYS=pubkey1,pubkey2,pubkey3

# Your subnet information (from create-subnet.sh output)
SUBNET_ID=0x0000000000000000000000000000000000000000000000000000000000000002

# ============================================================
# FIXED VALUES (Already configured in .env.example)
# ============================================================
# These are protocol-wide and should NOT be changed:
# - PIN_BASE_SEPOLIA_INTENT_MANAGER
# - PIN_BASE_SEPOLIA_SUBNET_FACTORY
# - PIN_BASE_SEPOLIA_STAKING_MANAGER
# - PIN_BASE_SEPOLIA_CHECKPOINT_MANAGER
# - ROOTLAYER_GRPC=3.17.208.238:9001
# - ROOTLAYER_HTTP=http://3.17.208.238:8081/api/v1
# - CHAIN_RPC_URL=https://sepolia.base.org
```

**Format 2: Individual variables**
```bash
# Matcher key
TEST_PRIVATE_KEY=your_key_here

# Validator keys (3 nodes need 3 keys)
VALIDATOR_KEY_1=your_key_1
VALIDATOR_PUBKEY_1=your_pubkey_1
VALIDATOR_KEY_2=your_key_2
VALIDATOR_PUBKEY_2=your_pubkey_2
VALIDATOR_KEY_3=your_key_3
VALIDATOR_PUBKEY_3=your_pubkey_3

# Subnet configuration
SUBNET_ID=0x0000000000000000000000000000000000000000000000000000000000000003
ROOTLAYER_GRPC=3.17.208.238:9001
ROOTLAYER_HTTP=http://3.17.208.238:8081/api/v1
```

> Format 2 exists for legacy Docker/deployment scripts. The `start-subnet.sh` launcher requires the comma-separated Format 1 variables.

> ⚠️ **Contract Addresses:** Base Sepolia addresses were updated on 2025‑11‑03. Copy the latest values from `.env.example` to avoid using deprecated contracts.

**Key Source**:
Keys must be those generated and registered in steps 3-4 of the "Quick Registration Process". Do not randomly generate new keys!

```bash
# Example: If you used these keys during registration
VALIDATOR_KEYS=abc123...,def456...,ghi789...
VALIDATOR_PUBKEYS=0x123...,0x456...,0x789...
```

---

## 🆘 Quick Help

### Docker Method
```bash
# View logs
cd docker && docker compose logs -f

# View status
docker compose ps

# Restart services
docker compose restart

# Stop and clean up
docker compose down -v
```

### Traditional Method
```bash
# View logs
tail -f subnet-logs/*.log

# Stop services
pkill -f 'bin/matcher|bin/validator|bin/simple-agent'
```

---

## 📋 Log Files and Debugging

### Log Directory Structure

After starting the subnet, logs are stored in the `./subnet-logs/` directory:

```
subnet-logs/
├── matcher.log           # Matcher service logs (intent ingestion, bid matching)
├── validator-1.log       # Validator 1 logs (consensus, validation, checkpoint)
├── validator-2.log       # Validator 2 logs
├── validator-3.log       # Validator 3 logs
├── agent.log             # Agent logs (task execution, result submission)
└── rootlayer.log         # RootLayer mock logs (if using mock-rootlayer)
```

**Note**: Docker deployments store logs internally. Use `docker compose logs -f [service]` instead.

### Viewing Specific Logs

```bash
# Watch all logs (verbose)
tail -f subnet-logs/*.log

# Watch specific service
tail -f subnet-logs/matcher.log      # Matcher only
tail -f subnet-logs/validator-1.log  # Validator 1 only
tail -f subnet-logs/agent.log        # Agent only

# Watch multiple specific services
tail -f subnet-logs/matcher.log subnet-logs/validator-1.log

# Search for specific events
grep "intent_id" subnet-logs/matcher.log          # Find intent processing
grep "ValidationBundle" subnet-logs/validator-*.log  # Find consensus activity
grep "Executing task" subnet-logs/agent.log       # Find agent task execution

# Docker logs (container-based)
docker compose logs -f                   # All services
docker compose logs -f validator-1       # Specific validator
docker compose logs -f matcher agent     # Multiple services
docker compose logs --tail=100 validator-1  # Last 100 lines
```

### Key Log Patterns

**What to look for in each log:**

| Service | Log File | Key Events |
|---------|----------|------------|
| **Matcher** | `matcher.log` | `Received intent`, `Received bid`, `Selected winner`, `Created assignment` |
| **Validator** | `validator-*.log` | `Processed execution report`, `Creating ValidationBundle`, `Submitting ValidationBundle`, `Checkpoint submitted` |
| **Agent** | `agent.log` | `Received task`, `Executing task`, `Submitting execution report`, `Task completed` |

**Example: Tracing an intent through the system:**

```bash
# 1. Check matcher received the intent
grep "Received intent" subnet-logs/matcher.log | tail -1

# 2. Check bids were received and winner selected
grep "Selected winner" subnet-logs/matcher.log | tail -1

# 3. Check agent received the task
grep "Received task" subnet-logs/agent.log | tail -1

# 4. Check agent submitted execution report
grep "Submitting execution report" subnet-logs/agent.log | tail -1

# 5. Check validators processed the report
grep "Processed execution report" subnet-logs/validator-*.log | tail -3

# 6. Check consensus and bundle submission
grep "ValidationBundle" subnet-logs/validator-*.log | tail -5
```

### Log Levels

Logs use standard levels: `DEBUG`, `INFO`, `WARN`, `ERROR`

```bash
# Filter by log level
grep "ERROR" subnet-logs/*.log           # Show only errors
grep "WARN\|ERROR" subnet-logs/*.log     # Show warnings and errors
grep "INFO.*intent" subnet-logs/matcher.log  # Info logs about intents
```

### Common Debugging Scenarios

**1. Intent not being processed:**
```bash
# Check if matcher received it
grep "Received intent" subnet-logs/matcher.log

# Check if any bids came in
grep "Received bid" subnet-logs/matcher.log

# Check for errors
grep "ERROR" subnet-logs/matcher.log
```

**2. Agent not receiving tasks:**
```bash
# Check if agent is connected
grep "Connected to matcher" subnet-logs/agent.log

# Check matcher assigned the task
grep "Created assignment" subnet-logs/matcher.log
```

**3. Validation not completing:**
```bash
# Check if validators received reports
grep "execution report" subnet-logs/validator-*.log

# Check consensus progress
grep "ValidationBundle" subnet-logs/validator-*.log

# Check for errors
grep "ERROR" subnet-logs/validator-*.log
```

### Log Retention

**Default behavior:**
- Logs are appended to existing files
- No automatic rotation (files grow indefinitely)

**Managing log size:**
```bash
# Check log sizes
du -h subnet-logs/

# Clear old logs (stops services first!)
pkill -f 'bin/matcher|bin/validator|bin/simple-agent'
rm -rf subnet-logs/*.log
./scripts/start-subnet.sh

# Archive logs
mkdir -p logs-archive/$(date +%Y%m%d)
cp subnet-logs/*.log logs-archive/$(date +%Y%m%d)/
> subnet-logs/*.log  # Truncate current logs
```

📚 **Detailed flow tracing**: See [`docs/subnet_deployment_guide.md`](subnet_deployment_guide.md#intent-execution-flow--observability) for complete intent execution flow with log examples.

---

## 📚 Detailed Documentation

- **Docker Deployment**: [`deployment/README.md`](../deployment/README.md) - Complete Docker usage guide (with 3-node cluster details)
- **Scripts Guide**: [`docs/scripts_guide.md`](docs/scripts_guide.md) - All scripts documentation
- **Environment Setup**: [`docs/environment_setup.md`](docs/environment_setup.md) - Dependency installation
- **Architecture**: [`docs/architecture.md`](docs/architecture.md) - System architecture

---

## 🎉 Get Started

**Complete workflow (including registration)**:

```bash
# 0. Build tools
make build

# 1. Create Subnet (scripts auto-load .env with fixed addresses)
./scripts/create-subnet.sh --name "My Subnet"
# 📝 Save the Subnet ID and Contract Address from output

# 2. Register components (scripts auto-load .env)
./scripts/register.sh --subnet <SUBNET_CONTRACT> --key <KEY>

# 3. Generate validator keys and configure .env
# See "Step 1: Generate Validator Keys" section above for detailed instructions
cp .env.example .env

# Generate validator keys
PRIVKEY1=$(openssl rand -hex 32)
PRIVKEY2=$(openssl rand -hex 32)
PRIVKEY3=$(openssl rand -hex 32)
PUBKEY1=$(./bin/derive-pubkey "$PRIVKEY1")
PUBKEY2=$(./bin/derive-pubkey "$PRIVKEY2")
PUBKEY3=$(./bin/derive-pubkey "$PRIVKEY3")

# Generate separate matcher key (SECURITY: never reuse validator keys!)
MATCHER_KEY=$(openssl rand -hex 32)

# Save to .env
echo "TEST_PRIVATE_KEY=$MATCHER_KEY" >> .env
echo "VALIDATOR_KEYS=$PRIVKEY1,$PRIVKEY2,$PRIVKEY3" >> .env
echo "VALIDATOR_PUBKEYS=$PUBKEY1,$PUBKEY2,$PUBKEY3" >> .env

# Then manually add subnet info from step 1:
nano .env
# Fill in:
#   - SUBNET_ID (from step 1)
# Note: Keys are already set above, fixed addresses are pre-configured!

# 4. Start (Docker 3-node)
cd docker && ./docker-start.sh

# 5. View logs
docker compose logs -f
```

✅ **Simplified Configuration**:
- Scripts automatically load `.env` and provide defaults for fixed contract addresses
- You only need to configure your **private keys and subnet info**
- No need to manually set `PIN_BASE_SEPOLIA_*` addresses anymore!

⚠️ **Important**: You must complete blockchain registration (Create Subnet + Register components) before starting services!

That's it! 🚀

---

Need help? Check `deployment/README.md` or detailed docs in the `docs/` directory.

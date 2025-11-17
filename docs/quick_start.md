# 🚀 PinAI Subnet - Quick Start

## 🤔 Which Guide Should I Follow?

**Quick decision table:**

| I want to... | Use this guide | Time |
|-------------|----------------|------|
| 🎯 Quick test/demo (first time) | [Docker Deployment](../docker/README.md) ⭐ Recommended | 5 min |
| 🔧 Full control over setup | [Manual Deployment](#option-2-traditional-deployment) | 15 min |
| 📦 Build custom agent | [Agent SDK Docs](https://github.com/PIN-AI/subnet-sdk) | - |
| 🏭 Deploy to production | [Production Guide](subnet_deployment_guide.md#production-deployment) | - |
| 🔍 Fix issues | [Troubleshooting](subnet_deployment_guide.md#troubleshooting) | - |

> 💡 **New users**: Start with Docker deployment (Option 1 below), then explore customization after you understand the flow.

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

### Quick Registration Process:

```bash
# 1. Build tools
make build

# 2. Create Subnet (script auto-loads .env with fixed contract addresses)
./scripts/create-subnet.sh --name "My Test Subnet"
# 📝 Record the output Subnet ID and Contract Address

# 3. Generate keys (if you don't have any)
openssl rand -hex 32  # Generate Validator 1 private key
openssl rand -hex 32  # Generate Validator 2 private key
openssl rand -hex 32  # Generate Validator 3 private key
# 📝 Save these private keys!

# 4. Register each Validator (script auto-loads .env with fixed addresses)
./scripts/register.sh \
  --subnet <SUBNET_CONTRACT_ADDRESS> \
  --key <VALIDATOR_1_KEY> \
  --domain validator1.test.pinai.xyz
# Repeat for validator 2 and 3 with different keys

# 5. Configure .env (fill in registered keys and subnet info)
cp .env.example .env
nano .env
# Fill in:
#   - TEST_PRIVATE_KEY (your matcher/test key)
#   - VALIDATOR_KEYS (comma-separated: key1,key2,key3)
#   - VALIDATOR_PUBKEYS (derive using: ./bin/derive-pubkey <key>)
#   - SUBNET_ID (from step 2)
#   - BLOCKCHAIN_SUBNET_CONTRACT (from step 2)
# Note: Fixed addresses (PIN_BASE_SEPOLIA_*) are already set correctly
```

📚 **Detailed Registration Docs**: [`docs/scripts_guide.md`](docs/scripts_guide.md)

---

## Option 1: Docker Deployment (Recommended! ⭐)

**Simplest approach, production-ready 3-node cluster by default**

```bash
# Start 3-node Raft cluster
cd docker && ./docker-start.sh

# Or single-node dev mode
cd docker && ./docker-start-dev.sh

# Stop services
cd docker && docker compose down
```

📚 **Detailed Documentation**: [`docker/README.md`](docker/README.md) - Complete configuration, testing, and troubleshooting

**Benefits**:
- ✅ 5-minute deployment
- ✅ 3-node by default, production-ready
- ✅ Automatic fault tolerance (tolerates 1 node failure)
- ✅ No need to install Go or other dependencies
- ✅ Fully isolated, clean system environment

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
├── docker/                    # Docker deployment files
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
SUBNET_ID=0x0000000000000000000000000000000000000000000000000000000000000006
BLOCKCHAIN_SUBNET_CONTRACT=0x4cA582Ef4D2B9a474cf3fEf91231d373DeE5cA87

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
ROOTLAYER_HTTP=http://3.17.208.238:8081
```

> Format 2 exists for legacy Docker/deployment scripts. The `start-subnet.sh` launcher requires the comma-separated Format 1 variables.

> ⚠️ **Contract Addresses:** Base Sepolia addresses were updated on 2025‑11‑03. Copy the latest values from `.env.example` or `deployment/env.template` to avoid using deprecated contracts.

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
pkill -f 'bin/matcher|bin/validator|bin/registry'
```

---

## 📚 Detailed Documentation

- **Docker Deployment**: [`docker/README.md`](docker/README.md) - Complete Docker usage guide (with 3-node cluster details)
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

# 3. Configure .env
cp .env.example .env
nano .env
# Fill in ONLY user-specific values:
#   - TEST_PRIVATE_KEY (your key)
#   - VALIDATOR_KEYS=key1,key2,key3 (comma-separated)
#   - VALIDATOR_PUBKEYS=pubkey1,pubkey2,pubkey3 (derive using ./bin/derive-pubkey)
#   - SUBNET_ID (from step 1)
#   - BLOCKCHAIN_SUBNET_CONTRACT (from step 1)
# Note: Fixed addresses are already configured correctly!

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

Need help? Check `docker/README.md` or detailed docs in the `docs/` directory.

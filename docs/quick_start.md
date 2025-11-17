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

### 🔑 Step 1: Generate Validator Keys

**What are these keys?**
- Each validator needs an **ECDSA key pair** (private key + public key) for signing consensus messages
- The private key (64 hex characters) is kept secret and used for signing
- The public key (130 hex characters) is derived from the private key and shared with other validators

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
  Private: EXAMPLE_PRIVATE_KEY_DO_NOT_USE_1234567890ABCDEF1234567890ABCDEF
  Public:  0482ea12c5481d481c7f9d7c1a2047401c6e2f855e4cee4d8df0aa197514f3456528ba6c55092b20b51478fd8cf62cde37f206621b3dd47c2be3d5c35e4889bf94

Validator 2:
  Private: abc123...
  Public:  04def456...

Validator 3:
  Private: 789ghi...
  Public:  04jkl012...
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

**Create your subnet on the blockchain:**

```bash
# Create subnet (script auto-loads .env with fixed contract addresses)
./scripts/create-subnet.sh --name "My Test Subnet"
```

**What happens:**
1. Script connects to Base Sepolia blockchain
2. Deploys a new subnet contract
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

## 📋 Log Files and Debugging

### Log Directory Structure

After starting the subnet, logs are stored in the `./subnet-logs/` directory:

```
subnet-logs/
├── matcher.log           # Matcher service logs (intent ingestion, bid matching)
├── validator-1.log       # Validator 1 logs (consensus, validation, checkpoint)
├── validator-2.log       # Validator 2 logs
├── validator-3.log       # Validator 3 logs
├── registry.log          # Registry service logs (participant discovery)
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
| **Registry** | `registry.log` | `Registered participant`, `Participant lookup` |

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

# Check registration
grep "Registered" subnet-logs/registry.log

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
pkill -f 'bin/matcher|bin/validator|bin/registry'
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

Need help? Check `docker/README.md` or detailed docs in the `docs/` directory.

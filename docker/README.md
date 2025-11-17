# 🐳 Docker Deployment - 3 Steps to Launch!

**📍 You are here:** First-Time Setup → Docker Deployment (Recommended)

**Prerequisites:**
- Completed [Environment Setup](../docs/environment_setup.md) - Docker installed
- Have subnet registration info (Subnet ID, validator keys)

**Time to complete:** ~5 minutes

**What you'll learn:**
- Deploy 3-node validator cluster with Docker Compose
- Configure environment variables
- Verify deployment and monitor logs

**Next steps:**
- ✅ After deployment → [Verify Intent Flow](../docs/subnet_deployment_guide.md#intent-execution-flow--observability)
- 🔧 Customize → [Development Guide](../docs/subnet_deployment_guide.md#custom-development-guide)

---

The simplest deployment method, 3-node Raft cluster by default, just 3 commands!

---

## ⚠️ Important: Prerequisites

**Before starting Docker services, you must complete blockchain registration!**

### Registration Process (in order):

#### 1️⃣ Create Subnet
```bash
# Build tools
make build

# Create Subnet (requires test tokens)
./scripts/create-subnet.sh --name "My Test Subnet"

# Record the output:
# - Subnet ID (e.g.: 0x0000...0003)
# - Subnet Contract Address (e.g.: 0x5697DFA...)
```

#### 2️⃣ Register Components (Validator, Matcher, Agent)

**If you don't have keys yet, generate 3 validator keys first:**
```bash
# Generate 3 validator keys
for i in 1 2 3; do
  KEY=$(openssl rand -hex 32)
  echo "Validator $i Private Key: $KEY"
  # Save these keys! You'll need them to register and start services
done
```

**Then register each validator with these keys:**
```bash
# Register each validator (need to register 3 times, once for each key)
./scripts/register.sh \
  --subnet <SUBNET_CONTRACT_ADDRESS> \
  --key <VALIDATOR_1_PRIVATE_KEY> \
  --domain validator1.test.pinai.xyz

# Repeat for validator 2 and validator 3 with different keys
```

💡 **Tip**: Save these private keys! You'll need them later when starting services.

#### 3️⃣ Configure .env
```bash
# Fill in registration info to .env file
nano .env
```

📚 **Detailed Registration Docs**: 
- [`docs/scripts_guide.md`](../docs/scripts_guide.md) - Complete registration instructions
- [`scripts/README.md`](../scripts/README.md) - All scripts usage

---

## Quick Start

### 1. Install Docker

```bash
# Ubuntu/Debian
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER
newgrp docker

# macOS
brew install --cask docker
```

### 2. Configure Environment

Edit `.env` file to configure 3 validator keys:

```bash
cd ~/Subnet
nano .env
```

**Required Configuration** - Two formats supported:

**Format 1: Comma-separated (Recommended, compatible with traditional scripts)**
```bash
# Matcher key
TEST_PRIVATE_KEY=your_matcher_private_key_here

# Validator keys (comma-separated)
VALIDATOR_KEYS=key1,key2,key3
VALIDATOR_PUBKEYS=pubkey1,pubkey2,pubkey3

# Subnet configuration
SUBNET_ID=0x...
ROOTLAYER_GRPC=3.17.208.238:9001
ROOTLAYER_HTTP=http://3.17.208.238:8081
```

**Format 2: Individual variables**
```bash
# Matcher key
TEST_PRIVATE_KEY=your_matcher_private_key_here

# Validator 1
VALIDATOR_KEY_1=your_validator1_private_key
VALIDATOR_PUBKEY_1=your_validator1_public_key

# Validator 2
VALIDATOR_KEY_2=your_validator2_private_key
VALIDATOR_PUBKEY_2=your_validator2_public_key

# Validator 3
VALIDATOR_KEY_3=your_validator3_private_key
VALIDATOR_PUBKEY_3=your_validator3_public_key

# Subnet configuration
SUBNET_ID=0x...
ROOTLAYER_GRPC=3.17.208.238:9001
ROOTLAYER_HTTP=http://3.17.208.238:8081
```

💡 **Tip**: Both formats are supported! If you're already using comma-separated format from traditional scripts, you can use it directly.

**⚠️ Important Notes**:
- `SUBNET_ID` and `SUBNET_CONTRACT_ADDRESS` come from step 1 "Create Subnet" output
- **Validator private keys must be keys already registered on the blockchain**
- Do not randomly generate new keys! Must use keys from registration

**Key Source**:
Keys should come from the private keys used in step 2 "Register Components". If you generated new keys during registration, make sure to save them and fill them in here.

### 3. Launch!

```bash
# Start 3-node cluster (from docker directory)
cd ~/Subnet/docker
./docker-start.sh

# Or dev mode (single node)
cd ~/Subnet/docker
./docker-start-dev.sh
```

That's it! ✨

**Recommendation**: Use default 3-node for production, single-node for development testing.

---

## Manual Startup

If you prefer manual control:

```bash
# Start 3-node cluster (default)
docker compose up -d

# Or start single-node (dev mode)
docker compose -f docker-compose-dev.yml up -d

# View logs
docker compose logs -f

# Stop services
docker compose down
```

---

## Common Commands

```bash
# View running status
docker compose ps

# View logs
docker compose logs -f matcher
docker compose logs -f validator

# Restart services
docker compose restart

# Enter container for debugging
docker compose exec validator sh

# Stop and clean up
docker compose down -v
```

---

## Service Addresses and Architecture

### 3-Node Cluster (Default)

**Architecture**:
```
Registry (:8101) → Matcher (:8090/:8092) → 3 Validators (Raft consensus)
                                             ├─ Validator-1 (:9090)
                                             ├─ Validator-2 (:9091)
                                             └─ Validator-3 (:9092)
```

**Service Ports**:
- **Registry**: gRPC :8091, HTTP :8101
- **Matcher**: gRPC :8090, HTTP :8092  
- **Validator-1**: gRPC :9090, Raft :7400, Gossip :7950
- **Validator-2**: gRPC :9091, Raft :7401, Gossip :7951
- **Validator-3**: gRPC :9092, Raft :7402, Gossip :7952

**Raft Consensus**:
- Threshold: 2/3 (at least 2 out of 3 nodes must agree)
- Fault tolerance: Can tolerate 1 node failure
- Automatic leader election

### Single Node (Dev Mode)
- **Validator gRPC**: localhost:9090

---

## Advantages

✅ **Simple**: One command starts 3-node cluster  
✅ **Fast**: 5-minute deployment  
✅ **Production-ready**: Default 3-node Raft, strong fault tolerance  
✅ **Isolated**: Doesn't pollute system environment  
✅ **Reliable**: Automatic restart and health checks  
✅ **Portable**: Run anywhere Docker is supported  
✅ **Flexible**: Supports single-node dev mode  

---

## Documentation

For detailed documentation, check:
- [Quick Start](../docs/quick_start.md) - Complete deployment guide
- [Scripts Guide](../docs/scripts_guide.md) - All scripts documentation
- [Scripts Documentation](../scripts/README.md) - Scripts usage reference
- [Architecture](../docs/architecture.md) - System architecture

---

## Testing and Validation

### Verify Raft Consensus
```bash
cd docker

# View leader election
docker compose logs validator-1 | grep -i "leader"

# Test fault tolerance: stop one node
docker stop pinai-validator-3
# Cluster should continue working (2/3 nodes normal)

# Recover node
docker start pinai-validator-3
# Node will automatically sync state
```

### View Consensus Logs
```bash
# View Raft consensus process
docker compose logs -f validator-1 validator-2 validator-3 | grep -i "raft\|consensus"

# View each node status
docker compose exec validator-1 sh -c "ps aux | grep validator"
```

---

## Troubleshooting

**Problem 1: Port already in use**
```bash
# Check port
lsof -i :9090

# Modify port mapping (edit docker-compose.yml)
ports:
  - "19090:9090"  # Use different host port
```

**Problem 2: Container startup failed**
```bash
# View detailed logs
docker compose logs validator-1

# Check environment variables
docker compose config | grep VALIDATOR_KEY

# Verify configuration
cat ../.env | grep VALIDATOR
```

**Problem 3: Cannot connect to RootLayer**
```bash
# Test connection
docker compose exec validator-1 curl -v $ROOTLAYER_HTTP/health

# Check network
docker compose exec validator-1 ping 3.17.208.238
```

**Problem 4: Raft cannot reach consensus**
```bash
# Check all nodes are running
docker compose ps

# View Raft logs
docker compose logs validator-1 validator-2 validator-3 | grep -i "raft\|election"

# Restart cluster
docker compose restart validator-1 validator-2 validator-3
```

---

## Deployment Comparison

| Feature | Traditional | **Docker 3-node** | Docker Single |
|---------|-------------|------------------|---------------|
| Install Time | 15-30 min | **5 min** ✨ | 5 min |
| Fault Tolerance | ✅ (needs config) | **✅ 1 node failure** | ❌ |
| Dependency Mgmt | Manual install | **Automatic** ✨ | Automatic |
| Startup Cmd | Multiple scripts | **One command** ✨ | One command |
| Isolation | ❌ | **✅** | ✅ |
| Resource Usage | Medium | **Medium** | Low |
| Recommended Use | Distributed deploy | **Production** ✨ | Dev/Test |

**Production-ready by default!** 🚀

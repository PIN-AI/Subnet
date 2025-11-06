# 🚀 PinAI Subnet - Production Deployment Quick Start

**Binary distribution for production environments - No source code required!**

---

## 📋 Prerequisites

- Docker & Docker Compose installed
- Pre-compiled binaries (or distribution package)
- Server with 8GB+ RAM recommended

---

## ⚡ Quick Deploy (3 Steps)

### Step 1: Build Images (Developer Only)

```bash
# On development machine
cd /Users/ty/pinai/protocol/Subnet

# Compile binaries
make build

# Build Docker image
cd deployment
./scripts/build-images.sh
```

### Step 2: Configure Environment

```bash
# Copy template
cp env.template .env

# Edit configuration
nano .env
```

**Required settings:**
- `VALIDATOR_KEY_1/2/3` - Validator private keys
- `VALIDATOR_PUBKEY_1/2/3` - Validator public keys
- `TEST_PRIVATE_KEY` - Matcher private key
- `INTENT_MANAGER_ADDR` - Contract address

### Step 3: Deploy

```bash
./scripts/deploy.sh
```

Done! 🎉

---

## 📦 Distribution Package Method

### Create Distribution Package

```bash
# On development machine
cd deployment
./scripts/export-images.sh

# Creates: pinai-subnet-dist-YYYYMMDD-HHMMSS.tar.gz
```

### Deploy on Target Server

```bash
# Transfer package
scp pinai-subnet-dist-*.tar.gz user@server:/opt/

# On target server
cd /opt
tar xzf pinai-subnet-dist-*.tar.gz
cd pinai-subnet-dist-*/

# Install
./install.sh

# Configure
cp env.template .env
nano .env

# Deploy
./scripts/deploy.sh
```

---

## 🏗️ Architecture

```
deployment/
├── .env                    # Configuration (NOT in git)
├── docker/
│   ├── Dockerfile          # Binary-only image
│   ├── docker-compose.yml  # Service definitions
│   └── entrypoint.sh       # Startup script
├── config/                 # Mounted into containers (read-only)
│   ├── matcher.yaml
│   ├── auth_config.yaml
│   └── policy_config.yaml
├── data/                   # Persistent volumes (NOT in git)
│   ├── registry/
│   ├── matcher/
│   └── validator-{1,2,3}/
└── scripts/
    ├── build-images.sh     # Build from binaries
    ├── deploy.sh           # Deploy services
    └── export-images.sh    # Create distribution
```

**Key Design Points:**

✅ **Binaries in Docker** - Pre-compiled, ready to run
✅ **Config as Volumes** - Easy to update without rebuilding
✅ **Data Persistence** - Volumes for runtime data
✅ **Security** - `.env` never committed, configs read-only
✅ **Distribution** - Single tarball with everything needed

---

## 🔐 Security Checklist

- [ ] `.env` file has correct permissions (`chmod 600 .env`)
- [ ] Private keys are generated securely (`openssl rand -hex 32`)
- [ ] `.env` is in `.gitignore`
- [ ] Config files mounted as read-only (`:ro`)
- [ ] Containers run as non-root user
- [ ] Firewall configured for required ports only

---

## 🌐 Service Endpoints

After deployment:

| Service | Port | URL/Address |
|---------|------|-------------|
| Registry | 8101 | http://localhost:8101/agents |
| Matcher gRPC | 8093 | localhost:8093 |
| Matcher HTTP | 8094 | http://localhost:8094/health |
| Validator-1 | 9090 | localhost:9090 |
| Validator-2 | 9091 | localhost:9091 |
| Validator-3 | 9092 | localhost:9092 |

---

## 🛠️ Management Commands

```bash
# View all logs
docker compose -f docker/docker-compose.yml logs -f

# View specific service
docker compose -f docker/docker-compose.yml logs -f validator-1

# Check status
docker compose -f docker/docker-compose.yml ps

# Restart service
docker compose -f docker/docker-compose.yml restart validator-1

# Stop all
docker compose -f docker/docker-compose.yml down

# Stop and remove data
docker compose -f docker/docker-compose.yml down -v
```

---

## 🔄 Update Deployment

### Update Configuration Only

```bash
# Edit config
nano config/matcher.yaml

# Restart affected service
docker compose -f docker/docker-compose.yml restart matcher
```

### Update Binaries

```bash
# On development machine
make build
cd deployment
./scripts/build-images.sh
./scripts/export-images.sh

# Transfer to production
scp pinai-subnet-dist-*.tar.gz user@server:/opt/

# On production
cd /opt/pinai-subnet-dist-*/
./install.sh
docker compose -f docker/docker-compose.yml down
docker compose -f docker/docker-compose.yml up -d
```

---

## 🐛 Troubleshooting

### Check Service Health

```bash
# Registry
curl http://localhost:8101/agents

# Matcher
curl http://localhost:8094/health

# Validators
nc -zv localhost 9090
nc -zv localhost 9091
nc -zv localhost 9092
```

### View Logs

```bash
# All services
docker compose -f docker/docker-compose.yml logs --tail=100

# Specific service with timestamps
docker compose -f docker/docker-compose.yml logs -f --timestamps validator-1
```

### Check Raft Consensus

```bash
docker compose -f docker/docker-compose.yml logs validator-1 | grep -i "raft\|leader"
```

### Container Won't Start

```bash
# Check container logs
docker logs pinai-validator-1

# Check environment variables
docker exec pinai-validator-1 env

# Check mounted volumes
docker inspect pinai-validator-1 | grep -A 10 Mounts
```

---

## 📊 Resource Usage

Typical resource usage per service:

| Service | CPU | Memory | Disk |
|---------|-----|--------|------|
| Registry | ~5% | ~50MB | ~10MB |
| Matcher | ~10% | ~100MB | ~50MB |
| Validator | ~15% | ~200MB | ~500MB |

**Total for 3-validator cluster:**
- CPU: ~60%
- Memory: ~800MB
- Disk: ~2GB (with data)

---

## 🎯 Differences from Development Setup

| Aspect | Development | Production (This) |
|--------|-------------|-------------------|
| Docker image | Source + build tools | Binaries only |
| Image size | ~300MB | ~80MB |
| Build time | 5-10 min | < 1 min |
| Config | In image | Mounted volumes |
| Data | Ephemeral | Persistent volumes |
| .env | Maybe shared | Never shared |
| Updates | Rebuild everything | Replace binaries |

---

## 📞 Support

For issues:
1. Check logs: `docker compose -f docker/docker-compose.yml logs -f`
2. Review configuration: `cat .env`
3. Verify network: `docker network inspect deployment_subnet-network`

---

**Remember:** This is a production setup. Keep your `.env` file secure! 🔒


# Production Deployment Architecture

## 🎯 Design Philosophy

**Core Principle:** Pre-compiled binaries + External configuration + Volume persistence

This architecture separates concerns for production deployment:
- **Binaries**: In Docker image (immutable)
- **Configuration**: Mounted from host (mutable)
- **Data**: Persistent volumes (stateful)
- **Secrets**: Environment variables (never in image)

---

## 📁 Directory Structure

```
deployment/
├── .env                       # ⚠️  NEVER commit (contains private keys)
├── .gitignore                 # Protects .env and data/
│
├── README.md                  # Overview and instructions
├── QUICKSTART.md              # Quick start guide
├── ARCHITECTURE.md            # This file
│
├── docker/                    # Docker configuration
│   ├── Dockerfile             # Binary-only image (NO source code)
│   ├── docker-compose.yml     # Service definitions with volumes
│   └── entrypoint.sh          # Container startup script
│
├── config/                    # ✅ Mounted into containers (read-only)
│   ├── matcher.yaml           # Matcher service configuration
│   ├── auth_config.yaml       # Authentication policies
│   └── policy_config.yaml     # Validation policies
│
├── data/                      # ✅ Persistent volumes (NOT in git)
│   ├── registry/              # Registry data
│   ├── matcher/               # Matcher data
│   ├── validator-1/           # Validator 1 data
│   ├── validator-2/           # Validator 2 data
│   └── validator-3/           # Validator 3 data
│
└── scripts/                   # Deployment automation
    ├── build-images.sh        # Build Docker image from binaries
    ├── deploy.sh              # Deploy to production
    └── export-images.sh       # Create distribution package
```

---

## 🏗️ Architecture Layers

### Layer 1: Docker Image (Immutable)

```
pinai-subnet:latest
├── /app/bin/              # Pre-compiled binaries
│   ├── validator          # Validator service
│   ├── matcher            # Matcher service
│   ├── registry           # Registry service
│   └── simple-agent       # Agent binary
├── /app/entrypoint.sh     # Startup script
└── Runtime dependencies   # Alpine + tools
```

**Characteristics:**
- Size: ~80MB
- Build once, run anywhere
- No source code included
- No secrets included

### Layer 2: Configuration (External Mount)

```
Host: ./config/            →  Container: /app/config/ (read-only)
├── matcher.yaml           →  /app/config/matcher.yaml:ro
├── auth_config.yaml       →  /app/config/auth_config.yaml:ro
└── policy_config.yaml     →  /app/config/policy_config.yaml:ro
```

**Characteristics:**
- Mounted as read-only volumes
- Can be updated without rebuilding image
- Version controlled (safe to commit)
- Supports environment variable substitution

### Layer 3: Runtime Data (Persistent Volumes)

```
Host: ./data/              →  Container: /app/data/ (read-write)
├── registry/              →  /app/data/ (registry container)
├── matcher/               →  /app/data/ (matcher container)
└── validator-{1,2,3}/     →  /app/data/ (validator containers)
```

**Characteristics:**
- Persistent across container restarts
- NOT in version control
- Backed up separately
- Container-specific isolation

### Layer 4: Secrets (Environment Variables)

```
Host: .env                 →  Container: Environment variables
├── VALIDATOR_KEY_1        →  ${VALIDATOR_KEY_1}
├── VALIDATOR_KEY_2        →  ${VALIDATOR_KEY_2}
├── TEST_PRIVATE_KEY       →  ${TEST_PRIVATE_KEY}
└── ...                    →  ...
```

**Characteristics:**
- **NEVER** in image
- **NEVER** in git
- Loaded at runtime
- chmod 600 permissions

---

## 🔄 Data Flow

### 1. Build Time (Developer)

```
Source Code
    ↓
make build
    ↓
bin/ directory
    ↓
./scripts/build-images.sh
    ↓
Docker Image (pinai-subnet:latest)
    ↓
./scripts/export-images.sh
    ↓
Distribution Package (.tar.gz)
```

### 2. Deployment Time (User)

```
Distribution Package
    ↓
Extract & Install
    ↓
Configure .env
    ↓
./scripts/deploy.sh
    ↓
Docker Compose Up
    ↓
    ├─→ Load image
    ├─→ Mount config/ (read-only)
    ├─→ Mount data/ (read-write)
    ├─→ Inject env vars from .env
    └─→ Start services
```

### 3. Runtime

```
Container Startup
    ↓
entrypoint.sh
    ├─→ Substitute env vars in configs
    ├─→ Create runtime config in /tmp/config/
    └─→ Execute service binary
        ↓
    Service Running
        ├─→ Read from /app/config/ (mounted)
        ├─→ Write to /app/data/ (volume)
        ├─→ Use env vars for secrets
        └─→ Serve on configured ports
```

---

## 🔐 Security Model

### Secrets Management

```
Development Machine:
  ✗ .env (should not exist or use test keys)
  ✓ ../.env.example (safe to commit, in project root)

Production Server:
  ✓ .env (created from template)
  ✓ chmod 600 .env (only owner can read)
  ✓ Not in git, not in Docker image
  ✓ Injected at runtime only
```

### Access Control

```
User: subnet (UID 1000, non-root)
  ├─ /app/bin/*         → Read + Execute
  ├─ /app/config/*      → Read only (mounted :ro)
  ├─ /app/data/*        → Read + Write
  └─ /tmp/config/*      → Read + Write (runtime)
```

### Network Isolation

```
Docker Network: subnet-network (172.28.0.0/16)
  ├─ Internal: Services communicate by hostname
  └─ External: Only exposed ports accessible
     ├─ 8101 → Registry HTTP
     ├─ 8093 → Matcher gRPC
     ├─ 9090-9092 → Validator gRPC
     └─ 7400-7402 → Raft consensus
```

---

## 🚀 Deployment Scenarios

### Scenario 1: Local Testing

```bash
cd deployment
cp ../.env.example .env
nano .env  # Configure with test keys
./scripts/deploy.sh
```

### Scenario 2: Single Production Server

```bash
# Transfer distribution package
scp pinai-subnet-dist-*.tar.gz user@server:/opt/

# On server
cd /opt
tar xzf pinai-subnet-dist-*.tar.gz
cd pinai-subnet-dist-*/
./install.sh
cp ../.env.example .env
nano .env  # Configure with production keys
./scripts/deploy.sh
```

### Scenario 3: Multiple Servers (Same Config)

```bash
# Build once
./scripts/build-images.sh
./scripts/export-images.sh

# Deploy to multiple servers
for server in server1 server2 server3; do
  scp pinai-subnet-dist-*.tar.gz user@$server:/opt/
  ssh user@$server "cd /opt && tar xzf pinai-subnet-dist-*.tar.gz && cd pinai-subnet-dist-*/ && ./install.sh"
done

# Configure each separately (different keys per server)
```

---

## 🔄 Update Strategies

### Update Configuration Only

```bash
# Edit config
nano config/matcher.yaml

# Restart affected service
docker compose -f docker/docker-compose.yml restart matcher
```

**Impact:** Service restart (~5 seconds)
**Downtime:** Minimal
**Data:** Preserved

### Update Environment Variables

```bash
# Edit .env
nano .env

# Restart services
docker compose -f docker/docker-compose.yml restart
```

**Impact:** All services restart (~30 seconds)
**Downtime:** Brief
**Data:** Preserved

### Update Binaries (Minor Update)

```bash
# Receive new distribution package
./install.sh

# Recreate containers with new image
docker compose -f docker/docker-compose.yml up -d --force-recreate
```

**Impact:** Rolling restart
**Downtime:** Minimal with load balancer
**Data:** Preserved (volumes)

### Update Binaries (Major Update)

```bash
# Stop services
docker compose -f docker/docker-compose.yml down

# Backup data
cp -r data data.backup

# Install new version
./install.sh

# Update config if needed
# Check config migration guide

# Start with new version
./scripts/deploy.sh
```

**Impact:** Full restart
**Downtime:** Planned maintenance window
**Data:** Preserved + backed up

---

## 📊 Resource Management

### Per-Service Resources

```yaml
# Can be added to docker-compose.yml
services:
  validator-1:
    deploy:
      resources:
        limits:
          cpus: '1.0'
          memory: 2G
        reservations:
          cpus: '0.5'
          memory: 1G
```

### Volume Management

```bash
# Check volume usage
docker system df -v

# Cleanup old data (careful!)
docker compose down -v  # Removes volumes

# Backup volumes
tar czf data-backup-$(date +%Y%m%d).tar.gz data/
```

---

## 🔍 Monitoring & Debugging

### Health Checks

```bash
# Built into docker-compose.yml
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost:8101/agents"]
  interval: 30s
  timeout: 10s
  retries: 3
```

### Log Aggregation

```bash
# All logs
docker compose logs -f

# Specific service
docker compose logs -f validator-1

# With timestamps
docker compose logs -f --timestamps

# Last N lines
docker compose logs --tail=100
```

### Debugging

```bash
# Execute command in container
docker exec -it pinai-validator-1 bash

# Check environment
docker exec pinai-validator-1 env

# Check mounted configs
docker exec pinai-validator-1 ls -la /app/config/

# Check process
docker exec pinai-validator-1 ps aux
```

---

## 🎯 Design Decisions

### Why Binary-Only Images?

**Pros:**
- ✅ Faster deployment (no compilation)
- ✅ Smaller images (~80MB vs ~300MB)
- ✅ Reduced attack surface (no build tools)
- ✅ Consistent binaries across deployments
- ✅ Can protect intellectual property

**Trade-offs:**
- ⚠️ Need compilation step before Docker build
- ⚠️ Platform-specific binaries (use Docker build for portability)

### Why External Configuration?

**Pros:**
- ✅ Update config without rebuilding image
- ✅ Different configs for different environments
- ✅ Easier to version control
- ✅ Secrets never in image

**Trade-offs:**
- ⚠️ Need to manage config files separately
- ⚠️ Must ensure config compatibility with binary version

### Why Volume Persistence?

**Pros:**
- ✅ Data survives container restarts
- ✅ Easy backups (just copy data/ directory)
- ✅ Can inspect data directly on host
- ✅ Better performance than named volumes

**Trade-offs:**
- ⚠️ Tied to host filesystem
- ⚠️ Need backup strategy

---

## 📚 References

- [Quick Start Guide](QUICKSTART.md)
- [Full Deployment README](README.md)
- [Docker Architecture Comparison](../DOCKER_ARCHITECTURE.md)
- [Security Best Practices](../docs/security.md)

---

**Summary:** This architecture provides a production-ready deployment solution with clear separation of concerns, security best practices, and operational flexibility. 🚀


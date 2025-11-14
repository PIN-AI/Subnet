# PinAI Subnet - Production Deployment

**Pre-compiled Binary Distribution for Production Environments**

This directory contains everything needed to deploy PinAI Subnet in production using pre-compiled binaries.

## 📁 Directory Structure

```
deployment/
├── README.md              # This file
├── .env.template          # Environment configuration template
├── docker/
│   ├── Dockerfile         # Binary-only Dockerfile (no source code)
│   ├── docker-compose.yml # Production docker-compose
│   └── entrypoint.sh      # Container entrypoint
├── config/
│   ├── matcher.yaml       # Matcher configuration
│   ├── auth_config.yaml   # Authentication configuration  
│   └── policy_config.yaml # Policy configuration
├── scripts/
│   ├── build-images.sh    # Build Docker images from binaries
│   ├── deploy.sh          # Deploy to production
│   └── export-images.sh   # Export images for distribution
└── data/                  # Runtime data (mounted as volumes)
    ├── registry/
    ├── matcher/
    ├── validator-1/
    ├── validator-2/
    └── validator-3/
```

## 🚀 Quick Start

### Prerequisites

- Docker and Docker Compose installed
- Pre-compiled binaries in `../bin/` directory

### Step 1: Prepare Binaries

```bash
# From project root
cd /Users/ty/pinai/protocol/Subnet
make build

# Verify binaries exist
ls -lh bin/
```

### Step 2: Configure Environment

```bash
cd deployment
cp .env.template .env
nano .env  # Edit configuration
```

### Step 3: Build Docker Images

```bash
./scripts/build-images.sh
```

### Step 4: Deploy

```bash
./scripts/deploy.sh
```

## 🔒 Security Notes

- `.env` file contains sensitive keys - **never commit to git**
- Set proper permissions: `chmod 600 .env`
- Config files are mounted read-only into containers
- Data directories are persistent volumes

## 📦 Distribution

### Export Images for Distribution

```bash
./scripts/export-images.sh
# Creates: pinai-subnet-images.tar.gz
```

### Deploy on Target Server

```bash
# Transfer files
scp pinai-subnet-images.tar.gz .env ubuntu@server:/opt/pinai/
scp -r config scripts ubuntu@server:/opt/pinai/

# On target server
cd /opt/pinai
docker load < pinai-subnet-images.tar.gz
./scripts/deploy.sh
```

## 🛠️ Management

```bash
# View logs
docker compose -f docker/docker-compose.yml logs -f

# Check status
docker compose -f docker/docker-compose.yml ps

# Stop services
docker compose -f docker/docker-compose.yml down

# Restart
docker compose -f docker/docker-compose.yml restart
```

## 📊 Monitoring

- Registry:   http://localhost:8101/agents
- Matcher:    http://localhost:8092/health
- Validators: gRPC on 9090, 9091, 9092

## 🔧 Troubleshooting

Common deployment issues:

### Services Not Starting
- Check Docker logs: `docker compose -f docker/docker-compose.yml logs`
- Verify binaries are present in `../bin/` directory
- Ensure `.env` file is properly configured

### Port Conflicts
- Check if ports are already in use: `lsof -i :8090,8091,8092,9090,9091,9092`
- Modify port mappings in `docker/docker-compose.yml` if needed

### Configuration Issues
- Verify environment variables in `.env` file
- Check configuration files in `config/` directory
- Ensure RootLayer endpoints are accessible

For detailed troubleshooting guides, see [Subnet Deployment Guide](../docs/subnet_deployment_guide.md#troubleshooting).

#!/bin/bash
# 启动 CometBFT 共识模式的 subnet

# 停止旧服务
pkill -9 -f "bin/validator" 2>/dev/null
pkill -9 -f "bin/matcher" 2>/dev/null
pkill -9 -f "bin/registry" 2>/dev/null
pkill -9 -f "bin/simple-agent" 2>/dev/null

# 清理数据
rm -rf subnet-logs test-cometbft

echo "启动 CometBFT 共识模式..."

# 设置环境变量
export SUBNET_ID="0x0000000000000000000000000000000000000000000000000000000000000009"
export ROOTLAYER_GRPC="3.17.208.238:9001"
export ROOTLAYER_HTTP="http://3.17.208.238:8081"
export CONSENSUS_TYPE="cometbft"
export TEST_MODE=true
export START_AGENT=true

# 启动
./scripts/start-subnet.sh

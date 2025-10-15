#!/bin/bash
# E2E Test Runner Script
# 方便快速运行端到端测试

set -e

echo "🚀 PinAI Subnet E2E 测试启动器"
echo "================================"
echo ""

# 清理旧进程
echo "📦 清理旧进程..."
pkill -f "bin/matcher" 2>/dev/null || true
pkill -f "bin/validator" 2>/dev/null || true
pkill -f "test-agent" 2>/dev/null || true
sleep 1

# 检查 NATS
echo "🔍 检查 NATS..."
if ! lsof -Pi :4222 -sTCP:LISTEN -t >/dev/null 2>&1; then
    echo "⚠️  NATS 未运行，请先启动: nats-server -D"
    exit 1
fi
echo "✓ NATS 运行正常"

# 检查二进制文件
echo "🔨 检查二进制文件..."
if [ ! -f "./bin/matcher" ]; then
    echo "📦 构建项目..."
    make build
fi
echo "✓ 二进制文件就绪"

# 设置环境变量
echo ""
echo "⚙️  配置测试环境..."

# 合约地址
export PIN_BASE_SEPOLIA_INTENT_MANAGER="0xD04d23775D3B8e028e6104E31eb0F6c07206EB46"
export PIN_BASE_SEPOLIA_SUBNET_FACTORY="0x493c5B1c7Ee9eDe75bf2e57e5250E695F929A796"
export PIN_BASE_SEPOLIA_STAKING_MANAGER="0xAc11AE66c7831A70Bea940b0AE16c967f940cB65"
export PIN_BASE_SEPOLIA_CHECKPOINT_MANAGER="0xe947c9C4183D583fB2E500aD05B105Fa01abE57e"

# RootLayer 配置
export SUBNET_ID="0x0000000000000000000000000000000000000000000000000000000000000002"
export ROOTLAYER_GRPC="3.17.208.238:9001"
export ROOTLAYER_HTTP="http://3.17.208.238:8081"

# 区块链配置
export ENABLE_CHAIN_SUBMIT="${ENABLE_CHAIN_SUBMIT:-true}"
export CHAIN_RPC_URL="https://sepolia.base.org"
export CHAIN_NETWORK="base_sepolia"

# Check if TEST_PRIVATE_KEY is set
if [ -z "$TEST_PRIVATE_KEY" ]; then
    echo "❌ ERROR: TEST_PRIVATE_KEY environment variable not set"
    echo ""
    echo "Please set your test private key:"
    echo "  export TEST_PRIVATE_KEY=\"your_test_private_key_here\""
    echo ""
    echo "WARNING: Use a test-only key with no real funds!"
    exit 1
fi

echo "✓ 环境配置完成"
echo ""
echo "📋 测试配置:"
echo "   Subnet ID: $SUBNET_ID"
echo "   RootLayer: $ROOTLAYER_GRPC"
echo "   区块链提交: $ENABLE_CHAIN_SUBMIT"
echo ""

# 运行测试
echo "🧪 启动 E2E 测试..."
echo "================================"
echo ""

./scripts/e2e-test.sh "$@"

EXIT_CODE=$?

echo ""
echo "================================"
if [ $EXIT_CODE -eq 0 ]; then
    echo "✅ 测试完成!"
else
    echo "❌ 测试失败 (退出码: $EXIT_CODE)"
    echo ""
    echo "📋 查看日志:"
    echo "   tail -f e2e-test-logs/matcher.log"
    echo "   tail -f e2e-test-logs/validator-1.log"
    echo "   tail -f e2e-test-logs/test-agent.log"
fi

exit $EXIT_CODE

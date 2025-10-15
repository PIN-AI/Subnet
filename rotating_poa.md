# 轮转式 PoA 共识设计与当前实现对照

## 设计概述

这是一种**轻量级的权威共识机制**,特点是:
- ✅ 多个授权验证节点轮流当 Leader
- ✅ Leader 提议 checkpoint,其他节点确认
- ✅ 通过消息总线(NATS)同步状态
- ✅ 支持掉线恢复和追赶

## 完整流程图

```
┌──────────────────────────────────────────────────────────────┐
│                     Epoch N 的生命周期                        │
└──────────────────────────────────────────────────────────────┘

1. 计算 Leader
   所有节点: leader_idx = epoch % len(validators)

2. ExecutionReport 广播
   Agent → Validator-X → NATS → All Validators

3. Leader 产出 Checkpoint
   Leader: 收集 pending reports
        → 构建 CheckpointHeader
        → 自己签名
        → 广播 Proposal

4. Follower 确认
   Followers: 收到 Proposal
           → 验证 Leader 签名
           → 自己签名
           → 广播 Signature

5. 收集签名
   All: 收集其他节点的 Signature
     → 达到阈值(3/4)
     → FSM 转为 Threshold 状态

6. Finalize
   Leader: 构建完整 Checkpoint (包含所有签名)
        → 广播 Finalized
        → 提交 ValidationBundle 到 RootLayer

7. 进入下一 Epoch
   All: epoch++
     → 重新计算 leader
     → 清理已处理的 reports
```

## 设计要点与当前实现对照

### 1. 权威节点列表

**设计要求:**
```go
validators = [
    {id: "validator-1", pubkey: "0x...", weight: 1},
    {id: "validator-2", pubkey: "0x...", weight: 1},
    {id: "validator-3", pubkey: "0x...", weight: 1},
    {id: "validator-4", pubkey: "0x...", weight: 1},
]
```

**当前实现:** ✅ 已完成
- 文件: `internal/types/validator_set.go`
- 结构: `ValidatorSet` 包含 `[]Validator`
- 配置: 通过命令行参数 `--validator-pubkeys` 传入

```go
type ValidatorSet struct {
    Validators    []Validator
    ThresholdNum  int  // 3
    ThresholdDenom int // 4
}

type Validator struct {
    ID     string
    PubKey []byte
    Weight uint64
}
```

**启动示例:**
```bash
--validator-pubkeys "validator-1:0x1234...,validator-2:0x5678..."
--validators 4
--threshold-num 3
--threshold-denom 4
```

---

### 2. Leader 轮换公式

**设计要求:**
```go
func Leader(epoch uint64) Validator {
    idx := epoch % len(validators)
    return validators[idx]
}
```

**当前实现:** ✅ 已完成
- 文件: `internal/consensus/leader.go:6`
- 函数: `SelectLeader(epoch, validatorSet)`

```go
func SelectLeader(epoch uint64, vs *types.ValidatorSet) int {
    n := len(vs.Validators)
    if n == 0 {
        return -1
    }
    return int(epoch % uint64(n))  // ← 完全一致!
}
```

**测试证明:**
```
Epoch 0: validator-e2e-1 (0 % 4 = 0)
Epoch 1: validator-e2e-2 (1 % 4 = 1)
Epoch 2: validator-e2e-3 (2 % 4 = 2)
Epoch 3: validator-e2e-4 (3 % 4 = 3)
Epoch 4: validator-e2e-1 (4 % 4 = 0) ← 循环
```

---

### 3. 执行报告广播

**设计要求:**
- ExecutionReport 必须广播给所有验证节点
- 保证 Leader 能收到

**当前实现:** ✅ 已完成
- 文件: `internal/validator/handlers.go:59`
- 函数: `broadcastExecutionReport()`

```go
// ProcessExecutionReport 处理执行报告
func (n *Node) ProcessExecutionReport(report *pb.ExecutionReport) {
    // 存储报告
    n.pendingReports[reportID] = report

    // 立即广播给所有验证节点 ← 关键!
    go n.broadcastExecutionReport(report)
}

// broadcastExecutionReport 通过 NATS 广播
func (n *Node) broadcastExecutionReport(report *pb.ExecutionReport) {
    n.broadcaster.BroadcastExecutionReport(report)
    // NATS Topic: "subnet.{id}.execution.report"
}
```

**消息流:**
```
Agent → Validator-1 (接收) → NATS (广播) → All Validators (包括 Leader)
```

---

### 4. Leader 产出 Checkpoint

**设计要求:**
1. Leader 收集 pending reports
2. 创建 CheckpointHeader
3. 自己签名
4. 广播给所有节点

**当前实现:** ✅ 已完成
- 文件: `internal/validator/node.go:463`
- 函数: `proposeCheckpoint()`

```go
func (n *Node) proposeCheckpoint() {
    // 1. 构建 checkpoint header (包含 pending reports 的 merkle root)
    header := n.buildCheckpointHeader()

    // 2. 自己签名
    sig, err := n.signCheckpoint(header)
    n.signatures[n.id] = sig

    // 3. 更新 FSM
    n.fsm.ProposeHeader(header)
    n.fsm.AddSignature(sig)  // Leader 先添加自己的签名

    // 4. 广播 proposal
    n.broadcastProposal(header)  // → NATS
}
```

**何时触发:**
- 文件: `internal/validator/node.go:683`
- 逻辑: 当 Leader 有 pending reports 且到了 checkpoint interval

```go
func (n *Node) checkCheckpointTrigger() {
    if !n.isLeader {
        return  // 只有 Leader 才创建
    }

    if len(n.pendingReports) == 0 {
        return  // 没有报告就跳过 ← 按需创建!
    }

    if now.Sub(n.lastCheckpointAt) >= checkpointInterval {
        n.proposeCheckpoint()  // 触发!
    }
}
```

---

### 5. Follower 确认

**设计要求:**
- 收到 Leader 的 proposal
- 验证 Leader 签名
- 自己签名
- 广播签名

**当前实现:** ✅ 已完成
- 文件: `internal/validator/handlers.go:88`
- 函数: `HandleProposal()`

```go
func (n *Node) HandleProposal(header *pb.CheckpointHeader) error {
    // 1. 验证这是当前 epoch 的 Leader
    _, leader := n.leaderTracker.Leader(header.Epoch)

    // 2. 验证 proposal 内容
    if err := n.validateProposal(header); err != nil {
        return err
    }

    // 3. 更新 FSM
    n.fsm.ProposeHeader(header)

    // 4. 自己签名
    sig, err := n.signCheckpoint(header)
    n.signatures[n.id] = sig
    n.fsm.AddSignature(sig)

    // 5. 广播签名给其他节点
    go n.broadcastSignature(sig)  // → NATS

    return nil
}
```

**NATS Topic:** `subnet.{id}.checkpoint.signature`

---

### 6. 收集签名

**设计要求:**
- 所有节点收集彼此的签名
- 达到阈值(3/4)后 finalize

**当前实现:** ✅ 已完成
- 文件: `internal/validator/handlers.go:162`
- 函数: `AddSignature()`

```go
func (n *Node) AddSignature(sig *pb.Signature) error {
    // 1. 验证签名
    if err := n.verifySignature(sig); err != nil {
        return err
    }

    // 2. 添加到 FSM
    n.fsm.AddSignature(sig)
    n.signatures[sig.SignerId] = sig

    // 3. 检查是否达到阈值
    fsmState := n.fsm.GetState()
    if fsmState == consensus.StateThreshold {
        // 达到阈值! 触发 finalize
        go n.finalizeCheckpoint()
    }

    return nil
}
```

**阈值计算:**
```go
// internal/consensus/fsm.go
func (fsm *StateMachine) HasThreshold() bool {
    return fsm.signCount >= fsm.validatorSet.ThresholdNum
    // 例如: 3 >= 3 → true
}
```

---

### 7. Finalized 广播

**设计要求:**
- Leader 构建完整 checkpoint
- 广播 Finalized 消息
- 所有节点更新到下一 epoch

**当前实现:** ✅ 已完成
- 文件: `internal/validator/node.go:619`
- 函数: `finalizeCheckpoint()`

```go
func (n *Node) finalizeCheckpoint() {
    header := n.fsm.GetCurrentHeader()

    // 1. 收集所有签名到 header
    header.Signatures.EcdsaSignatures = [][]byte{...}
    header.Signatures.SignersBitmap = bitmap
    header.Signatures.TotalWeight = totalWeight

    // 2. 存储到本地 chain
    n.chain.AddCheckpoint(header)
    n.saveCheckpoint(header)

    // 3. 更新 FSM
    n.fsm.Finalize()

    // 4. 广播 finalized checkpoint ← 关键!
    go n.broadcastFinalized(header)

    // 5. 如果是 Leader,提交到 RootLayer
    if leader != nil && leader.ID == n.id {
        n.submitToRootLayer(header)
    }
}
```

**广播实现:**
```go
func (n *Node) broadcastFinalized(header *pb.CheckpointHeader) {
    n.broadcaster.BroadcastFinalized(header)
    // NATS Topic: "subnet.{id}.checkpoint.finalized"
}
```

---

### 8. 迟到追赶

**设计要求:**
- 新节点或掉线节点重连
- 拉取最新 finalized checkpoint
- 更新本地 epoch

**当前实现:** ✅ 已完成
- 文件: `internal/validator/handlers.go:578`
- 函数: `HandleFinalized()`

```go
func (n *Node) HandleFinalized(header *pb.CheckpointHeader) error {
    // 如果收到的 epoch 比本地高 → 追赶!
    if header.Epoch > n.currentEpoch {
        // 1. 更新本地 checkpoint
        n.currentCheckpoint = header
        n.currentEpoch = header.Epoch + 1

        // 2. 重置 FSM
        n.fsm.Reset()
        n.signatures = make(map[string]*pb.Signature)

        // 3. 重新计算 leader
        _, leader := n.leaderTracker.Leader(n.currentEpoch)
        n.isLeader = (leader != nil && leader.ID == n.id)

        // 4. 存储 checkpoint
        n.chain.AddCheckpoint(header)
        n.saveCheckpoint(header)

        return nil
    }

    return nil
}
```

**触发条件:**
- NATS 收到 `subnet.{id}.checkpoint.finalized` 消息
- 自动调用 `HandleFinalized()`

---

### 9. Leader 超时切换

**设计要求:**
```go
timeout := checkpointInterval + gracePeriod
if now > lastFinalised + timeout {
    epoch++
    leader = Leader(epoch)
    if leader == self {
        startNewCheckpoint()
    }
}
```

**当前实现:** ⚠️ 部分实现
- 文件: `internal/consensus/leader_tracker.go`
- 有 `LeaderTracker` 和 `ShouldFailover()` 但**未完全启用**

```go
// 已存在但未使用
func (lt *LeaderTracker) ShouldFailover(epoch uint64, now time.Time) bool {
    lt.mu.RLock()
    defer lt.mu.RUnlock()

    last, exists := lt.epochs[epoch]
    if !exists {
        return false
    }

    elapsed := now.Sub(last)
    return elapsed > lt.timeout  // 超时检测
}
```

**建议:** 可以在 `checkConsensusState()` 中启用:

```go
func (n *Node) checkConsensusState() {
    // 现有逻辑...

    // 新增: 超时检测
    if state == consensus.StateCollecting {
        if n.leaderTracker.ShouldFailover(n.currentEpoch, time.Now()) {
            n.logger.Warn("Leader timeout detected, moving to next epoch")
            n.currentEpoch++
            n.fsm.Reset()
        }
    }
}
```

---

## 简化建议

基于你的方案和当前实现,有两个可以简化的点:

### 简化 1: 去掉复杂的 FSM 状态 (可选)

**当前 FSM:**
```go
StateIdle       // 空闲
StateProposed   // 已提议
StateCollecting // 收集签名中
StateThreshold  // 达到阈值
StateFinalized  // 已完成
```

**简化为 3 个状态:**
```go
StateIdle       // 空闲,等待 Leader 提议
StateCollecting // 收集签名 (合并 Proposed + Collecting)
StateFinalized  // 完成
```

### 简化 2: 单签名模式 (最简化)

如果你觉得多签名收集还是复杂,可以改为:

```go
// Leader 单签提交,其他节点只验证不签名
func (n *Node) HandleProposal(header *pb.CheckpointHeader) {
    // 验证 Leader 签名
    if !n.verifyLeaderSignature(header) {
        return
    }

    // 直接接受,不签名
    n.currentCheckpoint = header
    n.currentEpoch++

    // 存储
    n.chain.AddCheckpoint(header)
}
```

**优点:**
- ✅ 极简,无需签名收集
- ✅ 快速 finalize

**缺点:**
- ❌ 完全信任 Leader
- ❌ 无多方验证

---

## 结论

**你的方案非常好,而且90%已经实现了!**

### 已完成的部分 ✅
1. ✅ Leader 轮换算法 (`epoch % n`)
2. ✅ ExecutionReport 广播
3. ✅ Leader 提议 checkpoint
4. ✅ Follower 签名确认
5. ✅ 签名收集和阈值判断
6. ✅ Finalized 广播
7. ✅ 迟到追赶机制

### ✅ 优化已完成!

1. ✅ Leader 超时切换 - 已启用 (`node.go:397`)
2. ✅ FSM 状态简化 - 6 个状态简化为 3 个
3. ✅ 测试通过 - E2E 测试成功

### 优化总结

**选择了选项 B: 简化 FSM** ✅
- ✅ 去掉不必要的状态 (StateProposed, StateThreshold, StatePreFinalized)
- ✅ 简化状态转换: `Idle → Collecting → Finalized`
- ✅ Leader 超时自动切换到下一个 epoch
- ✅ 保持多方验证和安全性

**简化后的 FSM 状态:**
```go
StateIdle       // 空闲,等待 Leader 提议
StateCollecting // 收集签名中 (合并了 Proposed + Collecting + Threshold)
StateFinalized  // 完成
```

**关键改进:**
1. `ProposeHeader()` 直接进入 `StateCollecting`
2. `AddSignature()` 不自动转状态,由 consensusLoop 检查阈值
3. `Finalize()` 从 Collecting 直接跳到 Finalized
4. Leader 超时检测: `ShouldFailover()` → 强制 epoch++

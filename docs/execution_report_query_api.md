# ExecutionReport Query API

查询 Agent 执行结果的 API 文档。

## 功能说明

Validator 会自动保存所有接收到的 ExecutionReport 到持久化存储（LevelDB），并提供 gRPC API 供查询。

## API 接口

### 1. GetExecutionReport - 查询单个报告

根据 `report_id` 查询特定的执行结果。

**gRPC 方法**:
```protobuf
rpc GetExecutionReport(GetExecutionReportRequest) returns (ExecutionReport);
```

**请求参数**:
```protobuf
message GetExecutionReportRequest {
  string report_id = 1;  // Required: report ID
}
```

**返回数据**:
```protobuf
message ExecutionReport {
  string intent_id = 1;
  string assignment_id = 2;
  string agent_id = 3;
  bytes result_data = 4;      // Agent 执行结果（JSON 格式）
  int64 timestamp = 5;
  // ... 其他字段
}
```

**使用示例**:
```bash
# 使用 grpcurl（需安装 grpcurl）
grpcurl -plaintext \
  -d '{"report_id": "intent-id:assignment-id:agent-id:timestamp"}' \
  localhost:9090 \
  subnet.v1.ValidatorService/GetExecutionReport
```

### 2. ListExecutionReports - 列出所有报告

列出存储中的所有执行报告，支持分页。

**gRPC 方法**:
```protobuf
rpc ListExecutionReports(ListExecutionReportsRequest) returns (ListExecutionReportsResponse);
```

**请求参数**:
```protobuf
message ListExecutionReportsRequest {
  string intent_id = 1;  // Optional: 按 intent_id 过滤（暂未实现）
  uint32 limit = 2;      // Optional: 最大返回数量（默认 100）
}
```

**返回数据**:
```protobuf
message ListExecutionReportsResponse {
  repeated ExecutionReportEntry reports = 1;
  uint32 total = 2;  // 返回的报告总数
}

message ExecutionReportEntry {
  string report_id = 1;
  ExecutionReport report = 2;
}
```

**使用示例**:
```bash
# 列出最近 10 条报告
grpcurl -plaintext \
  -d '{"limit": 10}' \
  localhost:9090 \
  subnet.v1.ValidatorService/ListExecutionReports
```

## Report ID 格式

Report ID 由以下部分组成：
```
{intent_id}:{assignment_id}:{agent_id}:{timestamp}
```

示例：
```
test-intent-20251114-132632:test-assignment-001:test-agent-query-api:1763097992
```

## Result Data 格式

`result_data` 字段包含 Agent 执行的实际结果，通常是 JSON 格式：

```json
{
  "status": "success",
  "message": "Task completed successfully",
  "data": {
    "result": 42,
    "computeTime": "120ms"
  }
}
```

## Go 客户端示例

```go
package main

import (
    "context"
    "fmt"
    "log"

    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"

    pb "subnet/proto/subnet"
)

func main() {
    // 连接 Validator
    conn, err := grpc.Dial("localhost:9090",
        grpc.WithTransportCredentials(insecure.NewCredentials()))
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    client := pb.NewValidatorServiceClient(conn)
    ctx := context.Background()

    // 1. 查询单个报告
    getResp, err := client.GetExecutionReport(ctx, &pb.GetExecutionReportRequest{
        ReportId: "your-report-id",
    })
    if err != nil {
        log.Printf("Failed to get report: %v", err)
    } else {
        fmt.Printf("Intent: %s\n", getResp.IntentId)
        fmt.Printf("Agent: %s\n", getResp.AgentId)
        fmt.Printf("Result: %s\n", string(getResp.ResultData))
    }

    // 2. 列出所有报告
    listResp, err := client.ListExecutionReports(ctx, &pb.ListExecutionReportsRequest{
        Limit: 10,
    })
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Found %d reports:\n", listResp.Total)
    for _, entry := range listResp.Reports {
        fmt.Printf("- %s: %s\n", entry.ReportId, string(entry.Report.ResultData))
    }
}
```

## 存储位置

ExecutionReport 存储在 Validator 的 LevelDB 数据库中：
- 默认路径：`./data/`
- 可通过 `-storage` 参数配置

数据持久化，重启 Validator 后仍可查询历史记录。

## 注意事项

1. **Report ID 必须准确** - GetExecutionReport 需要完整的 report_id
2. **intentID 过滤暂未实现** - ListExecutionReports 的 intent_id 参数当前返回所有报告
3. **性能考虑** - 大量报告时建议使用 limit 参数分页
4. **存储清理** - 当前没有自动清理机制，需要手动管理存储空间

## 测试工具

使用内置测试工具快速验证：

```bash
cd /Users/ty/pinai/protocol/Subnet
./bin/test-execution-report-query
```

该工具会：
1. 提交一个测试 ExecutionReport
2. 查询刚提交的报告
3. 列出所有存储的报告

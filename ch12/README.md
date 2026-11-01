# 第十二章：Agent 评测与自动化测试（LLM Eval）

前面的章节让 Agent 能调用工具、管理上下文、持久化会话，也能记录执行轨迹。但「看起来能用」并不是质量保证：修改 prompt、模型或工具后，可能悄悄让原本正常的能力退化。

本章给 Agent 加上一条可重复执行的质量门禁：把任务写成数据集，用确定性的 Mock LLM 执行 Agent Loop，并生成可供 CI 比较的评测报告。

## 本章目标

- 用小接口隔离 LLM SDK，使 Agent 调度逻辑可在不联网、不花 token 的情况下测试。
- 用 `MockClient` 精确编排「模型回复 → 工具调用 → 模型最终回答」。
- 用版本化数据集定义输入、期望输出、期望工具序列与最大 loop 深度。
- 生成通过率、失败原因、耗时与 token 用量的结构化报告。

## 结构

```text
ch12/
├── agent/
│   ├── types.go       # 与厂商无关的消息、Completion、Client/Tool 接口
│   ├── agent.go       # 可测试的 Agent Loop，含最大循环保护
│   ├── mock.go        # MockClient 与 MockTool
│   └── agent_test.go
├── eval/
│   ├── dataset.go     # JSON 数据集加载
│   ├── runner.go      # 并发执行与断言评分
│   ├── report.go      # JSON / Markdown 报告
│   └── runner_test.go
├── dataset/demo.json  # 无需 API Key 的示例集
└── main/main.go       # `go run` 入口
```

## 为什么要 Mock LLM

真实 LLM 调用适合集成测试或线上评测，但不适合单元测试：输出有随机性、速度慢、需要密钥并且会产生费用。Agent 最值得先保护的部分是调度逻辑；也就是模型声明工具调用时，Agent 是否正确执行工具、把结果回填给下一轮模型，以及是否在异常循环时停止。

```text
MockClient (第 1 个 Completion: 调 read)
                 ↓
Agent 执行 MockTool
                 ↓
MockClient (第 2 个 Completion: 最终答案)
                 ↓
Runner 对答案、工具序列、loop 深度评分
```

`MockClient` 会按队列顺序返回 Completion，并保存每一次请求消息。因此测试不仅能断言最终答案，还能验证工具结果确实被回填到了下一轮上下文。

## 快速运行

在仓库根目录执行：

```bash
# 运行本章单元测试
go test -v ./ch12/...

# 离线运行示例评测；不需要 config.json 或 API Key
go run ./ch12/main

# 同时写出机器可读与可读报告
go run ./ch12/main \
  -output /tmp/ch12-report.json \
  -markdown /tmp/ch12-report.md
```

示例成功时会输出：

```text
# Eval report: offline-demo

- Total: 2
- Passed: 2
- Failed: 0
```

只要任一用例失败，命令会以非零状态退出，可直接作为 CI 检查。

## 数据集格式

评测数据保存在 JSON 文件中。每个 case 支持以下字段：

| 字段 | 含义 |
|---|---|
| `id` | 稳定、唯一的用例标识 |
| `query` | 发送给 Agent 的用户问题 |
| `expected_contains` | 最终回复必须包含的词或短语（不区分大小写） |
| `expected_tools` | 必须按顺序调用的工具名 |
| `max_loop_depth` | 允许的最大 LLM 调用轮次 |
| `mock_responses` | Mock LLM 依次返回的 Completion 队列 |

最小直接回答用例：

```json
{
  "id": "greeting",
  "query": "Say hello",
  "expected_contains": ["hello"],
  "max_loop_depth": 1,
  "mock_responses": [
    {"content": "Hello!"}
  ]
}
```

工具调用用例：

```json
{
  "id": "read-doc",
  "query": "Read README",
  "expected_tools": ["read"],
  "expected_contains": ["README"],
  "max_loop_depth": 2,
  "mock_responses": [
    {"tool_calls": [{"id": "call-1", "name": "read", "arguments": "README.md"}]},
    {"content": "The README was read."}
  ]
}
```

示例入口使用 `MockTool`，所以它不会读取或修改真实文件。接入真实 Agent 时，只需把 `eval.Run` 的 executor 换成实际模型客户端和真实工具；断言、报告与 CI 逻辑无需改变。

## 评测指标与边界

本章默认采用确定性断言，适合保护基础行为：

- 回复是否包含关键事实；
- 工具是否按预期被调用；
- Agent 有没有无效循环；
- 每次任务的耗时与累计 token。

这不是完整的「答案质量」判定。对开放式任务，可在下一层加入人工标注、规则评分、代码测试，或 LLM-as-a-Judge；后者应记录模型版本、judge prompt 和原始判定，避免把不稳定评分误当成绝对事实。

## 与第十一章的关系

第 11 章回答线上请求发生了什么；第 12 章回答一次改动是否让 Agent 变好或变坏。可将第 11 章的慢请求、工具失败或异常 loop 轨迹匿名化后沉淀为本章数据集，再用评测防止同类问题回归。

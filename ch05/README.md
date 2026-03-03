# 第五章：上下文工程（Context Engineering）

欢迎来到第五章！在第四章的基础上，本章介绍 Agent 开发中最重要的概念之一：**上下文工程**（Context Engineering）。

随着 Agent 交互轮次增加，上下文长度会持续增长，最终触及模型上下文窗口限制，导致性能下降或请求失败。本章实现一套完整的上下文管理机制，让 Agent 能在有限窗口内稳定运行。

---

## 🎯 你将学到什么

1. **上下文管理机制**：理解为什么需要上下文工程，以及常见压缩手段。
2. **截断策略（Truncate）**：如何安全删除旧消息并保留对话连续性。
3. **卸载策略（Offload）**：将长内容写入外部存储，在上下文中保留预览与恢复入口。
4. **摘要策略（Summarize）**：使用 LLM 对历史消息进行增量压缩。
5. **Policy 模式设计**：如何设计可扩展的策略接口，支持多策略组合执行。
6. **策略协同与调参**：如何根据阈值、批大小、保留条数进行工程化调优。

---

## 🛠 准备工作

本章启动时会读取新的配置文件（见 `ch05/tui/main.go`）：

1. `config.json`：应用模型配置（前台模型 + 后台摘要模型）
2. `mcp-server.json`：MCP 服务配置（可选，读取失败仅打印日志）

可按下面方式准备：

```bash
cp config.example.json config.json
```

`config.json` 示例：

```json
{
  "llm_providers": {
    "front_model": {
      "base_url": "https://api.openai.com/v1",
      "model": "gpt-5.2",
      "api_key": "sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
      "context_window": 200000
    },
    "back_model": {
      "base_url": "https://api.openai.com/v1",
      "model": "gpt-4o-mini",
      "api_key": "sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
      "context_window": 128000
    }
  }
}
```

`mcp-server.json` 示例：

```json
{
  "filesystem": {
    "command": "npx",
    "args": [
      "-y",
      "@modelcontextprotocol/server-filesystem",
      "${workspaceFolder}"
    ]
  }
}
```

---

## 📖 核心原理解析

### 1. 为什么需要上下文工程？

大语言模型的**上下文窗口**（Context Window）是有限的，例如：
- GPT-4o-mini: 128K tokens
- Claude 3.5 Sonnet: 200K tokens

Agent 每轮对话都会消耗 tokens，包括：
- 用户输入
- 模型输出
- 工具调用与工具结果

如果不做管理，多轮对话后会出现：
1. **超出窗口限制**：请求被拒绝
2. **成本增加**：长上下文导致更高 API 费用
3. **性能下降**：模型在长上下文中注意力分散，响应质量下降

---

### 2. 三种上下文管理策略

#### 2.1 截断策略（Truncate）

**原理**：删除较早历史消息，保留最近对话。

**实现要点**：
- 触发条件：`contextUsage > UsageThreshold`
- 保留最近 `KeepRecentMessages` 条消息作为“不可裁剪区”
- 在可裁剪区中寻找最后一条 user 消息作为截断边界，减少语义断裂
- 避免极端情况下把历史全部删空

**适用场景**：对远期历史依赖较弱、优先保障实时交互的任务。

相关代码：`ch05/context/policy_truncate.go`

---

#### 2.2 卸载策略（Offload）

**原理**：将长消息正文存到外部存储，在上下文中保留短预览与恢复提示。

**实现要点**：
- 触发条件：`contextUsage > UsageThreshold`
- 只处理“非最近 N 条消息”中的 `tool` 消息
- 只处理长度超过 `PreviewCharLimit` 的长内容
- 通过 `Storage.Store` 存储原文
- 用“预览 + `load_storage(key="...")` 提示”替换正文

**示例效果**：
```text
原始消息（2000 tokens）：
"这是一个非常长的工具输出..."

卸载后消息（约200 tokens）：
"这是一个非常长的工具输出...（更多内容已卸载，如需查看全文请使用 load_storage(key=\"/offload/...\") 工具）"
```

**适用场景**：有大量长工具输出，但允许按需回读全文的任务。

相关代码：`ch05/context/policy_offload.go`、`ch05/tool/load_storage.go`

---

#### 2.3 摘要策略（Summarize）

**原理**：把多条历史消息压缩成一条摘要消息，保留关键信息。

**实现要点**：
- 触发条件：`contextUsage > UsageThreshold`
- 仅处理“非最近 N 条消息”
- 分批摘要：每批最多 `SummaryBatchSize` 条，且受 `Summarizer.GetSummaryInputTokenLimit()` 限制
- 增量摘要：`runningSummary + batchMessages -> newSummary`
- 最终用一条摘要消息替换被压缩历史

**适用场景**：对话较长但只需保留核心事实、决策与结果。

相关代码：`ch05/context/policy_summary.go`、`ch05/context/summary.go`

---

### 3. Policy 模式设计

本章通过 `Policy` 接口实现可扩展上下文管理：

```go
type Policy interface {
    Name() string
    ShouldApply(ctx context.Context, engine *Engine) bool
    Apply(ctx context.Context, engine *Engine) (PolicyResult, error)
}
```

**优势**：
- 可扩展：新增策略无需改 `Engine` 核心流程
- 可组合：多个策略按注册顺序依次执行
- 可控制：每个策略通过 `ShouldApply` 决定是否触发

**执行流程**：
1. 每轮新增消息后，`Engine.CommitTurn` 调用内部 `applyPolicies`
2. 按顺序检查每个 `Policy.ShouldApply`
3. 返回 `true` 则执行 `Policy.Apply`
4. `Apply` 返回新的消息列表和 token 计数（`PolicyResult`）

相关代码：`ch05/context/policy.go`、`ch05/context/engine.go`

---

### 4. Engine 与 Summarizer 协作

当前实现中，摘要能力从策略中解耦为 `Summarizer` 抽象：

```go
type Summarizer interface {
    GetSummaryInputTokenLimit() int
    Summarize(ctx context.Context, runningSummary string, messages []openai.ChatCompletionMessageParamUnion) (string, error)
}
```

默认实现是 `LLMSummarizer`，使用 `shared.ModelConfig` 创建摘要模型客户端。

**好处**：
1. 摘要策略只关心“何时摘要、摘要哪些消息”
2. 摘要实现可替换（不同模型、本地模型、规则摘要器）
3. 便于单测与演进

相关代码：`ch05/context/summary.go`、`ch05/context/policy_summary.go`

---

### 5. Token 计数

为了准确判断是否触发策略，需要实时统计上下文 token。

**实现方式**：
- 使用 `tiktoken-go`（`cl100k_base`）
- 对消息正文做 token 计数
- 每轮提交和策略执行后重算上下文 token

**注意**：属于近似估算，和实际 API 计费可能有细微差异。

相关代码：`ch05/context/share.go`

---

## 💻 代码结构速览

### Context 包
- `ch05/context/engine.go`：上下文引擎核心（`Engine`）
- `ch05/context/policy.go`：`Policy` 接口与 `PolicyResult`
- `ch05/context/policy_truncate.go`：截断策略
- `ch05/context/policy_offload.go`：卸载策略
- `ch05/context/policy_summary.go`：摘要策略
- `ch05/context/summary.go`：`Summarizer` 接口与 `LLMSummarizer`
- `ch05/context/share.go`：token 计数与角色识别工具

### 其他核心模块
- `ch05/storage/storage.go`：存储接口（卸载策略依赖）
- `ch05/tool/load_storage.go`：读取卸载内容的工具
- `ch05/agent.go`：集成上下文引擎的 Agent
- `ch05/tui/main.go`：策略装配与启动入口

---

## 🚀 动手运行

进入项目根目录，执行：

```bash
go run ./ch05/tui
```

在 TUI 中可尝试：

**1. 测试截断策略**
```text
请连续输出数字 1 到 100，每个数字单独输出
```

**2. 测试卸载策略**
```text
请读取 README.md 并总结每一章节的内容
```

**3. 测试摘要策略**
```text
我们来进行多轮对话：第一轮，请介绍你自己
第二轮，请列出当前目录的文件
第三轮，请读取 agent.go 文件
...
```

---

## ⚠️ 注意事项

1. **策略顺序有影响**：当前默认顺序是 `offload -> summarize -> truncate`
2. **摘要有额外成本**：摘要策略会引入额外 LLM 调用
3. **存储建议持久化**：生产环境建议用 Redis/数据库替代内存存储
4. **阈值需按模型调参**：不同模型上下文窗口不同，触发阈值需要按实际容量设置

---

## 📚 扩展阅读与参考资料

1. **[OpenAI Tokenizer](https://platform.openai.com/tokenizer)**
   - 在线工具，可以直观看到文本的 token 切分结果
2. **[tiktoken GitHub](https://github.com/openai/tiktoken)**
   - OpenAI 官方的 Python tokenizer 库
3. **[tiktoken-go GitHub](https://github.com/tiktoken-go/tiktoken-go)**
   - 本章使用的 Go 语言 tokenizer 移植版本
4. **[Anthropic Context Window Management](https://platform.claude.com/docs/en/build-with-claude/context-windows)**
   - Anthropic 官方关于上下文管理的最佳实践

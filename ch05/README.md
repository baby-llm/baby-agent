# 第五章：上下文工程（Context Engineering）

欢迎来到第五章！在第四章的基础上，本章介绍 Agent 开发中最重要的概念之一——**上下文工程**（Context Engineering）。

随着 Agent 交互轮次的增加，上下文长度会线性增长，最终触及模型的上下文窗口限制，导致性能下降或请求失败。本章将实现一套完整的上下文管理策略，让 Agent 能够在有限的上下文窗口内高效运行。

---

## 🎯 你将学到什么

1. **上下文管理策略**：理解为什么需要上下文工程，以及常见的策略类型。
2. **截断策略（Truncation）**：如何安全地删除旧消息，保留最近的关键对话。
3. **卸载策略（Offloading）**：将长消息内容存储到外部存储，在上下文中保留摘要和恢复提示。
4. **摘要策略（Summarization）**：使用 LLM 对历史对话进行摘要压缩，保留关键信息。
5. **策略模式设计**：如何设计可扩展的策略接口，支持灵活组合多种上下文管理策略。
6. **策略事件系统**：如何通过事件机制在 TUI 中展示策略执行状态。

---

## 🛠 准备工作

复用根目录的 `.env` 配置（见项目根目录 `README.md`）。

```env
OPENAI_API_KEY=sk-your-api-key-here
OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_MODEL=gpt-4o-mini
```

---

## 📖 核心原理解析

### 1. 为什么需要上下文工程？

大语言模型的**上下文窗口**（Context Window）是有限的，例如：
- GPT-4o-mini: 128K tokens
- Claude 3.5 Sonnet: 200K tokens

Agent 的每次对话都会消耗 tokens，包括：
- 用户输入
- 模型输出
- 工具调用和工具结果

如果不进行管理，多轮对话后会出现：
1. **超出窗口限制**：请求被拒绝
2. **成本增加**：长上下文导致更高的 API 费用
3. **性能下降**：模型在长上下文中可能"注意力分散"，影响响应质量

---

### 2. 四种上下文管理策略

#### 2.1 截断策略（Truncation）

**原理**：直接删除旧消息，只保留最近 N 条消息。

**实现要点**：
- 找到最后一条 User 消息，确保对话完整性
- 设置最少保留消息数量，避免删光所有历史
- 更新 token 计数

**适用场景**：对历史信息要求不高、只需关注最近对话的任务。

相关代码：`ch05/context/truncate.go`

---

#### 2.2 卸载策略（Offloading）

**原理**：将长消息内容存储到外部存储（如内存、Redis），在上下文中保留简短摘要和恢复提示。

**实现要点**：
- 只卸载超过一定长度的消息
- 在消息末尾添加恢复提示（如 `load_storage(key=xxx)`）
- 提供 `load_storage` 工具，让 Agent 可以按需恢复完整内容

**示例效果**：
```
原始消息（2000 tokens）：
"这是一个非常长的消息内容..."

卸载后消息（200 tokens）：
"这是一个非常长的消息内容...（更多内容已卸载，如需查看全文请使用 load_storage(key=/offload/msg_001) 工具）"
```

**适用场景**：有少量关键长消息、需要时可以按需恢复的场景。

相关代码：`ch05/context/offload.go`、`ch05/tool/load_storage.go`

---

#### 2.3 摘要策略（Summarization）

**原理**：使用 LLM 将多条历史消息摘要成一条简短消息。

**实现要点**：
- 批量处理消息（避免单次摘要请求过长）
- 支持增量摘要（新摘要基于旧摘要生成）
- 使用专门的摘要模型（更快、更便宜）

**示例效果**：
```
原始消息（10条，共5000 tokens）：
user: 列出当前目录文件
assistant: [bash 工具调用]
tool: file1.txt file2.go
assistant: 目录中有 file1.txt 和 file2.go
...

摘要后消息（1条，共300 tokens）：
用户询问如何列出目录文件，助手使用 bash 工具执行了 ls 命令，结果显示目录中有 file1.txt 和 file2.go 两个文件。
```

**适用场景**：长对话历史、需要保留关键信息但可以忽略细节的场景。

相关代码：`ch05/context/summary.go`

---

### 3. 策略模式设计

本章使用策略模式设计上下文管理：

```go
type ContextStrategy interface {
    Name() string
    ShouldApply(ctx context.Context, engine *ContextEngine) bool
    Apply(ctx context.Context, engine *ContextEngine) error
}
```

**优势**：
- 可扩展：新增策略无需修改核心逻辑
- 可组合：多个策略可以按注册顺序同时生效
- 可配置：通过 `ShouldApply` 控制触发条件

**执行流程**：
1. 每次添加消息后，`ContextEngine` 自动调用 `ApplyStrategies`
2. 按顺序检查每个策略的 `ShouldApply`
3. 如果返回 `true`，执行 `Apply` 方法
4. 策略可以修改 `messages` 和 `contextTokens`
5. 执行过程中发送策略事件到 TUI 展示

相关代码：`ch05/context/strategy.go`、`ch05/context/context.go`

---

### 4. 策略事件系统

为了在 TUI 中实时展示策略执行状态，本章实现了事件通知机制：

**事件类型**：
- `StrategyEventStart`：策略开始执行
- `StrategyEventComplete`：策略执行完成（成功或失败）

**实现原理**：
1. `ContextEngine` 持有 `strategyEventChan` 用于发送事件
2. `Agent` 在 `RunStreaming` 开始时设置事件 channel
3. 启动独立 goroutine 监听策略事件并转发到 TUI
4. TUI 接收事件后更新日志区域，显示"策略: truncation (运行中...)"或"(已完成)"

**TUI 展示效果**：
```
你: 请连续输出数字 1 到 100

回答: 好的，我将为您输出数字 1 到 100...

策略: truncation (运行中...)
策略: truncation (已完成)

回答: 1, 2, 3, ...
```

相关代码：`ch05/context/event.go`、`ch05/agent.go`、`ch05/tui/tui.go`

---

### 5. Token 计数

为了准确判断是否需要触发策略，我们需要实时计算上下文 token 数量。

**实现方式**：
- 使用 `tiktoken-go` 库（OpenAI 官方 tokenizer 的 Go 移植）
- 对 `cl100k_base` 编码进行计数（GPT-4/GPT-3.5 使用）
- 每次添加/删除消息时更新计数

**注意**：Token 计数是估算值，实际 API 调用可能有细微差异。

相关代码：`ch05/context/share.go`

---

## 💻 代码结构速览

### Context 包
- `ch05/context/context.go`：上下文引擎核心，管理消息列表和策略应用
- `ch05/context/strategy.go`：策略接口定义
- `ch05/context/event.go`：策略事件类型定义
- `ch05/context/truncate.go`：截断策略实现
- `ch05/context/offload.go`：卸载策略实现
- `ch05/context/summary.go`：摘要策略实现
- `ch05/context/share.go`：Token 计数工具

### 其他核心模块
- `ch05/storage/storage.go`：存储接口（用于卸载策略）
- `ch05/tool/load_storage.go`：加载存储数据的工具
- `ch05/agent.go`：集成上下文引擎的 Agent，支持策略事件转发
- `ch05/vo.go`：视图对象定义，包含策略事件 VO
- `ch05/tui/tui.go`：TUI 界面，展示策略执行状态

---

## 🚀 动手运行

进入项目根目录，执行：

```bash
go run ./ch05/tui
```

在 TUI 中尝试以下场景，观察上下文管理的效果：

**1. 测试截断策略**
```
请连续输出数字 1 到 100，每个数字单独输出
```

**2. 测试卸载策略**
```
请读取 README.md 并总结每一章节的内容
```

**3. 测试摘要策略**
```
我们来进行多轮对话：第一轮，请介绍你自己
第二轮，请列出当前目录的文件
第三轮，请读取 agent.go 文件
...
```

---

## ⚠️ 注意事项

1. **策略优先级**：当前实现按策略注册顺序执行，建议优先级为：摘要 > 卸载 > 截断
2. **摘要成本**：摘要策略需要额外的 LLM 调用，会产生额外的 API 费用
3. **存储选择**：生产环境中建议使用 Redis 或数据库替代内存存储
4. **阈值设置**：根据实际模型的上下文窗口调整触发阈值

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
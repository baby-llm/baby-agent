# 第三章：让 Agent “更能看见”（Reasoning 展示、TUI）

欢迎来到第三章！在第二章的基础上，本章继续完善一个更“可观察”的 Agent：

- **可视化展示推理与工具调用过程**（便于调试）
- **兼容推理模型的 `reasoning_content` / `reasoning` / `thinking` 字段**

本章依旧保留最小 Agent Loop 的结构，但在“输出可观察性”和“工具生态扩展性”上向前迈了一步。

---

## 🎯 你将学到什么

1. **推理字段兼容处理**：如何从流式响应中解析 `reasoning_content`、`reasoning`、`thinking`（以及不同厂商字段差异的处理思路）。
2. **TUI 可视化层**：用 Bubble Tea 实现一个轻量的可视化输出（这不是本课重点，仅为后续调试服务）。

---

## 🛠 准备工作

复用根目录的 `.env` 配置（见项目根目录 `README.md`）。

```env
OPENAI_API_KEY=sk-your-api-key-here
OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_MODEL=gpt-5.2
```

如果你本地使用的是 Ollama + Qwen3，可以改成：

```env
OPENAI_API_KEY=ollama
OPENAI_BASE_URL=http://localhost:11434/v1
OPENAI_MODEL=qwen3:8b
```

此外，本章默认在 `mcp-server.json` 中配置了一个 MCP 文件系统服务器（基于 `@modelcontextprotocol/server-filesystem`）。这需要本地能够运行 `npx`（通常意味着安装 Node.js）。

---

## 📖 核心原理解析

### 1. 推理模型的 `reasoning_content` 兼容处理


**推理模型 vs 非推理模型**

- 推理模型：强调“步骤化思考”，可能输出额外的推理字段，适合复杂任务、工具编排与多步规划，但输出更慢、成本更高。
- 非推理模型：更偏“直接生成”，通常只有最终内容字段，适合简单问答与低延迟场景。

部分“推理模型”在流式响应里会返回额外的推理字段，用于展示中间思考过程。**但不同厂商字段命名不一致**，常见情况包括：

- `reasoning_content`（OpenAI 兼容推理接口常见）
- `reasoning`（例如 Ollama 的 OpenAI 兼容接口）
- `thinking`（例如 Ollama 原生接口）

`ch03/agent.go` 中的 `RunStreaming` 把推理内容单独作为 `MessageTypeReasoning` 发送给 TUI，这样就能把“推理过程”和“最终内容”分开显示，利于调试与对齐。

> **实现状态**：`reasoning_content` 由项目使用的 baby-llm/openai-go fork 在 SDK 层原生解析（见根 README 的 SDK fork 说明）。`reasoning` / `thinking` 是部分厂商的别名字段，SDK 未建模，代码里用 `RawJSON()` 兜底解析，**仅用于展示**。
>
> 工具调用轮次中，SDK 的 `ChatCompletionMessage.ToParam()` 会自动把已累积的 `reasoning_content` 带回消息链，满足 DeepSeek 思考模式下“工具调用后必须回传推理内容”的要求，业务代码不需要手工拼接该字段。

> 例如，本地使用 Ollama 跑 `qwen3` 时，OpenAI 兼容接口通常返回 `reasoning`；若直接调用 Ollama 原生接口，则常见字段名为 `thinking`。

相关代码：`ch03/agent.go`

---

### 2. 推理模型与非推理模型的 Chat Template 差异 （可选阅读）

在第二章中，我们介绍了 Chat Template 将 API 的结构化消息转换为模型输入文本的过程。但对于**推理模型**（Reasoning Models）和**非推理模型**（Standard Models），它们在 Chat Template 的使用上存在显著差异。

#### 推理模型的特殊 Template 格式

推理模型（如 OpenAI o1/o3 系列、DeepSeek-R1 等）需要特殊的 Prompt 结构来触发推理模式。它们的 Chat Template 通常包含：

**1. 推理触发标记**
推理模型需要特定的系统提示词或特殊格式来激活推理能力：

```
<thinking>
请按照以下步骤思考问题：
1. 理解用户需求
2. 分析可用工具
3. 规划执行步骤
4. 逐步执行并验证
</thinking>
```

**2. 推理过程与最终输出的分离**

推理模型的 Chat Template 会将输出分为两个阶段：

```
<|im_start|>assistant

根据 README.md 的内容，这个项目的目标是...
<|im_end|>
```

API 响应中的 `reasoning_content` 字段对应 `<think>` 标签内的内容，而 `content` 字段对应最终输出。

#### 不同模型的 Template 对比

| 模型类型 | Template 特点 | 示例格式 |
|---------|--------------|---------|
| **标准模型**<br>(GPT-4, Claude-3) | 单阶段输出<br/>直接生成最终答案 | `<\|im_start\|>assistant\n答案是：42<\|im_end\|>` |
| **推理模型**<br>(o1, o3, R1) | 双阶段输出<br/>先推理后答案 | `<\|im_start\|>assistant\n<think>...推理过程...</think>\n最终答案<\|im_end\|>` |
| **混合模型**<br>(某些支持 reasoning 模式的模型) | 可选推理<br/>根据指令决定是否推理 | 根据系统指令决定是否生成 reasoning 字段 |

#### 推理模型的 Token 消耗特点

理解推理模型的 Chat Template 后，就能解释为什么推理模型的 Token 消耗更高：

```
标准模型对话：
User (10 tokens) → Assistant (100 tokens)
总计：110 tokens

推理模型对话：
User (10 tokens) → Assistant [thinking] (5000 tokens) → Assistant (100 tokens)
总计：5110 tokens
```

推理模型的 `<think>` 内容通常会被计入**推理 Token（Reasoning Tokens）**，这部分可能有单独的计费标准。

#### 实际开发中的处理策略

**1. 自动适配不同模型**

```go
// 根据模型类型选择不同的系统提示词
if isReasoningModel(modelName) {
    systemPrompt = "你是一个推理型助手。请在 <think> 标签中展示思考过程。"
} else {
    systemPrompt = "你是一个直接的助手。请简洁回答问题。"
}
```

**2. Template 解析的兼容性**

```go
// 解析流式响应时，需要区分 reasoning_content / reasoning / thinking 和 content
if chunk.ReasoningContent != nil {
    // 推理模型：显示在推理区域
    displayInThinkingArea(*chunk.ReasoningContent)
}
if chunk.Content != nil {
    // 最终内容：显示在答案区域
    displayInAnswerArea(*chunk.Content)
}
```

**3. 成本优化建议**

- **简单任务**使用标准模型，避免推理 Token 浪费
- **复杂任务**才使用推理模型，充分利用其推理能力
- 根据模型文档了解其推理 Token 计费规则

#### 主流推理模型的 Template 差异

| 模型系列 | Reasoning 字段名 | Template 特点 |
|---------|-----------------|--------------|
| **OpenAI o1/o3** | `reasoning_content` | 内置格式，无需特殊标签 |
| **DeepSeek-R1** | `reasoning_content` | 兼容 OpenAI 格式 |
| **Ollama + Qwen3** | `reasoning` / `thinking` | 取决于 OpenAI 兼容接口或原生接口 |
| **自建推理模型** | 自定义字段 | 需根据文档调整解析逻辑 |

---

### 3. TUI 只是"可视化外壳"


在 `ch03/tui/tui.go` 中，使用 Bubble Tea 搭建了一个轻量的 TUI：

- 以流式方式展示推理、工具调用、错误和最终内容
- 便于观察 Agent Loop 的执行轨迹

**流式输出与 UI 协程**

本章让 Agent 以流式方式输出，并通过 Go 的 `channel` 将增量消息传递给 UI 协程。`RunStreaming` 会持续向 `viewCh` 写入 `MessageVO`，而 TUI 侧以事件循环消费这些消息并渲染。这种“流式 + 通道”的模式可以在响应尚未完成时就持续显示过程输出。

这部分**不是本课程的核心内容**，也不需要深入理解 Bubble Tea 的内部机制。你只需要知道：TUI 的存在是为了让调试更直观，后续章节会频繁用到“可视化输出”。

相关代码：`ch03/tui/tui.go`


---

## 💻 代码结构速览

- `ch03/agent.go`：增强后的 Agent Loop（流式 + 多种 reasoning 字段解析 + MCP 支持）
- `ch03/tui/tui.go`：Bubble Tea TUI 可视化界面

---

## 🚀 动手运行

进入项目根目录，执行：

```bash
go run ./ch03/main
```

示例：

- “请读取 README.md 并总结项目目标”
- “列出当前目录下的文件”

如果服务正常启动，你会看到工具调用日志出现在 TUI 中。

---

## 📚 扩展阅读与参考资料

以下资料可帮助你进一步理解 TUI 相关内容：

1. Bubble Tea（TUI 框架）：`https://github.com/charmbracelet/bubbletea`

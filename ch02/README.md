# 第二章：赋予 AI “手脚”（Tool Calling 和 Agent）

> **前置**：ch01｜**本章新增**：Tool Calling、最小 Agent Loop、本地工具｜**下一章**：ch03

欢迎来到第二章！本章我们把 LLM 从“只会聊天”升级为“能动手做事”的小型 Agent：它不仅能回答问题，还能**读取/修改本地文件、写入新文件、执行命令**。

你将看到一个最小可用的 Agent 循环：LLM 先规划行动 -> 触发工具 -> 把工具结果喂回 LLM -> 直到产出最终答案。这就是后续复杂 Agent 的雏形。

---

## 🎯 你将学到什么

1. **Function Calling 的协议形态**：如何向模型声明工具、如何解析工具调用。
2. **最小 Agent Loop**：工具调用 -> 反馈 -> 再推理的闭环流程。
3. **本地工具封装**：读取文件、写入文件、编辑文件、执行 shell 命令。

---

## 🛠 准备工作

本章复用根目录的 `.env` 配置（见项目根目录 `README.md`）。

```env
OPENAI_API_KEY=sk-your-api-key-here
OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_MODEL=gpt-5.2
```

---

## 📖 核心原理解析

### 1. 工具的”声明”就是一份函数签名

在 `ch02/tool/*.go` 中，每个工具都通过 `Info()` 返回一个 `FunctionDefinition`，告诉模型：

- 工具名（如 `read`、`write`、`edit`、`bash`）
- 工具用途（描述）
- 参数结构（JSON Schema）

例如 `read` 工具的参数定义：

```go
type ReadToolParam struct {
    Path string `json:"path"`
}
```

模型会根据这个 schema 生成 `tool_call`，并附上 JSON 参数。

---

### 2. Agent Loop：工具调用 -> 反馈 -> 再推理

`ch02/agent.go` 中的核心逻辑：

1. 发送请求：带上 `messages` 和 `tools`。
2. 读取模型回复：
   - 如果没有 `tool_calls`，说明任务完成，返回最终内容。
   - 如果有 `tool_calls`，执行工具。
3. 将工具结果拼回 `messages`，进入下一轮推理。

伪流程如下：

```text
User -> LLM -> tool_calls -> execute tools -> tool results -> LLM -> final answer
```

---

### 3. ReAct / Tool-Loop 的原理

Function Calling 的本质是“外部能力的函数接口”，而 ReAct（Reason + Act）强调让模型在**思考**与**行动**之间交替推进任务。本章的 Agent Loop 正是一个最小化的 ReAct/Tool-Loop：

- **Reason（思考）**：模型根据系统提示与用户问题决定下一步要不要用工具，以及用哪个工具。
- **Act（行动）**：执行工具调用（读文件、写文件、执行命令）。
- **Observe（观察）**：把工具结果作为 `tool` 消息塞回上下文，让模型基于真实世界反馈继续推理。

这个循环的关键点是：**模型并没有“直接修改世界”，它只能通过工具调用获得反馈**。因此只要我们控制工具，就能控制 Agent 的边界与安全性。

---

### 4. 本章实现的四个工具

在 `ch02/tool/` 目录下：

- `read`：读取本地文件内容
- `write`：写入文件（覆盖）
- `edit`：按文本替换内容
- `bash`：执行命令并返回输出

这些工具都实现了统一接口：

```go
type Tool interface {
    ToolName() AgentTool
    Info() openai.ChatCompletionToolUnionParam
    Execute(ctx context.Context, argumentsInJSON string) (string, error)
}
```

这样 Agent 可以用统一方式注册、调用任何工具。

---

## 💻 代码结构速览

- `ch02/main/main.go`：程序入口，创建 Agent，注册工具
- `ch02/agent.go`：最小 Agent 循环（function calling 驱动）
- `ch02/tool/*.go`：本地工具实现
- `shared/config.go`：模型配置（ch02 通过 `shared.NewModelConfig()` 读取）
- `ch02/prompt.go`：系统提示词

---

## 🚀 动手运行

进入项目根目录，执行：

```bash
go run ./ch02/main -q "请读取 README.md 并总结项目目标"
```

你也可以尝试更“能动手”的指令：

```bash
go run ./ch02/main -q "在 ch02 目录下创建一个 TODO.md，内容为 1. 研究 agent"
```

---

## ⚠️ 安全提示

在赋予大模型执行本地系统命令（如 `bash` 工具）和修改文件的能力时，存在极高的安全风险。模型可能会因为幻觉或恶意指令注入（Prompt Injection）执行危险操作（如 `rm -rf`）。
在真实的生产环境中，**务必引入严格的安全策略**（例如：命令白名单、执行前的二次人工确认、或在 Docker 沙盒环境中隔离运行）。相关的高级安全防护策略将在后续章节详细探讨。

---

### Chat Template——从 API 到模型输入的桥梁（可选阅读）

当我们调用 OpenAI API 时，发送的是结构化的 `messages` 数组（包含 `role` 和 `content`）。但大语言模型（LLM）的底层是一个**文本生成模型**，它只能理解和生成纯文本序列。

**Chat Template** 就是将 API 的结构化消息转换为模型可理解文本格式的"翻译层"。

#### 转换过程示例

假设我们发送这样的 API 请求：

```json
{
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "What's the capital of France?"},
    {"role": "assistant", "content": "The capital of France is Paris."},
    {"role": "user", "content": "Tell me more."}
  ]
}
```

**Chat Template 会将其转换为类似这样的文本**：

```text
<|im_start|>system
You are a helpful assistant.<|im_end|>
<|im_start|>user
What's the capital of France?<|im_end|>
<|im_start|>assistant
The capital of France is Paris.<|im_end|>
<|im_start|>user
Tell me more.<|im_end|>
<|im_start|>assistant
```

#### Chat Template 的实现：Jinja2 模板语言

绝大多数开源模型（如 DeepSeek、Qwen、GLM 等）在 HuggingFace 上发布时，都会在 `tokenizer_config.json` 中内置一个 `chat_template` 字段，使用 **Python Jinja2** 模板语法来定义消息格式。

以 [Qwen3-8B 的 tokenizer_config.json](https://huggingface.co/Qwen/Qwen3-8B/blob/main/tokenizer_config.json) 为例，其 `chat_template` 字段是一段 Jinja2 模板：

```jinja2
{%- if tools %}
    {{- '<|im_start|>system\n' }}
    ...
    {{- "# Tools\n\nYou may call one or more functions..." }}
{%- endif %}
{%- for message in messages %}
    {{- '<|im_start|>' + message['role'] + '\n' + message['content'] | trim + '<|im_end|>\n' }}
{%- endfor %}
```

这个模板由 `transformers` 库的 `apply_chat_template()` 方法自动执行，开发者无需手写。理解它的存在，能帮你在调试时直接查看模型的"真实期望格式"。

#### 不同模型的 Template 格式

不同模型使用不同的 Chat Template 格式（点击链接可在 HuggingFace 查看完整模板）：

| 模型系列 | 格式示例 | HuggingFace 模板 |
|---------|---------|-----------------|
| **DeepSeek-V3** | `<\|im_start\|>role\ncontent<\|im_end\|>` | [查看模板](https://huggingface.co/deepseek-ai/DeepSeek-V3-0324/blob/main/tokenizer_config.json) |
| **Qwen3** | `<\|im_start\|>role\ncontent<\|im_end\|>` | [查看模板](https://huggingface.co/Qwen/Qwen3-8B/blob/main/tokenizer_config.json) |
| **GLM-4.5** | `[gMASK]<sop><\|system\|>\ncontent<\|user\|>\ncontent<\|assistant\|>` | [查看模板](https://huggingface.co/zai-org/GLM-4.5/blob/main/chat_template.jinja) |

> **提示**：在 HuggingFace 模型页面，打开 `Files and versions` → `tokenizer_config.json`，搜索 `chat_template` 字段即可看到该模型完整的 Jinja2 模板。

#### Tool Calling 的 Template 转换

当涉及工具调用时，Chat Template 的转换更为关键。API 发送的工具定义和调用会被转换为特殊格式：

**API 格式**：
```json
{
  "tools": [{"type": "function", "function": {"name": "get_weather", "parameters": {...}}}],
  "messages": [
    {"role": "user", "content": "What's the weather in Beijing?"},
    {"role": "assistant", "tool_calls": [{"id": "call_123", "function": {"name": "get_weather", "arguments": "{\"city\": \"Beijing\"}"}}]},
    {"role": "tool", "tool_call_id": "call_123", "content": "Sunny, 25°C"}
  ]
}
```

**转换为模型输入**（简化示意）：
```text
<|im_start|>system
You have access to the following functions:
- get_weather: Get the current weather for a city

<|im_start|>user
What's the weather in Beijing?<|im_end|>
<|im_start|>assistant
<tool_call>[{"name": "get_weather", "arguments": "{\"city\": \"Beijing\"}"}]<tool_end><|im_end|>
<|im_start|>tool
Sunny, 25°C<|im_end|>
<|im_start|>assistant
```

#### 为什么理解 Chat Template 很重要？

1. **调试问题时**：当模型输出不符合预期，理解 template 帮助你判断是模型问题还是 prompt 格式问题
2. **切换模型时**：不同模型的 template 不同，可能导致相同 prompt 表现不同
3. **自定义 Prompt 时**：有时需要手动模拟 template 格式来绕过 API 限制
4. **理解 Token 计算**：template 会额外消耗 token（如 `<|im_start|>` 等）

对于本章的 Function Calling，SDK 会自动处理 Chat Template 转换，你只需关注 API 层的 `messages` 和 `tools` 结构。但理解转换原理有助于排查"为什么模型不调用工具"等问题。

---

## 📚 扩展阅读与参考资料

为了更深入地掌握 Function Calling，推荐阅读以下官方资源：

1. **[OpenAI Function Calling 官方文档](https://platform.openai.com/docs/guides/function-calling)**
   - 详细介绍了工具调用的完整协议格式、JSON Schema 的编写技巧以及常见用例。
2. **[OpenAI Go SDK GitHub 仓库](https://github.com/openai/openai-go)**
   - 深入 SDK 源码，查看 `ChatCompletionToolUnionParam` 等底层结构是如何构建工具调用请求的。

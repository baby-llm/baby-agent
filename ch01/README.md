# 第一章：初识 LLM（Raw HTTP 与 OpenAI SDK）

欢迎来到第一章！作为后端工程师，我们每天都在和各种 API 打交道。AI 大模型的调用本质上也是一次 HTTP 请求。

本章的目标是带你**拨开 SDK 的迷雾，直视大模型调用的本质**。我们将分别使用 Go 标准库 `net/http`（原生手写请求）和官方 `openai-go` SDK 来完成同一个任务：向 LLM 发起对话。这不仅能帮助你深刻理解 OpenAI 协议与 SSE（流式输出）机制，更为后续开发健壮的 AI Agent 奠定坚实的基础。

---

## 🎯 你将学到什么

1. **协议本质**：掌握 `chat/completions` 接口的最小化请求和响应结构。
2. **流式输出解析**：深入了解如何解析 Server-Sent Events (SSE) 协议，实现“打字机”效果。
3. **工程实践**：对比 `Raw HTTP` 和 `OpenAI Go SDK` 的使用方式。

---

## 🛠 准备工作

在开始之前，请确保你已经准备好了环境配置。我们在项目根目录使用了 `.env` 来管理敏感信息（`main.go` 中通过 `godotenv.Load()` 自动读取）。

在项目根目录创建 `.env` 文件，并填入以下内容：

```env
# 如果你使用的是国内模型（如 DeepSeek/GLM），请修改 Base URL 和 Model
OPENAI_API_KEY=sk-your-api-key-here
OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_MODEL=gpt-4o-mini
```

> **💡 小贴士**：得益于 OpenAI 协议的非正式标准化，只需更改 `BASE_URL` 和 `MODEL`，同一套代码就能无缝切换到绝大多数主流大模型厂商。

---

## 📖 核心原理解析

### 1. 最小请求结构 (Request)

发起一次对话的核心路径是：`POST {OPENAI_BASE_URL}/chat/completions`

我们来看 `raw.go` 中定义的结构体，这就是发送给大模型的核心数据：

```go
type OpenAIChatCompletionRequest struct {
	Model    string           `json:"model"`    // 例如："gpt-4o-mini"
	Messages []RequestMessage `json:"messages"` // 对话上下文历史
	Stream   bool             `json:"stream"`   // 是否开启流式增量返回
}
```
*   `messages` 是一个数组，大模型本身是**无记忆**的，你需要把之前的聊天记录一并传给它，这就是所谓的“上下文”。
*   `stream` 是控制体验的关键，开启后将大幅降低首字响应时间 (TTFT)。

### 2. 标准与流式响应的区别 (Response vs SSE)

**非流式模式 (`Stream: false`)**
模型会思考完毕后，一次性返回一个巨大的 JSON。在 `raw.go` 中我们解析 `OpenAIChatCompletionResponse`：
你需要提取的内容在 `choices[0].message.content` 中。同时，`usage` 字段会告诉你这次调用花了多少 Token。

**流式模式 (`Stream: true`)**
这是主流 AI 产品的标准体验。模型会通过 **SSE (Server-Sent Events)** 协议，像水流一样逐字吐出数据。

如果你使用原生 HTTP 请求（见 `raw.go` 中的 `StreamingRequestRawHTTP`），你会看到服务端返回的是这样逐行的纯文本：
```text
data: {"choices":[{"delta":{"content":"Hi"}}]}
data: {"choices":[{"delta":{"content":"!"}}]}
data: [DONE]
```
在 Go 中，我们通常使用 `bufio.NewScanner(httpResp.Body)` 来逐行读取，遇到 `data: [DONE]` 标志着生成结束。你需要将每个 chunk 中 `delta.content` 的内容拼接起来。

---

## 💻 代码实现对比

本项目提供了两套完整的实现，通过命令行参数进行切换。

### 方式一：使用官方 SDK (推荐在生产使用)
在 `sdk.go` 中，我们使用了官方的 `github.com/openai/openai-go`。代码非常简洁：
```go
// 流式调用的核心逻辑
stream := client.Chat.Completions.NewStreaming(ctx, req)
for stream.Next() {
    chunk := stream.Current()
    log.Printf("stream chunk: %v", chunk)
}
```
SDK 帮我们封装了底层的 HTTP 请求、JSON 序列化、SSE 解析以及错误重试逻辑。在后续章节中，我们将统一采用这种方式。

### 方式二：Raw HTTP 手写调用 (推荐学习原理)
在 `raw.go` 中，我们使用 Go 原生的 `net/http` 发起调用。这对于排查网络问题、理解底层通信机制、或者在不方便引入庞大 SDK 的极简项目中非常有帮助。

---

## 🧠 LLM 原理深度解析（可选阅读）

在掌握了如何调用大模型之后，让我们深入了解大语言模型（LLM）的工作原理。理解这些原理将帮助你更好地设计 Prompt、优化性能以及构建更可靠的 AI Agent。

### 什么是大语言模型？

大语言模型（Large Language Model，LLM）是一种基于深度学习的神经网络模型，通过在海量文本数据上进行预训练，学习语言的统计规律和语义表示。其核心能力包括：

- **文本理解**：能够理解复杂指令和上下文
- **内容生成**：可以生成连贯、有逻辑的文本
- **推理能力**：具备一定程度的逻辑推理和知识运用能力
- **多任务适应**：通过 Prompt 即可完成各种任务，无需特定训练

### Transformer 架构：LLM 的基石

现代大模型普遍基于 **Transformer** 架构（2017年由 Google 提出）。Transformer 的核心创新是**自注意力机制（Self-Attention）**，它让模型能够：

1. **并行处理**：相比 RNN/LSTM，Transformer 可以并行处理序列中的所有 token
2. **长距离依赖**：能够捕捉相隔很远的词语之间的关联
3. **灵活的上下文理解**：每个 token 都能关注到整个输入序列

**注意力机制示例**：
```
输入："The cat sat on the mat"
当处理 "cat" 时，模型会同时关注 "The"、"sat"、"on"、"mat"
```

### 训练过程：预训练 + 微调

**1. 预训练（Pre-training）**
- 目标：在海量文本（互联网、书籍、代码等）上学习语言知识
- 任务：通常是"下一个词预测"（Next Token Prediction）
- 结果：模型获得了世界知识和语言理解能力，但只是个"续写机器"

**2. 有监督微调（SFT，Supervised Fine-Tuning）**
- 目标：让模型学会遵循指令和对话
- 数据：高质量的（指令，回答）对
- 结果：模型变成了"对话助手"

**3. 对齐训练（如 RLHF）**
- 目标：让模型的输出更符合人类价值观
- 方法：通过人类反馈强化学习
- 结果：模型更安全、更有用、更诚实

### Token：模型的基本单位

LLM 不直接处理文本，而是将文本切分成 **Token**：

- Token 可以是单词、词根、字符或字符的组合
- 切分使用 **BPE（Byte Pair Encoding）** 或类似算法
- 不同的模型使用不同的 Tokenizer

**Token 化示例**（以 GPT 为例）：
```
"Hello, world!" → ["Hello", ",", " world", "!"]
"人工智能" → ["人", "工", "智", "能"] 或 ["人工", "智能"]
```

**Token 的重要性**：
- 计费单位：API 调用按 Token 数量计费
- 上下文限制：模型有最大 Token 数限制（如 128K、1M 等）
- 影响输出：不同语言的 Token 密度不同（中文通常需要更多 Token）

### 生成原理：概率预测

LLM 的生成过程本质上是**迭代的下一个 Token 预测**：

```
输入: "What is the capital of"
模型预测下一个 token 的概率分布:
  " of": 0.001
  " France": 0.15
  " China": 0.08
  " the": 0.001
  ...

采样策略选择: " France"
新输入: "What is the capital of France"
继续预测...
```

**关键参数**：
- `temperature`：控制输出的随机性（0=确定性，1=更随机）
- `top_p`：核采样，只考虑累积概率达到 p 的 tokens
- `max_tokens`：限制输出的最大长度

### 为什么 LLM 能"理解"？

LLM 的"理解"本质上是**模式识别和统计关联**，而非人类式的认知：

1. **分布式表示**：每个 token 被映射为高维向量，捕获语义信息
2. **上下文编码**：通过注意力机制，每个 token 的表示都融合了上下文
3. **概率推理**：基于训练数据中学到的模式，生成最可能的续写

**局限性**：
- 可能产生"幻觉"（编造事实）
- 数学计算能力有限（虽然可以通过工具增强）
- 缺乏真实世界的体验和常识更新

---

## 🚀 动手运行

你可以通过命令行标志（Flags）随意组合调用方式。进入项目根目录后，执行以下命令：

**1. 基础调用（SDK + 非流式，默认）**
```bash
go run ./ch01/main -q "讲一个关于程序员的冷笑话"
```

**2. 体验流式输出（SDK + 流式）**
注意观察控制台日志，体会 `chunk` 是一块块返回的：
```bash
go run ./ch01/main --stream -q "用 Go 语言写一个 Hello World"
```

**3. 探索底层原理（Raw HTTP + 非流式）**
```bash
go run ./ch01/main --raw -q "什么是大语言模型？用一句话解释"
```

**4. 终极挑战：手写 SSE 解析（Raw HTTP + 流式）**
```bash
go run ./ch01/main --raw --stream -q "从 1 数到 5"
```

---

## 📚 扩展阅读与参考资料

为了更深入地理解本章涉及的内容，强烈推荐阅读以下官方文档：

1.  **[OpenAI API Reference - Chat Completions](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)**
   *   了解所有可用的请求参数（如 `temperature`, `top_p`, `presence_penalty` 等），以及完整的响应数据结构。这是构建任何基于 OpenAI 协议的 Agent 的基石。
2.  **[MDN Web Docs - Server-sent events (SSE)](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events/Using_server-sent_events)**
   *   虽然这里展示的是 Web 前端的文档，但 SSE 协议的本质是一样的。了解 SSE 协议的标准格式（如 `data:`, `event:`, `id:` 等），有助于你理解大模型流式输出的底层机制。
3.  **[openai-go SDK GitHub Repository](https://github.com/openai/openai-go)**
   *   官方 Go SDK 的源码仓库。如果你想了解 SDK 是如何封装重试逻辑、处理错误回调的，阅读其源码是非常好的学习途径。
4.  **[DeepSeek API Docs](https://api-docs.deepseek.com)** (或者你常用的国内模型文档)
   *   看看其他厂商是如何实现兼容 OpenAI 格式的 API 接口的，通常只需要替换 Base URL 和模型名即可。

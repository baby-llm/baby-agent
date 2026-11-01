# Baby Agent 学习日志

## ch01：初识 LLM（Raw HTTP 与 OpenAI SDK）

### 学习目标

理解一次大模型对话调用的本质，并能区分 SDK、原生 HTTP、非流式响应和 SSE 流式响应。

### 核心内容

1. **LLM 调用的本质是 HTTP 请求**

   调用路径为 `POST {OPENAI_BASE_URL}/chat/completions`。请求通常包含：

   ```json
   {
     "model": "gpt-5.2",
     "messages": [{"role": "user", "content": "你好"}],
     "stream": false
   }
   ```

   - `model`：本次调用使用的模型。
   - `messages`：模型需要读取的完整对话上下文。
   - `stream`：控制一次性返回结果还是增量返回结果。

2. **非流式与流式响应的差异**

   - 非流式：模型生成完成后，返回一个完整 JSON；文本在 `choices[0].message.content`。
   - 流式：服务端通过 SSE 持续发送 `data:` 行；每一段新增文本在 `choices[0].delta.content`；`data: [DONE]` 表示生成结束。

3. **SSE 流式解析流程**

   ```text
   SSE 文本行 → 取出 data 内容 → 解析 JSON chunk → 读取 delta → 累积并展示
   ```

   `bufio.Scanner` 逐行读取响应即可，它会等待下一段数据，因此普通终端输出不需要额外的 goroutine 或 channel。

4. **SDK 与 Raw HTTP 的分工**

   - `sdk.go`：官方 SDK 封装 HTTP 请求、JSON 编解码和 SSE 解析，适合业务开发。
   - `raw.go`：使用 `net/http` 手写协议细节，适合理解底层、调试和排查兼容性。

5. **“模型记忆”来自应用层上下文**

   LLM 的每一次 API 调用彼此独立。应用必须在下一次请求中重新携带先前的 user/assistant 消息，才能营造连续对话的效果。

   ```text
   用户问题 → 写入 history → 携带 history 请求模型
            → 收集完整回答 → 将回答写入 history → 下一轮复用 history
   ```

   流式输出时，应该先将多个 `delta` 拼接为完整回答，收到 `[DONE]` 后再作为一条 assistant 消息写入 history；不要将每个 chunk 都存为一条消息。

### 代码定位

- `ch01/main/main.go`：读取 `-raw`、`-stream`、`-q` 参数，并选择四种调用模式。
- `ch01/raw.go`：原生 HTTP 的非流式和 SSE 流式实现，以及请求/响应结构体。
- `ch01/sdk.go`：使用 `openai-go` SDK 的非流式和流式实现。
- `shared/config.go`：从 `.env` 中读取 `OPENAI_BASE_URL`、`OPENAI_API_KEY`、`OPENAI_MODEL`。

### 关键代码与解释

#### 1. 入口根据参数选择调用方式（`ch01/main/main.go`）

```go
switch {
case *useRaw && *useStream:
	ch01.StreamingRequestRawHTTP(ctx, modelConf, *query)
case *useRaw:
	ch01.NonStreamingRequestRawHTTP(ctx, modelConf, *query)
case *useStream:
	ch01.StreamingRequestSDK(ctx, modelConf, *query)
default:
	ch01.NonStreamingRequestSDK(ctx, modelConf, *query)
}
```

两个 flag 组合出四种模式：原生 HTTP/SDK 与流式/非流式。它让相同的用户问题可以在不同实现方式下对照运行。

#### 2. 原生 HTTP 请求的构造（`ch01/raw.go`）

```go
requestBody := OpenAIChatCompletionRequest{
	Messages: []RequestMessage{{Role: "user", Content: query}},
	Model:    modelConf.Model,
	Stream:   false,
}
bodyBytes, _ := json.Marshal(requestBody)

httpReq, _ := http.NewRequestWithContext(
	ctx, "POST", fmt.Sprintf("%s/chat/completions", modelConf.BaseURL), bytes.NewReader(bodyBytes),
)
httpReq.Header.Set("Content-Type", "application/json")
httpReq.Header.Set("Authorization", "Bearer "+modelConf.ApiKey)
```

这段代码将 Go 结构体编码为 JSON，创建带取消能力的 HTTP 请求，并设置 JSON 类型和 Bearer Token 鉴权。它揭示了 SDK 最终替我们完成的底层协议工作。

#### 3. 原生 SSE 响应的读取（`ch01/raw.go`）

```go
scanner := bufio.NewScanner(httpResp.Body)
for scanner.Scan() {
	line := scanner.Text()
	if !strings.HasPrefix(line, "data:") {
		continue
	}

	v := strings.TrimPrefix(line, "data:")
	if strings.TrimSpace(v) == "[DONE]" {
		break
	}

	var chunk OpenAIChatCompletionStreamChunk
	if err := json.Unmarshal([]byte(v), &chunk); err != nil {
		return
	}
}
```

`Scanner` 按行等待 SSE 事件；`data:` 之后是 JSON 数据，`[DONE]` 是结束标记。解析后的新增正文位于 `chunk.Choices[0].Delta.Content`，实际产品中应立即显示并用 `strings.Builder` 累积完整答案。

#### 4. SDK 流式读取（`ch01/sdk.go`）

```go
stream := client.Chat.Completions.NewStreaming(ctx, req)
for stream.Next() {
	chunk := stream.Current()
	log.Printf("stream chunk: %s", chunk.RawJSON())
}
if stream.Err() != nil {
	return
}
```

SDK 将请求发送、SSE 分帧和 JSON 解析封装为 `Next()`/`Current()` 迭代接口；业务代码只需消费每个 chunk 并在循环结束后检查 `stream.Err()`。

### Question & Answer

#### Q1：为什么流式响应读取 `delta.content`，而非流式响应读取 `message.content`？

**A：**流式响应会多次返回内容，每个 chunk 只表示相对之前结果的新增部分，因此使用 `delta.content`。非流式响应只在生成结束后返回一次完整结果，所以使用 `message.content`。

#### Q2：如何让原生流式版本真正逐字输出？

**A：**从 `chunk.Choices[0].Delta.Content` 读取增量，在扫描 SSE 行的同一个循环中立即 `fmt.Print(delta)`，同时用 `strings.Builder` 累积完整回答。不需要 channel 或额外 goroutine，因为读取 SSE 的循环已天然按到达顺序阻塞等待数据。

```go
var answer strings.Builder

delta := chunk.Choices[0].Delta.Content
fmt.Print(delta)
answer.WriteString(delta)
```

#### Q3：如果流中同时有思考内容和正式回答，如何处理？

**A：**分别读取并累积 `delta.reasoning_content` 与 `delta.content`。前者在本项目中是 `*string`，必须先判断是否为 `nil`；后者是普通 `string`。产品界面通常将思考内容放进可折叠面板，不与正式答案混排。

```go
if delta.ReasoningContent != nil {
	reasoning.WriteString(*delta.ReasoningContent)
}
if delta.Content != "" {
	answer.WriteString(delta.Content)
	fmt.Print(delta.Content)
}
```

#### Q4：为什么说 LLM 没有记忆？如何实现连续对话？

**A：**模型服务不会自动保存上一次 API 请求的内容。客户端必须维护 `messages` 历史：发送用户消息，获得完整的 assistant 回答后保存它；下一轮请求时把两者都一起发送。对话变长会消耗更多 Token 并受到上下文窗口限制，这也是后续“上下文压缩、摘要、长期记忆”机制的动机。

## ch02：赋予 AI “手脚”（Tool Calling 和 Agent）

### 学习目标

理解 Function Calling 的协议角色，以及一个最小 Agent 如何通过“模型决策 → 工具执行 → 结果反馈”的循环完成真实任务。

### 核心内容

1. **工具调用不等于模型直接执行代码**

   模型只能输出结构化的工具调用请求，例如：

   ```json
   {
     "name": "read",
     "arguments": "{\"path\":\"README.md\"}"
   }
   ```

   程序收到请求后，才会在本机执行对应的 Go 工具代码，并将真实执行结果返给模型。

2. **统一工具接口**

   `ch02/tool/tool.go` 中的 `Tool` 接口将所有工具统一为三项能力：

   ```go
   type Tool interface {
   	ToolName() AgentTool
   	Info() openai.ChatCompletionToolUnionParam
   	Execute(ctx context.Context, argumentsInJSON string) (string, error)
   }
   ```

   - `ToolName()`：工具的唯一名称。
   - `Info()`：向模型声明工具名称、用途和 JSON Schema 参数。
   - `Execute()`：解析模型参数并执行真实操作。

3. **最小 Agent Loop**

   ```text
   用户问题
     → LLM 判断是否调用工具
     → assistant tool_calls
     → 程序执行工具
     → tool result 追加到 messages
     → LLM 基于真实反馈继续推理
     → 没有 tool_calls 时返回最终回答
   ```

   `ch02/agent.go` 中每轮都携带对话历史和已注册的工具。模型回复后，必须先将包含 `tool_calls` 的 assistant 消息写入 `messages`，再追加使用相同 `tool_call_id` 的 tool 消息；这是工具调用协议的必要消息顺序。

4. **本章提供的工具**

   - `read`：读取文件内容。
   - `write`：创建或覆盖文件。
   - `edit`：按文本查找并替换文件内容。
   - `bash`：使用 shell 执行命令并返回输出。

   新增工具时只需实现 `Tool` 接口并注册，Agent 主循环无需修改。

5. **Agent 的职责边界**

   ```text
   LLM：理解需求、规划行动、归纳结果
   工具：获取事实、执行动作
   程序：控制权限、保存上下文、处理协议
   ```

   工具能力决定 Agent 能影响的现实范围，也决定安全边界。本章的任意路径读写和任意 shell 命令仅适用于受控教学环境；生产环境应限制路径和命令、使用沙箱，并对高风险操作增加确认。

### 代码定位

- `ch02/main/main.go`：创建 Agent、注册 read/edit/write/bash 工具并运行请求。
- `ch02/agent.go`：维护 `messages`，实现 Tool Calling 循环。
- `ch02/prompt.go`：规定修改前读取、必要时修改后验证等 Agent 行为准则。
- `ch02/tool/tool.go`：工具统一接口与工具名定义。
- `ch02/tool/read.go`、`write.go`、`edit.go`、`bash.go`：四种本地工具实现。

### 关键代码与解释

#### 1. 注册工具并初始化对话（`ch02/agent.go`）

```go
a.tools = make(map[tool.AgentTool]tool.Tool)
for _, t := range tools {
	a.tools[t.ToolName()] = t
}
a.messages = append(a.messages, openai.SystemMessage(systemPrompt))
```

工具以名称为键注册到 map 中，后续模型返回工具名时，Agent 能快速定位实现。系统消息是整段会话的第一条上下文，用于约束模型的行为规范。

#### 2. 向模型发送历史与工具定义（`ch02/agent.go`）

```go
params := openai.ChatCompletionNewParams{
	Model:    a.model,
	Messages: a.messages,
	Tools:    make([]openai.ChatCompletionToolUnionParam, 0),
}
for _, t := range a.tools {
	params.Tools = append(params.Tools, t.Info())
}
resp, err := a.client.Chat.Completions.New(ctx, params)
```

`Messages` 提供当前对话和先前工具结果；`Tools` 是工具的 JSON Schema 声明。模型依据这两部分生成普通回答或 `tool_calls`，但不会自行执行工具。

#### 3. 保存 assistant 工具调用并返回最终回答（`ch02/agent.go`）

```go
message := resp.Choices[0].Message
a.messages = append(a.messages, message.ToParam())

if len(message.ToolCalls) == 0 {
	result = message.Content
	break
}
```

先保存 assistant 消息是协议要求：它包含 tool call 的 ID 和参数。没有 `ToolCalls` 时，说明模型已经具备足够信息，`Content` 就是本轮最终回答。

#### 4. 执行调用并反馈观察结果（`ch02/agent.go`）

```go
for _, toolCall := range message.ToolCalls {
	toolResult, err := a.execute(ctx, toolCall.Function.Name, toolCall.Function.Arguments)
	if err != nil {
		toolResult = err.Error()
	}
	a.messages = append(a.messages, openai.ToolMessage(toolResult, toolCall.ID))
}
```

每一个工具调用都使用模型给出的名称和 JSON 参数执行。无论成功还是失败，结果都会作为带有对应 `toolCall.ID` 的 `tool` 消息写回历史，使模型在下一轮推理时能观察真实世界状态。

#### 5. `read` 工具的参数解析与文件读取（`ch02/tool/read.go`）

```go
var p ReadToolParam
if err := json.Unmarshal([]byte(argumentsInJSON), &p); err != nil {
	return "", err
}

content, err := io.ReadAll(file)
if err != nil {
	return "", err
}
return string(content), nil
```

模型传来的 `arguments` 是 JSON 字符串，工具先反序列化为强类型参数，再读取文件并将内容返回给 Agent。返回内容随后会进入 `tool` 消息，而不是直接绕过模型展示给用户。

### Question & Answer

#### Q1：为什么总结 `README.md` 时应调用 `read`，而不是直接根据模型知识回答？

**A：**模型不知道当前工作区中 `README.md` 的实际、最新内容。直接回答只能依据一般模式猜测，容易产生幻觉；调用 `read` 后，模型才能根据工具返回的真实文件内容总结。

#### Q2：为什么调用 `write` 创建文件后，通常还应调用一次 `read`？

**A：**这是执行后验证。成功返回不必然代表文件处于完全符合预期的最终状态；重新读取可以确认路径、内容、格式和写入结果。当前 `write` 成功时返回空字符串，因此 `read` 是获得真实落盘内容的直接方法。

```text
理解需求 → read（修改前了解现状）
         → write / edit（执行）
         → read（验证结果）
         → 向用户总结
```

#### Q3：为什么工具失败后应将错误返回给模型，而非立即终止 Agent？

**A：**错误也是观察结果。将“文件不存在”等错误作为 `tool` 消息传回模型，模型才能基于真实状态决定重试、改用其他工具、创建文件或向用户澄清；若程序立即终止，Agent 就无法进行下一步决策。

```text
Reason（判断）→ Act（调用工具）→ Observe（成功结果或错误）→ Reason（调整）
```

## ch03：让 Agent “更能看见”（Reasoning 展示、TUI）

### 学习目标

理解如何将流式 Agent 的推理、正文、工具调用和错误转换为可观察事件，并由终端 TUI 实时展示；掌握取消请求时如何保护会话历史。

### 核心内容

1. **Agent 与 TUI 通过事件协议解耦**

   Agent 不把 OpenAI SDK 的原始 chunk 直接交给界面，而是转换为 `MessageVO`，并通过 Go channel 传递。

   ```text
   模型流式 chunk → Agent 解析/归类 → MessageVO → channel → TUI 渲染
   ```

   事件类型包含 `reasoning`、`content`、`tool_call`、`error`。TUI 只关心事件类型和展示内容，不依赖具体模型或 SDK 的数据结构。

2. **兼容不同供应商的推理字段**

   不同 OpenAI-compatible 服务可能使用 `reasoning_content`、`reasoning` 或 `thinking` 保存推理文本。本章将 Raw JSON 解析到兼容结构，并按以下优先级选择展示内容：

   ```text
   reasoning_content → reasoning → thinking
   ```

   `content` 则作为正式回答单独展示，避免与推理文本混排。

3. **本轮历史采用临时副本，成功后提交**

   `RunStreaming` 从 `a.messages` 复制出局部 `messages`，在其中追加本轮 user、assistant tool calls、tool result 和最终 assistant 消息。只有整轮成功结束才执行 `a.messages = messages`。

   这不是为了避免重复发送历史，而是避免网络失败、工具失败或用户取消时的半截消息污染下一轮会话。

4. **工具调用不会让流式任务立即结束**

   模型发出 `tool_calls` 后，Agent 执行工具并把结果写回 `messages`，然后回到外层循环，再次请求模型；只有 assistant 消息不包含 `tool_calls` 时才结束本轮。

5. **取消由 context 向下游传播**

   TUI 按 `Esc` 后调用 `cancel()`。虽然 `RunStreaming` 未直接监听 `ctx.Done()`，但它将 `ctx` 传给 SDK 的流式请求和 `exec.CommandContext`：前者会终止 HTTP 流，后者会终止 bash 子进程。流错误返回后，本轮临时历史不会提交。

6. **当前工具注册范围**

   虽然 `tool.go` 定义了 read/write/edit/bash 四个工具名，`ch03/main/main.go` 实际仅注册 `BashTool`。因此当前程序读取 `README.md` 时，需要模型通过 `bash` 执行 `cat README.md` 等命令完成。

### 代码定位

- `ch03/agent.go`：流式 Agent Loop、工具调用循环、临时会话历史、推理字段兼容解析。
- `ch03/vo.go`：Agent 发往 TUI 的 `MessageVO` 事件模型。
- `ch03/tui/tui.go`：Bubble Tea 事件循环、channel 消费、日志渲染、取消与回滚。
- `ch03/main/main.go`：初始化 TUI，当前只注册 bash 工具。
- `ch03/tool/bash.go`：使用 `exec.CommandContext` 执行可取消的 shell 命令。

### 关键代码与解释

#### 1. 事件模型（`ch03/vo.go`）

```go
const (
	MessageTypeReasoning = "reasoning"
	MessageTypeContent   = "content"
	MessageTypeToolCall  = "tool_call"
	MessageTypeError     = "error"
)

type MessageVO struct {
	Type             string
	ReasoningContent *string
	Content          *string
	ToolCall         *ToolCallVO
}
```

`MessageVO` 是 Agent 与 TUI 的边界对象。`Type` 决定界面如何处理当前事件，指针字段让某类事件只携带所需数据。

#### 2. 创建临时历史（`ch03/agent.go`）

```go
messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(a.messages))
messages = append(messages, a.messages...)
messages = append(messages, openai.UserMessage(query))
```

本轮不直接修改正式的 `a.messages`，而是在其副本上工作。这样取消或失败时丢弃局部变量即可，不会保留不完整的 user 消息、工具调用或半截回答。

#### 3. 流式 chunk 转换为 UI 事件（`ch03/agent.go`）

```go
delta, err := parseDeltaWithReasoning(deltaRaw.RawJSON())
if reasoningContent := delta.ReasoningText(); reasoningContent != "" {
	viewCh <- MessageVO{Type: MessageTypeReasoning, ReasoningContent: &reasoningContent}
}
if delta.Content != "" {
	content := delta.Content
	viewCh <- MessageVO{Type: MessageTypeContent, Content: &content}
}
```

`RawJSON()` 允许处理 SDK 未强类型定义的供应商扩展字段。推理和正文以不同事件发送，TUI 就能分别累积、显示它们。

#### 4. 工具结果驱动下一次模型请求（`ch03/agent.go`）

```go
if len(message.ToolCalls) == 0 {
	break
}

for _, toolCall := range message.ToolCalls {
	toolResult, err := a.execute(ctx, toolCall.Function.Name, toolCall.Function.Arguments)
	if err != nil {
		toolResult = err.Error()
	}
	messages = append(messages, openai.ToolMessage(toolResult, toolCall.ID))
}
```

只有没有工具调用时才跳出外层循环。否则，工具结果会与对应的 `toolCall.ID` 一起追加到临时历史，随后下一次循环将其发送给模型，用于下一步决策。

#### 5. 成功提交和 TUI 后台运行（`ch03/agent.go`、`ch03/tui/tui.go`）

```go
a.messages = messages
```

```go
go func() {
	err := m.agent.RunStreaming(ctx, query, streamC)
	close(streamC)
	doneC <- err
	close(doneC)
}()
```

前者只在 `RunStreaming` 正常完成后执行，用完整的新历史替换正式历史。后者让耗时的网络流在后台运行，TUI 主事件循环能持续消费 `streamC`，渲染增量内容并响应 `Esc` 取消。

### Question & Answer

#### Q1：为什么要把 SDK chunk 转换成带 `Type` 的 `MessageVO`？

**A：**为了区分并分别展示推理、工具调用、回答和错误等不同类型的内容。`MessageVO` 也使 TUI 不必依赖 SDK chunk 的具体结构。

#### Q2：为什么需要兼容多个推理字段？本章的优先级是什么？

**A：**不同模型供应商的推理字段名可能不同。当前优先级是：`reasoning_content > reasoning > thinking`。

#### Q3：工具调用后为什么要再次请求模型？

**A：**Agent 必须将工具调用结果传递给模型，模型才能依据真实结果做出下一步决策，决定继续调用工具或给出最终回答。

#### Q4：`RunStreaming` 没有显式检测 `ctx.Done()`，取消当前模型流或 bash 命令仍会生效吗？

**A：**会。`ctx` 被传给 SDK 的 `NewStreaming` 和 `exec.CommandContext`；前者感知取消后终止 HTTP 流，后者终止 shell 子进程。`RunStreaming` 随后返回错误，未提交本轮临时 `messages`。

#### Q5：当前入口只注册哪个工具？对读取文件任务有什么影响？

**A：**只注册了 `BashTool`。模型必须通过 `bash` 调用 `cat README.md` 等命令读取文件，无法直接调用独立的 `read` 工具。
在 Go 中，可把历史保存成切片：
```go
messages := []RequestMessage{
    {Role: "user", Content: "我叫小明"},
    {Role: "assistant", Content: "你好，小明。"},
}

messages = append(messages,
    RequestMessage{Role: "user", Content: "我叫什么？"},
)
```
调用完成后，再把模型最终回答追加进去：
```go
messages = append(messages,
    RequestMessage{Role: "assistant", Content: fullAnswer},
)
```
流式场景尤其要注意：不能把每一个 delta 都单独追加成一条 assistant 消息。应先累积为完整 fullAnswer，等收到 [DONE] 后，再作为一条完整的 assistant 消息存入历史。

这也带来一个必然问题：聊天越久，messages 越长，Token 更多、成本和延迟更高，最终还会超过模型上下文窗口。这正是后续章节会引入“上下文压缩、摘要和长期记忆”的原因。

## ch04：让 Agent 接入 MCP 生态

### 学习目标

理解 MCP 如何让 Agent 动态发现、声明和调用外部工具，并掌握 MCP 与本地工具的共存方式。

### 核心内容

1. **MCP 的角色**：MCP Server 提供工具；MCP Client 连接 Server、发现工具和转发调用；Agent/Host 将这些工具声明给模型。

2. **工具生命周期**：

   ```text
   mcp-server.json → McpClient → 连接 Server → ListTools
   → McpTool 适配 → Agent 声明给模型 → tools/call → 结果回到模型
   ```

3. **命名空间避免冲突**：原始工具 `read_file` 在模型侧会变成 `babyagent_mcp__filesystem__read_file`。其中 `filesystem` 是 Server 名；MCP Server 实际仍接收原始名 `read_file`。

4. **本地与 MCP 工具共存**：Agent 构建请求时合并两类工具；执行时先查 native tool，再查 MCP Client 的工具。原有 Tool Calling 循环不需要改变。

5. **降级与安全**：MCP Server 刷新失败时跳过该 Server 并继续启动。MCP 只标准化发现和调用；路径权限、危险操作确认、沙箱、身份控制和 Prompt Injection 防护仍需由 Host、Server 与运行环境负责。

### 代码定位

- `mcp-server.json`：默认文件系统 MCP Server 配置。
- `shared/mcp.go`：配置加载及 `${workspaceFolder}` 替换。
- `ch04/main/main.go`：创建 MCP Client 并调用 `RefreshTools()`。
- `ch04/mcp.go`：连接、`ListTools`、`CallTool`、`McpTool` 适配器。
- `ch04/agent.go`：合并、路由本地和 MCP 工具。

### 关键代码与解释

#### 1. 连接并发现工具（`ch04/main/main.go`）

```go
mcpClient := ch04.NewMcpToolProvider(k, v)
if err := mcpClient.RefreshTools(ctx); err != nil {
	log.Printf("Failed to refresh tools for MCP server %s: %v", k, err)
	continue
}
mcpClients = append(mcpClients, mcpClient)
```

`RefreshTools()` 会连接 MCP Server、调用 `ListTools()` 并缓存可用工具。失败时仅跳过该 Server，不阻止本地工具或其他 Server 启动。

#### 2. 将 MCP 工具适配为项目工具（`ch04/mcp.go`）

```go
mcpToolResult, err := e.session.ListTools(ctx, &mcp.ListToolsParams{})
for _, mcpTool := range mcpToolResult.Tools {
	e.tools = append(e.tools, &McpTool{
		client: e, toolName: mcpTool.Name, mcpTool: mcpTool,
	})
}
```

每个外部 `mcp.Tool` 被包装为实现项目 `tool.Tool` 接口的 `McpTool`，使 Agent 可像处理本地工具一样处理 MCP 工具。

#### 3. 模型名与 MCP 原始名的映射（`ch04/mcp.go`）

```go
func (t *McpTool) ToolName() string {
	return fmt.Sprintf("babyagent_mcp__%s__%s", t.client.Name(), t.toolName)
}

func (t *McpTool) Execute(ctx context.Context, argumentsInJSON string) (string, error) {
	return t.client.callTool(ctx, t.toolName, argumentsInJSON)
}
```

`ToolName()` 为模型生成无冲突的命名空间名；`Execute()` 却传入原始 `t.toolName`，使 Server 收到其协议定义的名称。

#### 4. 合并工具并路由调用（`ch04/agent.go`）

```go
for _, t := range a.nativeTools {
	tools = append(tools, t.Info())
}
for _, mcpClient := range a.mcpClients {
	for _, t := range mcpClient.GetTools() {
		tools = append(tools, t.Info())
	}
}
```

这段将两类工具统一声明给模型。执行时 `execute()` 先在 `nativeTools` 查找，未命中再遍历 MCP Client 的工具。

#### 5. 调用 MCP 工具并取回文本结果（`ch04/mcp.go`）

```go
mcpResult, err := e.session.CallTool(ctx, &mcp.CallToolParams{
	Name:      toolName,
	Arguments: json.RawMessage(argumentsInJSON),
})
for _, content := range mcpResult.Content {
	if textContent, ok := content.(*mcp.TextContent); ok {
		builder.WriteString(textContent.Text)
	}
}
```

Client 发出 MCP `tools/call`，并将返回的文本内容块拼接为字符串。Agent 会把该字符串作为 `tool` 消息发回模型；本章尚未处理图片、资源链接等非文本块。

### Question & Answer

#### Q1：为什么模型侧不用原始名 `read_file`，而使用命名空间名？

**A：**不同 MCP Server 可能有同名工具。`babyagent_mcp__{serverName}__{toolName}` 带有 Server 身份，避免冲突并使模型能准确路由调用。

#### Q2：模型调用命名空间工具后，Server 实际收到什么名称？

**A：**收到原始工具名，例如 `read_file`；命名空间只用于 Agent 与模型之间的路由。

#### Q3：为什么要在创建 Agent 前调用 `RefreshTools()`？

**A：**它先连接 Server、获取 `ListTools` 并创建 `McpTool`。没有这一步，MCP Client 的工具列表为空，Agent 无法向模型声明任何 MCP 工具。

#### Q4：MCP Server 连接或 `ListTools` 失败时怎么处理？

**A：**记录错误、跳过该 Server，并继续使用其他可用 MCP Client 和本地 bash 工具。

#### Q5：调用 `babyagent_mcp__filesystem__read_file` 的完整链路是什么？

**A：**Agent 收到 tool call 后先查本地工具，未命中则定位到 filesystem Client 的 `McpTool`；其 `Execute()` 将原始 `read_file` 和 JSON 参数交给 `McpClient.callTool()`；Client 通过 stdio 发送 `tools/call`；Server 返回内容块；Client 拼接文本；Agent 将文本作为 `tool` 消息追加，再请求模型继续推理或生成最终回答。

#### Q6：为什么 MCP 不自动解决安全问题？

**A：**MCP 只定义发现和调用协议，不判断调用是否安全。访问范围、权限、确认流程、沙箱及 Prompt Injection 防护仍要自行实现。

#### Q7：MCP 靠什么连接 Server？

**A：**本章支持两种传输方式：

- **stdio**：启动本地 Server 子进程，并通过其标准输入/输出传递 JSON-RPC 消息。
- **Streamable HTTP**：连接远程 MCP Server 的 HTTP 端点。

当前 `mcp-server.json` 配置了 `command: "npx"`，因此使用 stdio。连接建立后会依次完成初始化、`tools/list` 工具发现和 `tools/call` 工具调用。

```go
// stdio
e.client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)

// HTTP
e.client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: e.serverConfig.Url}, nil)
```

#### Q8：连接后获得的 `session` 是什么？

**A：**`session` 的类型是 `*mcp.ClientSession`，代表 MCP Client 与某个 MCP Server 建立并初始化后的可复用通信会话。它保存该连接的会话状态，并用于在同一连接上发送 MCP 请求：

```go
e.session.Ping(ctx, ...)
e.session.ListTools(ctx, ...)
e.session.CallTool(ctx, ...)
```

`McpClient` 负责建立和管理连接；`ClientSession` 是已经建立好、能实际收发 MCP 协议消息的会话。`connect()` 会优先 `Ping` 已有 session，正常时复用，失效时才重新连接。

## ch05：上下文工程（Context Engineering）

### 学习目标

理解 Agent 如何在有限上下文窗口内维护可用的对话和工具结果，并掌握截断、卸载、摘要三种上下文策略及其协作关系。

### 核心内容

1. **上下文是有限预算**

   Agent 每轮都会携带用户消息、assistant 消息、tool calls 与 tool results。上下文过长会带来窗口超限、成本上升和注意力分散，因此必须主动管理。

2. **Context Engine 接管历史管理**

   ch04 中 Agent 直接维护 `messages`；ch05 改由 `Context Engine` 维护消息、每条消息的 token 估算、总 token、窗口大小和策略列表。

3. **草稿提交保护会话历史**

   ```text
   StartTurn → 在 TurnDraft 中累积本轮 user/assistant/tool 消息
             → 成功时 CommitTurn → 运行上下文策略
             → 失败或取消时不提交
   ```

   这样流中断、工具失败或取消都不会污染正式上下文。

4. **三种策略**

   | 策略 | 做法 | 主要取舍 |
   |---|---|---|
   | Offload | 长工具结果存到外部存储，正文改为预览和 key | 保留完整内容的恢复入口 |
   | Summarize | 用后台 LLM 将旧历史压缩为摘要 | 保留关键语义但可能遗漏细节，增加成本 |
   | Truncate | 删除早期消息 | 最便宜但不可逆，可能丢失重要信息 |

   默认顺序是 `offload → summarize → truncate`：优先可恢复压缩，其次语义压缩，最后才不可逆删除。

5. **Policy 模式**

   每个策略都实现 `Name()`、`ShouldApply()` 和 `Apply()`。Engine 按注册顺序检查阈值、执行策略、写入策略返回的新消息列表并重算 token；因此前一策略降低使用率后，后一策略本轮可能不再触发。

6. **有效上下文能力取决于上下文工程**

   模型的上下文窗口是硬上限；Agent 的有效上下文能力取决于策略种类、顺序、阈值、保留消息数、摘要质量、外部存储恢复能力和系统提示。策略不会扩大物理窗口，但决定有限空间内保留什么、压缩什么、如何恢复。

### 代码定位

- `ch05/agent.go`：使用 `TurnDraft`，完成后调用 `CommitTurn`。
- `ch05/context/engine.go`：上下文 Engine、token 统计、策略调度和 system prompt 变量替换。
- `ch05/context/policy.go`：Policy 接口与策略结果模型。
- `ch05/context/policy_offload.go`：长工具结果卸载。
- `ch05/context/policy_summary.go`、`summary.go`：增量摘要与后台摘要模型。
- `ch05/context/policy_truncate.go`：保留最新历史的截断策略。
- `ch05/tool/load_storage.go`、`storage/storage.go`：卸载内容的恢复工具与内存存储。
- `ch05/main/main.go`：默认策略顺序和阈值装配。

### 关键代码与解释

#### 1. 本轮草稿与成功提交（`ch05/agent.go`）

```go
draft := a.contextEngine.StartTurn(openai.UserMessage(query))
defer a.contextEngine.AbortTurn(draft)

messages := a.contextEngine.BuildRequestMessages()
messages = append(messages, draft.NewMessages...)
// 流式模型调用与工具循环，将新消息追加到 draft.NewMessages

err := a.contextEngine.CommitTurn(ctx, draft, ctxengine.Usage{PromptTokens: int(usage.TotalTokens)})
```

`draft` 是本轮临时消息链。`CommitTurn` 只在成功完成时执行；`AbortTurn` 当前是 no-op，因为未提交的 draft 仅存在于内存局部变量中。

#### 2. Engine 提交消息并运行策略（`ch05/context/engine.go`）

```go
for i := range draft.NewMessages {
	msg := draft.NewMessages[i]
	c.messages = append(c.messages, messageWrap{Message: msg, Tokens: CountTokens(msg)})
}
c.recountTokens()
return c.applyPolicies(ctx)
```

正式历史中的每条消息都有 token 估算。提交后才检查和执行策略，保证策略操作的是完整成功轮次。

#### 3. Policy 接口与顺序执行（`ch05/context/policy.go`、`engine.go`）

```go
type Policy interface {
	Name() string
	ShouldApply(ctx context.Context, engine *Engine) bool
	Apply(ctx context.Context, engine *Engine) (PolicyResult, error)
}
```

```go
for _, policy := range c.policies {
	if !policy.ShouldApply(ctx, c) {
		continue
	}
	result, err := policy.Apply(ctx, c)
	c.messages = result.Messages
	c.recountTokens()
}
```

策略以返回值替换状态，便于测试和组合。每次执行后重新计数，下一策略会看到更新后的使用率。

#### 4. Offload 只替换长工具结果（`ch05/context/policy_offload.go`）

```go
if shared.GetRoleName(messages[i].Message) != "tool" {
	continue
}
if len(*contentStr) <= p.PreviewCharLimit {
	continue
}

key := p.makeStorageKey(i)
if err := p.Storage.Store(ctx, key, *contentStr); err != nil {
	continue
}
```

策略仅处理超出预览长度的 tool 消息：先存储原文，再把上下文正文改为“预览 + `load_storage(key)` 提示”。模型可按需调用恢复工具读取全文。

#### 5. 增量摘要与最后截断（`ch05/context/policy_summary.go`、`policy_truncate.go`）

```text
runningSummary + 第 1 批旧消息 → 新摘要
新摘要 + 第 2 批旧消息 → 更新后的摘要
```

摘要策略保留最近消息，并以一条摘要替换旧历史；截断策略则从 user 消息边界保留较新历史、直接删除更早消息。两者分别是语义压缩和最后兜底。

### Question & Answer

#### Q1：为什么必须在一轮成功后才 `CommitTurn`？

**A：**避免失败、取消或中断的对话污染下一次调用使用的正式上下文。

#### Q2：Offload 后模型如何重新获得完整工具结果？

**A：**上下文预览中包含 storage key；模型可调用 `load_storage(key="/offload/...")` 恢复原始全文。

#### Q3：为什么 `truncate` 适合作为最后一道策略？

**A：**它直接、不可逆地删除早期信息，可能造成目标、约束或重要事实遗失；应先尝试可恢复的 Offload 和尽量保留语义的 Summary。

#### Q4：使用率 65%，Offload 阈值 40%，Offload 后降至 50%，Summary 阈值 60%；Summary 本轮会执行吗？

**A：**不会。Offload 后 Engine 重算 token，Summary 的 `ShouldApply()` 看到 50%，不满足严格的大于条件 `50% > 60%`。

#### Q5：`key := p.makeStorageKey(i)` 是否存在问题？

**A：**存在碰撞风险。当前 key 由精确到秒的时间戳和当前索引 `i` 组成；若两次 Offload 在同一秒内处理相同索引，后一次 `Store` 可能覆盖前一次内容。`i` 只能区分同一次执行中的消息，不能区分不同批次。应使用 UUID，或至少使用纳秒级时间戳；生产中还可加入 user/session 命名空间。

```go
func (p *OffloadPolicy) makeStorageKey() string {
	return "/offload/" + uuid.NewString()
}
```

#### Q6：模型的上下文能力是否取决于策略内容？

**A：**模型的物理上下文窗口由模型决定；Agent 的有效上下文能力很大程度由上下文策略决定。策略不会扩大硬上限，但通过信息取舍、压缩和恢复，决定 Agent 在有限窗口内能持续保留多少有用信息。

## ch06：记忆机制（Memory System）

### 学习目标

理解短期上下文与跨会话长期记忆的差异，掌握两层记忆的持久化、LLM 增量更新与 System Prompt 注入过程。

### 核心内容

1. **上下文与记忆的职责不同**

   | 类型 | 内容 | 生命周期 |
   |---|---|---|
   | Context | 当前会话消息、工具结果 | 会话内，受上下文窗口限制 |
   | Memory | 用户偏好、项目知识 | 跨会话持久化 |

   Context 解决有限窗口内的即时任务连续性；Memory 解决进程重启或切换会话后的知识延续。

2. **两层长期记忆**

   - **Global Memory**：跨项目的用户偏好、背景和长期模式；存储在 `~/.babyagent/memory/MEMORY.md`。
   - **Workspace Memory**：当前项目的结构、技术栈、命令和约定；存储在 `{workspace}/.babyagent/memory/MEMORY.md`。

   两层使用相同内部 key `/memory/MEMORY.md`，但使用不同 `baseDir`，最终写入不同位置。

3. **记忆更新与注入时序**

   ```text
   本轮开始：BuildSystemPrompt 注入旧记忆
   → 模型与工具完成本轮任务
   → CommitTurn 提交短期上下文并执行策略
   → Memory.Update(本轮新增消息)
   → Global / Workspace 持久化
   → 下一轮 system prompt 注入新记忆
   ```

   因此刚更新的记忆从下一轮开始生效；这不是 bug。

4. **LLM 驱动的增量更新**

   `MemoryUpdater` 读取旧记忆与本轮 `draft.NewMessages`，输出更新后的 Global 和 Workspace Markdown。只传入本轮新增消息可避免重复处理全部历史，降低成本和记忆漂移风险。

5. **记忆更新的限制**

   当前实现每轮都会额外调用 LLM；全部记忆都会注入 system prompt；XML 标签缺失可能导致对应记忆被提取为空；全局存储成功而工作区存储失败时可能造成两层不一致。生产系统还需处理隐私、权限、检索、衰减、冲突与人工编辑。

### 代码定位

- `ch06/memory/memory.go`：Memory 接口、两层记忆、加载和持久化。
- `ch06/memory/update.go`：MemoryUpdater 接口、LLM 更新 Prompt、XML 标签提取。
- `ch06/context/engine.go`：在 CommitTurn 后更新记忆，并在 system prompt 注入记忆。
- `ch06/storage/filesystem.go`：文件系统存储实现。
- `ch06/main/main.go`：创建 home/workspace 两个存储与多层记忆。
- `ch06/agent.go`：注册记忆事件，供 TUI 显示更新状态。
- `ch06/prompt.go`：包含 `{memory}` 占位符的系统提示。

### 关键代码与解释

#### 1. MultiLevelMemory 的加载与更新（`ch06/memory/memory.go`）

```go
content.WorkspaceMemory, _ = m.workspaceStorage.Load(ctx, m.workspaceKey)
content.GlobalMemory, _ = m.globalStorage.Load(ctx, m.globalKey)
```

```go
newMemory, err := m.updater.Update(ctx, m.content, newMessages)
if err := m.globalStorage.Store(ctx, m.globalKey, newMemory.GlobalMemory); err != nil {
	return err
}
if err := m.workspaceStorage.Store(ctx, m.workspaceKey, newMemory.WorkspaceMemory); err != nil {
	return err
}
m.content = newMemory
```

初始化时从两个持久化存储读取记忆；更新时先让 Updater 生成新内容，再写入 Global/Workspace 存储，最后更新进程内缓存。

#### 2. 将记忆写入 System Prompt（`ch06/context/engine.go`）

```go
if c.memory != nil {
	replaceMap["{memory}"] = c.memory.String()
} else {
	replaceMap["{memory}"] = ""
}
```

每次构建请求消息时，`{memory}` 都会被替换成当前两层记忆的 Markdown。因此模型能“记住”信息的本质是每次请求都重新读取该文本。

#### 3. 成功提交后更新记忆（`ch06/context/engine.go`）

```go
if err := c.applyPolicies(ctx); err != nil {
	return err
}

err := c.memory.Update(ctx, draft.NewMessages)
```

先更新短期上下文并执行 ch05 策略，后将本轮新增 user/assistant/tool 消息交给记忆系统。失败或取消的 turn 不会执行到这里，因此不会写入长期记忆。

#### 4. LLM 提炼两层记忆（`ch06/memory/update.go`）

```go
prompt = strings.ReplaceAll(prompt, "{current_memory}", oldMemory.String())
prompt = strings.ReplaceAll(prompt, "{new_messages}", b.String())

resp, err := u.client.Chat.Completions.New(ctx, request)
newMemory.GlobalMemory = extractXMLTag(respContent, "global")
newMemory.WorkspaceMemory = extractXMLTag(respContent, "workspace")
```

LLM 将旧记忆与本轮消息合并，按 Prompt 要求输出 `<global>` 和 `<workspace>` 标签中的 Markdown；程序提取后得到更新结果。

#### 5. 两个文件系统存储（`ch06/main/main.go`）

```go
homeStorage := storage.NewFileSystemStorage(
	filepath.Join(shared.GetHomeDir(), ".babyagent"),
)
workspaceStorage := storage.NewFileSystemStorage(
	filepath.Join(shared.GetWorkspaceDir(), ".babyagent"),
)
```

相同 key 与不同 base directory 相组合，让用户全局记忆和当前工作区记忆隔离持久化。

### Question & Answer

#### Q1：用户说“我习惯使用 vim”，应写入哪一层？

**A：**Global Memory。这是通常跨项目仍成立的用户偏好。

#### Q2：当前项目使用 Gin，测试命令是 `go test ./...`，应写入哪一层？

**A：**Workspace Memory。这是当前项目的技术栈与操作约定，切换项目后不一定仍适用。

#### Q3：为什么 `Memory.Update()` 只传入 `draft.NewMessages`，而不是整个历史？

**A：**`draft.NewMessages` 只含本轮新增的 user、assistant 和 tool 消息；旧信息已存在于 `oldMemory`。若每轮处理完整历史，会重复消耗 token、增加延迟，并可能反复改写旧结论造成记忆漂移。

#### Q4：程序重启后还能记住信息，依靠哪两个步骤？

**A：**更新时由 LLM 提炼信息并通过 `FileSystemStorage` 写入磁盘；重启时 `NewMultiLevelMemory()` 再从 Global/Workspace 的文件读取记忆。

#### Q5：本轮模型响应没有使用刚更新的记忆，是 bug 吗？

**A：**不是。本轮请求开始前已经构建并发送了携带旧记忆的 system prompt；模型响应完成后才调用 `Memory.Update()`。新记忆在下一轮 `BuildSystemPrompt()` 时注入。本轮用户消息本身已提供刚出现的事实，无需为同一轮额外重发模型请求。

## ch07：Agentic RAG（检索增强生成）

### 学习目标

理解 RAG 的离线建索引与在线检索流程，掌握 Chunking、Embedding、pgvector 向量搜索、Rerank 的分工，以及语义搜索如何作为 Agent 工具使用。

### 核心内容

1. **Agentic RAG**

   检索被封装为 `semantic_search` 工具，由 Agent 自主决定是否检索、使用什么查询语句、检索几条以及是否继续检索。

   ```text
   用户问题 → Agent 判断是否需要项目事实
            → semantic_search(query, top_k)
            → 检索证据作为 tool result
            → 模型基于证据回答
   ```

2. **离线建索引流程**

   ```text
   FileWalker 遍历文本文件
   → Chunker 切成带文件名/行号的 Chunk
   → Embedding API：每个文本块 → 向量
   → pgvector：持久化文档块和向量
   ```

   FileWalker 会跳过 `.git`、`node_modules` 等目录，并按支持的文本扩展名过滤文件。

3. **在线查询流程与组件职责**

   ```text
   用户查询文本
   → Embedding API：查询文本 → 查询向量
   → pgvector：查询向量 → 相近的已索引文档块
   → Rerank API：query + 候选块 → 更精确排序
   → SemanticSearchTool：带路径、行号和内容的文本结果
   ```

   Embedding API 不读取 pgvector 数据库；它只把传入文本编码为向量。pgvector 才保存预先索引的文档向量，并按查询向量执行近邻搜索。

4. **Chunking 的必要性**

   整份长文件 Embedding 可能超出模型输入限制，也会稀释不同函数/段落的语义。小块能提高定位精度、控制送入 LLM 的 token，并支持文件局部更新。本章提供行切分和段落切分；块大小需要在上下文完整性与检索精度之间评测权衡。

5. **两阶段检索**

   向量检索适合快速从大量 chunks 中粗召回；Rerank 更精确但成本较高，因此应只重排少量候选：

   ```text
   全库 chunks → 向量召回 Top-N → Rerank → 最终 Top-K
   ```

6. **增量索引**

   Indexer 将文件修改时间与数据库最早索引时间比较：未修改则跳过，已修改则删除旧 chunks 后重新切分、向量化和插入。生产中可用内容 hash 和事务提高正确性，避免删除旧索引后新索引失败造成数据缺失。

7. **当前实现的注意点**

   - `top_k=5` 时，工具先召回 `5 * 2 = 10` 条；当前 Rerank 和格式化未截断，实际返回 10 条，而非最终 5 条。
   - 数据库列 `vector(1536)` 被写死，而 Embedding 服务/Store 配置可为其他维度；Embedding 输出维度、Store 配置和数据库 schema 必须一致。
   - `IndexConcurrent` 限制文件并发，但每个文件的 `embedChunks` 仍会为所有块启动 goroutine；大仓库应增加全局 Embedding 并发限制。
   - 当前仓库的 ch07 是 RAG 组件集合，尚无单独的 Agent/TUI `main` 将其实际注册到前面章节的 Agent。

### 代码定位

- `ch07/index/file_walker.go`：文件遍历与过滤。
- `ch07/index/indexer.go`：切分、向量化、写库、增量与并发索引。
- `ch07/rag/chunker.go`：行切分与段落切分。
- `ch07/rag/embedding.go`：Embedding HTTP API 客户端。
- `ch07/db/pgvector.go`：PostgreSQL/pgvector 建表、插入和相似度搜索。
- `ch07/rag/rerank.go`：Rerank HTTP API 客户端。
- `ch07/tool/semantic_search.go`：将 RAG 流程封装为 Tool Calling 工具。

### 关键代码与解释

#### 1. 建索引的主体流程（`ch07/index/indexer.go`）

```go
chunks := idx.chunker.Chunk(relPath, string(content))
vectorPoints, err := idx.embedChunks(ctx, chunks)
if err != nil {
	return nil, err
}
if err := idx.vectorStore.InsertBatch(ctx, vectorPoints); err != nil {
	return nil, err
}
```

文件内容先被切成带相对路径和行号的 chunks；每个 chunk 生成向量后，连同原文和元数据一起批量写入 VectorStore。

#### 2. Embedding API 只负责文本到向量（`ch07/rag/embedding.go`）

```go
req := embeddingRequest{Model: s.config.Model, Input: text}
r := s.client.R().SetContext(ctx).SetBody(req).SetResult(&resp)
_, err := r.Post("/embeddings")
return Vector(resp.Data[0].Embedding), nil
```

此代码发送文本给外部 Embedding 服务，并接收浮点向量；该服务不读取本地文件，也不访问 pgvector。

#### 3. 查询向量由 pgvector 匹配文档向量（`ch07/db/pgvector.go`）

```sql
SELECT id, content, document_id, start_pos, end_pos,
       1 - (embedding <=> ?) as score
FROM document_chunks
ORDER BY embedding <=> ?
LIMIT ?
```

`<=>` 是 pgvector 的余弦距离运算符。数据库中的 `embedding` 是建索引时存入的文档块向量；查询参数是刚由 Embedding API 生成的查询向量。

#### 4. 先粗召回再精排（`ch07/tool/semantic_search.go`）

```go
queryVector, err := s.embedService.Embed(ctx, params.Query)
vectorResults, err := s.vectorStore.Search(ctx, queryVector, params.TopK*2)
rerankedChunks, err = s.rerankService.Rerank(ctx, params.Query, candidates)
```

查询先被编码为向量，随后从数据库快速召回 `2 × TopK` 候选；Rerank API 再根据 query 和候选正文精细排序。当前代码没有在重排后切回 `TopK`，这是可改进点。

#### 5. 结果保留证据元数据（`ch07/tool/semantic_search.go`）

```go
result += fmt.Sprintf("文档: %s\n", chunk.Meta.DocumentID)
result += fmt.Sprintf("位置: 行 %d-%d\n", chunk.Meta.StartPos, chunk.Meta.EndPos)
result += fmt.Sprintf("内容:\n%s\n\n", chunk.Content)
```

检索结果不仅携带正文，也携带文件路径和行号。模型可据此追溯、验证结论，并在需要时继续读取或修改精确位置。

### Question & Answer

#### Q1：为什么索引和查询必须使用同一 Embedding 模型或兼容向量空间？

**A：**向量每一维的语义由模型定义。若文档和查询来自不兼容模型，即使文本语义相近，向量距离也没有可靠可比性；它们还必须具有相同维度。

#### Q2：为什么检索结果必须保留 `DocumentID` 和行号？

**A：**它们提供证据来源：模型和用户能追溯结论来自哪个文件、哪段代码，后续可精确读取、修改和验证，避免把无来源片段当作绝对事实。

#### Q3：为什么 Rerank 前必须先向量召回？

**A：**Rerank 更精确但需要处理 query 与每个候选的完整文本，无法经济地处理全库。向量检索先快速将海量 chunks 缩小到候选集，再精排少量候选，兼顾速度和精度。

#### Q4：当前 `top_k=5` 实际会返回多少结果？

**A：**10 条。代码先以 `TopK*2` 召回 10 条，Rerank 的 `TopN` 也是候选总数，格式化阶段又遍历所有重排结果，未在重排后截取前 5 条。

#### Q5：ch07 是否自己实现了向量化和重排序模型算法？

**A：**没有。它实现的是 HTTP 客户端、接口抽象、索引/查询编排和结果处理。Embedding 由外部 `/embeddings` API 完成，Rerank 由外部 `/rerank` API 完成，向量近邻搜索由 PostgreSQL + pgvector 完成。

#### Q6：Embedding API 是否会到 pgvector 中获取数据？

**A：**不会。查询时先由 Embedding API 将用户文本转换为查询向量；随后本地 Go 代码将该向量传给 pgvector，pgvector 在 `document_chunks` 表中比较它与已存文档向量的距离并返回近邻。索引时则反向进行：文档块 → Embedding API → 向量写入 pgvector。

## ch08：沙盒与安全防御（Guardrails）

### 学习目标

理解 Agent 工具执行的两层安全防护：Docker 沙箱隔离与人工确认；掌握工具工厂、确认 channel、取消后的状态处理以及实现中的安全边界。

### 核心内容

1. **两层防御**

   ```text
   Docker 沙箱：限制命令的运行环境
   + 人工确认：在工具执行前让用户作最终决定
   ```

   两者都不是绝对安全机制：Docker 仍需配置权限、网络和资源限制；人工确认也必须展示清晰、可信的工具参数。

2. **启动时选择 bash 实现**

   `CreateBashTool()` 使用 `docker ps` 判断 Docker 是否可用：可用时创建 `DockerBashTool`，否则创建宿主机 `BashTool`。

   这是启动时的**工具选择**，不是 Docker 命令失败后的自动重试。若已选 Docker Tool，但后续 `docker exec` 失败，当前实现直接返回错误，不会回退到宿主机 bash。

3. **Docker 沙箱的实际边界**

   容器用 `alpine:3.19` 持续运行，宿主机工作区以 `:rw` 挂载到容器 `/workspace`，并通过 `docker exec ... sh -c` 执行命令。

   因此容器中对 `/workspace` 的读写会真实影响宿主机项目；默认实现也未禁用网络、未设置 CPU/内存限制、只读根文件系统或非 root 用户。Docker 只隔离了部分宿主机环境。

4. **确认流程**

   Agent 判断工具是否配置为需要确认且未被当前会话 Always Allow：若需要，则通过 `MessageTypeToolConfirm` 向 TUI 发确认请求，在 `confirmCh` 等待 Allow / Reject / Always Allow。

   - Allow：执行本次调用。
   - Reject：本次不执行，将拒绝结果作为 tool message 反馈模型。
   - Always Allow：执行本次，并记录该工具名在当前 Agent 后续调用中免确认。

5. **拒绝不是立刻终止 Loop**

   当前代码将 `user rejected tool call` 写回 messages 后继续 Agent Loop。模型可根据该观察结果改用安全工具、请求澄清或解释无法完成，而不是直接失去调整机会。

6. **取消后的提交策略**

   特定 ESC 取消路径会提交已形成的 draft 消息，但使用 `skipPoliciesAndMemory=true` 跳过上下文压缩与长期记忆更新，避免取消后继续消耗资源。若取消发生在模型流运行期间，`stream.Err()` 可能直接返回，未必走到提交路径，因此“ESC 一定保存当前状态”不是所有时机都成立。

7. **Always Allow 的安全粒度与实现差异**

   当前授权按工具名（例如 `bash`）而非按具体命令授权：允许一次 `bash("ls")` 后，后续 `bash("rm -rf ...")` 也不再确认，粒度过粗。

   README 声称 `/clear` 会清除 Always Allow；但 `ResetSession()` 当前只重置 Context Engine，未清空 `alwaysAllowTools`，所以 `/clear` 后免确认授权仍存在。这是文档与代码不一致的安全问题。

### 代码定位

- `ch08/tool/factory.go`：Docker 可用性检测与 bash 工具选择。
- `ch08/tool/docker_bash.go`：容器命名、懒启动、工作区挂载和 `docker exec`。
- `ch08/agent.go`：工具查找、确认决策、Always Allow、取消与提交。
- `ch08/vo.go`：确认动作和确认事件 VO。
- `ch08/tui/tui.go`：确认弹框状态、按键选择和 `confirmCh` 传递。
- `ch08/context/engine.go`：`skipPoliciesAndMemory` 的提交行为。
- `ch08/main/main.go`：bash 确认配置与工具装配。

### 关键代码与解释

#### 1. 工具工厂的启动时降级（`ch08/tool/factory.go`）

```go
if !checkDockerAvailable() {
	return NewBashTool()
}
if workspaceDir == "" {
	return NewBashTool()
}
return NewDockerBashTool("", workspaceDir)
```

Docker daemon 可用且存在工作区时才创建 Docker Tool；否则程序依然可运行，但 bash 命令将直接在宿主机执行，安全性下降。

#### 2. 容器创建与可写工作区挂载（`ch08/tool/docker_bash.go`）

```go
createCmd := exec.CommandContext(ctx, "docker", "run", "-d",
	"--name", t.containerName,
	"--restart", "unless-stopped",
	"-v", t.workspaceDir+":/workspace:rw",
	"-w", "/workspace",
	t.image, "sleep", "infinity")
```

工作区以读写方式映射进入容器，因此命令会在隔离进程环境中运行，但对 `/workspace` 的修改仍会写回宿主机项目。

#### 3. 懒初始化与命令执行（`ch08/tool/docker_bash.go`）

```go
t.once.Do(func() {
	t.startErr = t.ensureSandboxContainer(ctx)
})

cmd := exec.CommandContext(ctx, "docker", "exec",
	t.containerName, "sh", "-c", p.Command)
```

`sync.Once` 避免多次/并发创建同一容器；后续调用通过 `docker exec` 复用。首次创建失败会缓存 `startErr`，当前 Tool 实例不自动重试。

#### 4. 确认请求与三种选择（`ch08/agent.go`）

```go
needConfirm := a.confirmConfig.RequireConfirmTools[toolName] && !a.alwaysAllowTools[toolName]
if needConfirm {
	viewCh <- MessageVO{Type: MessageTypeToolConfirm, ToolConfirmationRequest: &confirmReq}
	select {
	case <-ctx.Done():
		return nil
	case action := <-confirmCh:
		// Allow / Reject / AlwaysAllow
	}
}
```

Agent 通过 channel 暂停等待 TUI。确认配置和 Always Allow 均按工具名决定，不分析具体命令内容。

#### 5. 拒绝反馈与始终允许（`ch08/agent.go`）

```go
case ConfirmReject:
	toolMsg := openai.ToolMessage("user rejected tool call", toolCall.ID)
	messages = append(messages, toolMsg)
	draft.NewMessages = append(draft.NewMessages, toolMsg)
	continue
case ConfirmAlwaysAllow:
	a.alwaysAllowTools[toolName] = true
```

Reject 会将拒绝作为观察结果交给模型，而非立即退出整个 Loop；Always Allow 将工具名写入会话内 map，造成该工具后续调用免确认。

#### 6. 取消后跳过策略和记忆（`ch08/context/engine.go`）

```go
if skipPoliciesAndMemory {
	return nil
}

if err := c.applyPolicies(ctx); err != nil {
	return err
}
err := c.memory.Update(ctx, draft.NewMessages)
```

取消路径可先保存 draft 消息，再直接返回，不执行可能耗时的上下文压缩和长期记忆更新。

### Question & Answer

#### Q1：容器中执行 `rm -rf /workspace/tmp` 会影响宿主机工作区吗？

**A：**会。工作区以 `:rw` 挂载到容器 `/workspace`，该目录内的删改会同步影响宿主机对应项目文件。

#### Q2：Docker 不可用时降级到普通 bash 有什么利弊？

**A：**好处是程序没有 Docker 仍能继续运行；风险是命令直接在宿主机执行。当前降级发生在创建 Tool 时，而不是 Docker 命令失败后再次使用宿主机 bash 执行。

#### Q3：选择一次 Always Allow 后，后续 `bash("rm -rf ...")` 会再次确认吗？

**A：**不会。`alwaysAllowTools` 按 `bash` 工具名记录授权，不区分参数或命令危险程度。这是过粗的授权粒度，应改为命令分类/前缀或风险级别策略。

#### Q4：为什么用户拒绝工具调用后仍要把拒绝信息作为 tool message 发回模型？

**A：**拒绝是模型进行下一步决策的真实反馈。模型可调整方案、换用安全工具、请求澄清或说明无法完成，避免盲目重复调用。

#### Q5：`ResetSession()` 与 README 的 Always Allow 描述有什么安全问题？

**A：**README 说 `/clear` 会清除 Always Allow，但实际 `ResetSession()` 仅调用 `a.contextEngine.Reset()`，未清空 `alwaysAllowTools`。用户以为授权已结束，危险工具却仍可能免确认，形成安全语义不一致。

## ch09：Agent 技能插件（Skills）

### 学习目标

理解 Skill 作为“任务方法论提示包”的定位，掌握 Markdown + YAML front matter 格式、渐进式加载、`load_skill` 工具和 Skill 的安全边界。

### 核心内容

1. **Skill 不等于 Tool**

   ```text
   Tool：决定 Agent 能做什么（读文件、执行命令等）
   Skill：决定 Agent 面对某类任务应怎样做（步骤、清单、输出格式）
   ```

   Skill 是描述性提示，不会增加执行权限。没有 write/edit 等工具时，“代码修改 Skill”也不能真正修改文件。

2. **渐进式加载**

   ```text
   初始化/每轮系统提示：只注入 Skill 的名称和描述
   → 任务匹配时：模型调用 load_skill(name)
   → 返回该 Skill 的完整正文及资源清单
   ```

   不将全部 `SKILL.md` 正文注入 system prompt，可节省 token、避免占用上下文窗口并减少无关信息噪声。

3. **Skill 目录与格式**

   ```text
   .babyagent/skills/<skill-id>/
   ├── SKILL.md
   ├── scripts/       # 可选
   └── references/    # 可选
   ```

   `SKILL.md` 以 YAML front matter 提供 `name` 与 `description`，正文是详细指令。目录名是 Skill ID，也是 `load_skill` 的参数。

4. **资源不会自动执行或读取**

   `load_skill` 返回 Main Instruction，以及 scripts/references 的相对路径；它不自动运行脚本或读取参考文档。模型仍需按需使用 `read` 或 `bash`，后者继续受到 ch08 的人工确认策略约束。

5. **发现时机**

   `SkillManager.LoadAll()` 在创建 Context Engine 时只执行一次。运行中新增 Skill 不会自动出现在当前 system prompt；重启或实现刷新机制后才会被元数据发现。模型若偶然猜中 ID，仍可能直接调用 `load_skill` 成功，但不会自动知道其存在。

6. **安全边界**

   `load_skill` 的 name 直接参与路径拼接。除空值校验外，生产实现应限制 ID 格式，拒绝 `..`、`/`、`\` 与绝对路径，防范路径穿越。来自不可信来源的 Skill、脚本和参考文档也可能含 Prompt Injection，不能不经审查地加载或执行。

7. **当前工作区状态**

   本次学习时工作区仅有 `.babyagent/memory/MEMORY.md`，没有 `.babyagent/skills/`。因此当前 `FormatForPrompt()` 会注入 `No skills available.`；`load_skill` 工具已注册，但没有本地可发现的技能。

### 代码定位

- `ch09/skill/skill.go`：Skill 类型、技能目录扫描、仅元数据的 prompt 格式化。
- `ch09/skill/load.go`：YAML front matter 解析、完整正文和资源列表发现。
- `ch09/tool/load_skill.go`：语义化的 Skill 加载工具。
- `ch09/context/engine.go`：创建 SkillManager 并以 `{skills}` 注入系统提示。
- `ch09/prompt.go`：`{skills}` 占位符。
- `ch09/main/main.go`：注册 `read`、bash、`load_storage`、`load_skill` 工具。

### 关键代码与解释

#### 1. 扫描技能与格式化元数据（`ch09/skill/skill.go`）

```go
for _, entry := range entries {
	if !entry.IsDir() {
		continue
	}
	loadedSkill, err := LoadSkill(entry.Name())
	if err != nil {
		continue
	}
	m.skills = append(m.skills, loadedSkill)
}
```

```go
for _, loadedSkill := range m.skills {
	sb.WriteString(fmt.Sprintf("- **%s**: %s\n", loadedSkill.Name, loadedSkill.Description))
}
```

Manager 扫描技能子目录并解析其文件；向 system prompt 输出时只使用名称和描述，而非完整正文。

#### 2. 加载完整 Skill 与辅助资源（`ch09/skill/load.go`）

```go
parts := strings.SplitN(text, "---", 3)
if len(parts) < 3 {
	return Skill{}, errors.New("skill file must have YAML front matter enclosed in `---`")
}

scripts, err := listFiles(filepath.Join(skillDir, "scripts"), workspaceDir)
references, err := listFiles(filepath.Join(skillDir, "references"), workspaceDir)
```

代码解析 YAML front matter，正文作为 `MainInstruction`，并递归列出 scripts/references 下的相对文件路径；不读取辅助文件正文，也不执行脚本。

#### 3. 按需加载工具（`ch09/tool/load_skill.go`）

```go
loadedSkill, err := skill.LoadSkill(p.Name)
sb.WriteString(loadedSkill.MainInstruction)
for _, filePath := range loadedSkill.Scripts {
	sb.WriteString(fmt.Sprintf("- %s\n", filePath))
}
```

模型调用 `load_skill(name)` 后，获得完整指导和资源索引。它之后可自主调用 read 或 bash 使用资源；Skill 工具本质上是对约定路径的语义化读取封装。

#### 4. 注入 `{skills}` 到 System Prompt（`ch09/context/engine.go`）

```go
skillManager := skill.NewManager()
_ = skillManager.LoadAll()
```

```go
replaceMap["{skills}"] = c.skillManager.FormatForPrompt()
```

Manager 在 `NewContextEngine()` 时加载一次元数据，随后每轮构建 system prompt 时注入相同的技能目录；运行时新增 Skill 不会自动刷新。

### Question & Answer

#### Q1：为什么 system prompt 只注入名称和描述，而不注入全部 Skill 正文？

**A：**全部正文会导致系统提示过长、消耗 token、挤占上下文窗口，也会为不相关任务引入噪声。元数据提供导航，详细内容按需加载。

#### Q2：`load_skill` 返回脚本路径后，为什么不自动运行脚本？

**A：**Skill 仅提供指导和资源索引；是否运行应由模型结合任务判断。自动执行会失去任务与安全控制，实际运行仍需 bash 工具和确认机制。

#### Q3：运行后新建 Skill，当前 system prompt 会自动发现它吗？

**A：**不会。Manager 仅在 Context Engine 创建时调用一次 `LoadAll()`；重启或增加刷新机制后才会更新元数据。模型偶然猜中新 ID 时仍可能直接加载成功，但系统不会主动展示它。

#### Q4：Skill 能取代 Tool 吗？

**A：**不能。Skill 是规范性提示词，不提供执行能力。缺少相应 Tool 时，模型无法真正执行 Skill 中要求的读写、修改或命令操作。

#### Q5：为什么 `load_skill.name` 需要路径安全校验？

**A：**空名称会导致找不到文件，代码已有检查；更重要的是 name 会参与 `filepath.Join`。若接受 `../../...` 等路径，可能跳出预期 skills 目录并读取其他位置的 `SKILL.md` 或枚举资源。应限制为小写字母、数字和连字符，拒绝 `..`、路径分隔符和绝对路径。

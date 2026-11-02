# 第十章：Web 服务化与 SSE 流式传输

> **前置**：ch09（概念）｜**本章新增**：HTTP/SSE 服务、SQLite 持久化、树形历史、assistant-ui 前端｜**刻意简化**：去掉 TUI / context / memory / RAG / skills｜**下一章**：ch11

> **近期社区动态（2026-05 → 2026-11）**：见 [docs/COMMUNITY-2026.md](../docs/COMMUNITY-2026.md)（本章对应小节）。

欢迎来到第十章！在前面的章节中，我们构建的 Agent 都运行在终端里——用户在 TUI 里输入，Agent 在本地执行。

本章的核心任务是**服务化**：把 Agent 的核心逻辑从 TUI 中剥离出来，封装成 HTTP 服务，让浏览器也能像 ChatGPT 一样与 Agent 流式对话。

> **核心设计理念**：
> - **Agent 与传输层解耦**：Agent 内部用 `StreamEvent` channel 输出事件，不感知任何 HTTP 概念；HTTP 层负责把事件转成 SSE 格式推给客户端
> - **树形消息结构**：每条消息记录 `parent_message_id`，支持分支对话；查询历史时沿祖先链向上追溯，拼接成完整的 LLM history

---

## 🎯 本章目标

1. **Agent 服务化**：如何把 CLI Agent 剥离 TUI，封装为 HTTP 服务
2. **SSE 流式传输**：Server-Sent Events 的服务端实现，以及前后端协议设计
3. **会话持久化**：Conversation / Message 数据模型设计，用 SQLite 存储对话历史
4. **树形对话历史**：通过 `parent_message_id` 支持分支对话，`buildHistory` 沿祖先链重建 LLM context

---

## 🛠 准备工作

需要 Go 1.25+ 和根目录 `config.json`（服务端只读取 `front_model`）：

```bash
cp config.example.json config.json
# 填入 llm_providers.front_model.api_key
```

前端需要 Node 24 + pnpm：

```bash
pnpm --dir frontend install
pnpm --dir frontend dev
```

---

## 📌 相对上一章的变化

本章从 ch09 回退到 ch06 的能力面（没有 context / memory / skills / RAG），新增 HTTP + SSE、SQLite 会话与消息、树形历史，以及 `frontend/` 的 assistant-ui 客户端。重点从“单机 CLI”切换到“服务端 + 浏览器”。

---

## 📖 核心原理解析

### 1. Agent 与传输层解耦

**问题**：之前章节的 Agent 直接向 TUI channel 发送 `MessageVO`，耦合了展示层。如果要改成 HTTP 服务，需要修改 Agent 内部代码。

**解决方案**：定义 `StreamEvent`——一个与传输层无关的内部事件类型。Agent 只负责产生 `StreamEvent`，HTTP 层负责转换格式：

```
Agent.RunStreaming()
   ↓  chan StreamEvent（业务语义）
service.CreateMessage()
   ↓  toSSEMessage()
   ↓  chan SSEMessageVO（传输格式）
controller.createMessage()
   ↓  c.SSEvent() + Flush
   ↓  text/event-stream（HTTP 协议）
```

`StreamEvent` 只包含业务字段，不涉及 HTTP：

```go
type StreamEvent struct {
    Event            string  // reasoning / content / tool_call / tool_result / error
    Content          string
    ReasoningContent string
    ToolCall         string
    ToolArguments    string
    ToolResult       string
}
```

这样 Agent 核心逻辑完全不感知传输层，未来换成 WebSocket 也只需改 HTTP 层。

### 2. 树形消息结构与历史重建

**问题**：Web 服务需要支持多轮对话，但每次 HTTP 请求是无状态的。Agent 需要知道"之前聊了什么"才能继续对话。

**解决方案**：借鉴 ChatGPT 的设计，每条消息记录 `parent_message_id`，消息之间形成一棵树：

```
根消息（parent=""）
  └── 第2轮消息（parent=根消息ID）
        ├── 第3轮消息A（parent=第2轮ID）  ← 用户继续对话
        └── 第3轮消息B（parent=第2轮ID）  ← 用户重新生成
```

`buildHistory` 从 `parent_message_id` 出发，沿树向根追溯，将路径上每条消息的 `Rounds` 拼接成 LLM history：

```go
// 从 parentMessageID 向根节点追溯，收集路径（顺序：根 -> parent）
path := make([]*ChatMessage, 0)
cur := parentMessageID
for cur != "" {
    msg, ok := index[cur]
    if !ok { break }
    path = append(path, msg)
    cur = msg.ParentMessageID
}
// 反转后拼接每条消息的 rounds
```

### 3. Rounds 持久化

每次 `RunStreaming` 结束后，把本轮所有 LLM 消息（user + assistant + tool results）序列化为 JSON 存入 `ChatMessage.Rounds`：

```
一次对话轮次的 Rounds：
  [user消息, assistant消息（含tool_calls）, tool结果消息, assistant最终消息]
       ↓ json.Marshal
  存入 ChatMessage.Rounds 字段
       ↓ 下次对话时 json.Unmarshal
  恢复为 []OpenAIMessage，作为 history 传给 Agent
```

这样即使服务重启，历史对话也能完整恢复。

### 4. SSE 并发架构

每个请求的处理流程：

```
HTTP 请求进入
   ↓
创建 eventCh (chan SSEMessageVO)
   ↓
设置 SSE 响应头（text/event-stream）
   ↓
go func() {
    s.CreateMessage(ctx, ..., eventCh)  ← 后台运行 Agent
    close(eventCh)
}()
   ↓
for e := range eventCh {
    c.SSEvent("message", e)  ← 主协程写 SSE
    c.Writer.Flush()         ← 立即推送给客户端
}
```

客户端断开连接时，`ctx.Done()` 会触发，Agent loop 中的 `select` 检测到后提前退出，避免资源泄漏。

---

## 💻 代码结构速览

```
ch10/
├── agent/
│   ├── agent.go       # Agent 核心：RunStreaming 接受 history，通过 eventCh 输出 StreamEvent
│   ├── stream.go      # StreamEvent 类型定义（与传输层解耦）
│   └── tool/
│       ├── tool.go    # Tool 接口定义
│       └── bash.go    # Bash 工具实现
├── server/
│   ├── db.go          # GORM 数据模型（Conversation、ChatMessage）+ InitDB
│   ├── history.go     # buildHistory：沿 parent_message_id 树追溯，拼接 LLM history
│   ├── service.go     # 业务层：CRUD + 启动 Agent 流式执行 + toSSEMessage 转换
│   └── controller.go  # HTTP 路由 + SSE 控制器（Gin）
├── vo/
│   ├── vo.go          # 请求/响应 VO：CreateConversationReq、ChatMessageVO 等
│   └── sse.go         # SSEMessageVO：SSE 事件的 JSON 格式
└── main/
    └── main.go        # 入口：初始化 DB、Agent、Server，监听 :8080
```

### 数据模型

```go
type Conversation struct {
    ConversationID string  // UUID
    UserID         string
    Title          string
    CreatedAt      int64
}

type ChatMessage struct {
    MessageID       string  // UUID
    ConversationID  string
    ParentMessageID string  // 树形结构的关键字段
    Query           string  // 用户原始提问
    Response        string  // 模型最终输出
    Rounds          string  // 本轮所有 LLM 消息，JSON 序列化
    Model           string
    Usage           string  // token 使用量，JSON 序列化
    CreatedAt       int64
}
```

---

## 🚀 动手运行

### 运行服务

```bash
cp config.example.json config.json
# 编辑 config.json，填入 LLM 配置

go run ./ch10/main
# 服务监听 :8080，数据库文件 ch10.db 自动创建
```

### API 调用示例

```bash
# 1. 创建会话
curl -s -X POST http://localhost:8080/api/conversation \
  -H 'Content-Type: application/json' \
  -d '{"user_id":"alice","title":"我的第一个对话"}' | jq .

# 响应：
# {"code":0,"msg":"ok","data":{"conversation_id":"uuid-xxx",...}}

# 2. 发送消息，观察 SSE 流式输出（替换 {conversation_id}）
curl -N -X POST http://localhost:8080/api/conversation/{conversation_id}/message \
  -H 'Content-Type: application/json' \
  -d '{"user_id":"alice","query":"用 Go 写一个冒泡排序"}'

# 流式输出：
# data: {"message_id":"...","event":"content","content":"好的"}
# data: {"message_id":"...","event":"content","content":"，我来"}
# data: {"message_id":"...","event":"tool_call","tool_call":"bash","tool_arguments":"{...}"}
# data: {"message_id":"...","event":"tool_result","tool_call":"bash","tool_result":"..."}
# data: {"message_id":"...","event":"content","content":"..."}

# 3. 继续对话（传入 parent_message_id 保持上下文）
curl -N -X POST http://localhost:8080/api/conversation/{conversation_id}/message \
  -H 'Content-Type: application/json' \
  -d '{"user_id":"alice","query":"再加上注释","parent_message_id":"{message_id}"}'

# 4. 查询历史消息
curl -s http://localhost:8080/api/conversation/{conversation_id}/message | jq .
```

### SSE 事件格式

每个 SSE 事件的 `data` 字段是一个 JSON 对象：

| `event` 值 | 含义 | 有效字段 |
|-----------|------|---------|
| `reasoning` | 推理模型的思考内容（流式） | `reasoning_content` |
| `content` | 模型文本输出片段（流式） | `content` |
| `tool_call` | 发起工具调用 | `tool_call`（工具名）、`tool_arguments` |
| `tool_result` | 工具执行结果 | `tool_call`（工具名）、`tool_result` |
| `error` | 错误信息 | `content` |

---

## 🖥 前端

`frontend/` 目录包含一个配套的 React + TypeScript 前端，用于直观展示 SSE 流式对话效果，技术栈为 Vite + React + Tailwind CSS。

前端实现了：侧边栏会话列表、流式消息气泡（含推理内容折叠展示）、工具调用折叠面板。

```bash
cd frontend
pnpm install
pnpm dev   # 开发模式，自动代理 /api 到 localhost:8080
```

前端通过原生 `fetch` + `ReadableStream` 消费 SSE（避免 EventSource 自动重连导致重复 POST），代理配置在 `vite.config.js` 中：

```ts
server: {
  proxy: { '/api': 'http://localhost:8080' }
}
```

---

## 🔧 API 接口一览

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/api/conversation` | 创建会话 |
| `GET` | `/api/conversation?user_id=` | 列出会话 |
| `POST` | `/api/conversation/:id/message` | 发送消息（SSE 流式响应） |
| `GET` | `/api/conversation/:id/message` | 查询消息历史 |
| `PATCH` | `/api/conversation/:id` | 重命名会话 |
| `DELETE` | `/api/conversation/:id` | 删除会话 |

> ch11 在同一套 HTTP/SSE 结构上增加了 `/metrics` 与 `/api/trace/:id`，并默认同样监听 `:8080`。

---

## 与前几章的关系

ch10 从 ch09 的能力面**回退**到“核心 Agent Loop + bash 工具”，把注意力集中在服务化这一工程问题：去掉 TUI、上下文管理、记忆、RAG 与技能，不代表它们没有价值，而是让 HTTP / SSE / 持久化这条链路更容易看清。需要这些能力时，可以回到对应章节，或按同样的 `Agent` 接口把它们接回来。

---

## 🚫 故意不实现

- 不做鉴权、多租户与限流。
- 不做可观测性（ch11）。
- 不做上下文压缩、记忆、RAG、Skills。
- SSE 没有断线重连与消息去重；一次请求就是一条完整的流。

---

## ➡️ 下一章

ch11 在同一套服务上加入 OpenTelemetry、Prometheus 与 `AgentTrace`，让每次请求可追溯。

---

## 📚 扩展阅读与参考资料

- [MDN: Server-sent events](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events/Using_server-sent_events)
- [Gin documentation](https://gin-gonic.com/docs/)
- [GORM documentation](https://gorm.io/docs/)

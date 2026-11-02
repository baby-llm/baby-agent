# 第四章：让 Agent 接入 MCP 生态

> **前置**：ch03｜**本章新增**：MCP 客户端、工具命名空间、本地工具与 MCP 工具共存｜**下一章**：ch05

> **近期社区动态（2026-05 → 2026-11）**：见 [docs/COMMUNITY-2026.md](../docs/COMMUNITY-2026.md)（本章对应小节）。

欢迎来到第四章！在第三章的基础上，本章继续完善一个接入 MCP 生态的 Agent：

- **接入 MCP（Model Context Protocol）工具生态**

---

## 🎯 本章目标

1. **MCP 原理与接入**：理解 MCP 的基本角色与工具生命周期，并在 Agent 中同时管理 MCP 工具与本地工具。

---

## 🛠 准备工作

复用根目录的 `.env` 配置（见项目根目录 `README.md`）。

```env
OPENAI_API_KEY=sk-your-api-key-here
OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_MODEL=gpt-5.2
```

此外，本章默认在 `mcp-server.json` 中配置了一个 MCP 文件系统服务器（基于 `@modelcontextprotocol/server-filesystem`）。这需要本地能够运行 `npx`（通常意味着安装 Node.js）。

---

## 📌 相对上一章的变化

新增 MCP Client（stdio / HTTP）、工具发现、命名空间（`babyagent_mcp__<server>__<tool>`），并让本地工具与 MCP 工具共用同一个 `Tool` 接口。本地工具仍只有 `bash`；文件读写由 MCP filesystem server 提供。

---

## 📖 核心原理解析

### 1. MCP（Model Context Protocol）与工具生态

#### 1.1 MCP 的基本原理

MCP 是一种开放协议，用于让 Agent/LLM 以统一方式发现并调用外部工具。它主要定义了三类角色：

- **Host**：宿主应用（例如 Claude Desktop），负责用户交互
- **Client**：Host 内为每个 Server 建立的协议连接，负责发现与调用工具
- **Server**：对外暴露 tools / resources / prompts 的服务端

> **Tool** 是 Server 暴露的一种能力（输入 JSON Schema，输出结构化结果），不是独立的协议角色。

MCP 的意义在于：**用标准协议解决”模型 × 工具”爆炸式组合问题**，让工具接入更可复用、更可组合。

#### 1.2 MCP 协议交互流程（可选阅读）

MCP 基于 **JSON-RPC 2.0** 协议进行通信，支持两种传输方式：

| 传输方式 | 适用场景 | 特点 |
|---------|---------|------|
| **stdio** | 本地进程间通信 | 通过标准输入/输出通信，简单可靠 |
| **SSE/HTTP** | 远程/网络通信 | 基于服务器发送事件，支持跨机器调用 |

##### 协议交互的完整流程

**阶段一：初始化连接**

```json
// Client → Server: 初始化请求
{
  “jsonrpc”: “2.0”,
  “id”: 1,
  “method”: “initialize”,
  “params”: {
    “protocolVersion”: “2024-11-05”,
    “capabilities”: {
      “roots”: {
        “listChanged”: true
      }
    },
    “clientInfo”: {
      “name”: “babyagent”,
      “version”: “1.0.0”
    }
  }
}

// Server → Client: 初始化响应
{
  “jsonrpc”: “2.0”,
  “id”: 1,
  “result”: {
    “protocolVersion”: “2024-11-05”,
    “capabilities”: {
      “tools”: {
        “listChanged”: true
      }
    },
    “serverInfo”: {
      “name”: “filesystem-server”,
      “version”: “1.0.0”
    }
  }
}

// Client → Server: 通知初始化完成
{
  “jsonrpc”: “2.0”,
  “method”: “notifications/initialized”
}
```

**阶段二：工具发现**

```json
// Client → Server: 请求工具列表
{
  “jsonrpc”: “2.0”,
  “id”: 2,
  “method”: “tools/list”
}

// Server → Client: 返回可用工具
{
  “jsonrpc”: “2.0”,
  “id”: 2,
  “result”: {
    “tools”: [
      {
        “name”: “read_file”,
        “description”: “读取文件内容”,
        “inputSchema”: {
          “type”: “object”,
          “properties”: {
            “path”: {
              “type”: “string”,
              “description”: “文件路径”
            }
          },
          “required”: [“path”]
        }
      },
      {
        “name”: “write_file”,
        “description”: “写入文件内容”,
        “inputSchema”: {
          “type”: “object”,
          “properties”: {
            “path”: {“type”: “string”},
            “content”: {“type”: “string”}
          },
          “required”: [“path”, “content”]
        }
      }
    ]
  }
}
```

**阶段三：工具调用**

```json
// Client → Server: 调用工具
{
  “jsonrpc”: “2.0”,
  “id”: 3,
  “method”: “tools/call”,
  “params”: {
    “name”: “read_file”,
    “arguments”: {
      “path”: “/path/to/file.txt”
    }
  }
}

// Server → Client: 返回工具执行结果
{
  “jsonrpc”: “2.0”,
  “id”: 3,
  “result”: {
    “content”: [
      {
        “type”: “text”,
        “text”: “文件的内容在这里...”
      }
    ]
  }
}
```

**阶段四：资源访问（可选）**

MCP 还支持资源的统一访问，类似于”虚拟文件系统”：

```json
// Client → Server: 列出资源
{
  “jsonrpc”: “2.0”,
  “id”: 4,
  “method”: “resources/list”
}

// Client → Server: 读取资源内容
{
  “jsonrpc”: “2.0”,
  “id”: 5,
  “method”: “resources/read”,
  “params”: {
    “uri”: “file:///path/to/resource.txt”
  }
}
```

##### 协议特点与设计考虑

**1. JSON-RPC 2.0 的优势**

- **简单性**：基于 JSON，易于调试和实现
- **双向通信**：支持 Server 主动推送通知
- **错误处理**：标准化的错误响应格式

**2. 传输层抽象**

MCP 将协议层与传输层分离，同样的 JSON-RPC 消息可以通过：

- **stdio**：适合本地工具服务器（如本章的文件系统服务器）
- **HTTP/SSE**：适合远程服务器或云服务

**3. 能力协商（Capability Negotiation）**

在 `initialize` 阶段，Client 和 Server 会交换各自支持的能力：

```json
“capabilities”: {
  “tools”: {},           // 支持工具调用
  “resources”: {},       // 支持资源访问
  “prompts”: {}          // 支持提示模板
}
```

这样实现了协议的向后兼容和渐进式增强。

##### 实际开发中的调试技巧

**启用 MCP 协议日志**：

```go
// 伪代码：在 McpClient.callTool 中记录实际请求与响应
log.Printf("MCP tool call: %s args=%s", toolName, argumentsInJSON)
log.Printf("MCP result: %v", mcpResult)
```

**常见问题排查**：

| 问题 | 可能原因 | 解决方案 |
|------|---------|---------|
| 工具列表为空 | Server 未正确启动 | 检查 stdio 进程是否正常运行 |
| 调用超时 | Server 执行时间过长 | 增加 timeout 配置或优化 Server |
| 参数验证失败 | JSON Schema 不匹配 | 检查 inputSchema 与实际参数 |
| 通信断开 | 进程崩溃或网络问题 | 实现自动重连机制 |

#### 1.3 本章的 MCP 接入方式

在 `ch04/mcp.go` 中实现 MCP 客户端封装，核心流程：

1. 从 `mcp-server.json` 加载 MCP 服务器配置。
2. 连接 MCP Server（支持 stdio 或 HTTP 方式）。
3. 调用 `ListTools` 拉取工具列表，并封装为本项目统一的 `tool.Tool` 接口。
4. 在 Agent 中将 MCP 工具合并到 tools 列表中。

**工具命名策略**

为了避免冲突，本章将 MCP 工具名包装成：

```
babyagent_mcp__{serverName}__{toolName}
```

这样模型侧看到的是“命名空间化工具”，而 MCP 服务器端实际执行的是原始工具名。

相关代码：`ch04/mcp.go`、`shared/mcp.go`

#### 1.4 Agent 如何管理 MCP 工具与本地工具

`ch04/agent.go` 中，Agent 维护了两类工具：

- `nativeTools`：本地实现的工具（read / write / edit / bash）
- `mcpClients`：通过 MCP 动态加载的工具集合

> **实现状态**：ch04 的 `main` 只注册 `bash` 作为本地工具；`read` / `write` / `edit` 由 MCP filesystem server 提供，不是原生工具。

在 `buildTools()` 时统一注册给模型；在 `execute()` 时先查本地工具，再查 MCP 工具，确保两类工具可以无缝共存。

这使得 Agent 的工具能力具备“本地 + 远程”双模式：

- 本地工具：低延迟、可控、适合文件与命令
- MCP 工具：扩展性强、生态丰富、可跨应用复用

相关代码：`ch04/agent.go`

---

## 💻 代码结构速览

- `ch04/agent.go`：增强后的 Agent Loop（MCP 支持）
- `ch04/mcp.go`：MCP 客户端与 MCP Tool 封装
- `shared/mcp.go`：MCP 服务器配置解析
- `mcp-server.json`：MCP 服务器配置（默认文件系统工具）

---

## 🚀 动手运行

进入项目根目录，执行：

```bash
go run ./ch04/main
```

示例：

- “请读取 README.md 并总结项目目标”
- “使用 MCP 工具列出当前目录下的文件”

如果 MCP 文件系统服务正常启动，你会看到工具调用日志出现在 TUI 中。

---

## 🚫 故意不实现

- 不做上下文工程与记忆（ch05 / ch06）。
- MCP server 初始化失败时只跳过该 server，不做重连或健康检查。
- 工具执行仍然没有沙箱与人工确认（ch08）。

---

## ➡️ 下一章

ch05 开始管理上下文窗口：截断、卸载、摘要三种策略。

---

## 📚 扩展阅读与参考资料

以下资料可帮助你进一步理解 MCP 相关内容：

1. MCP 官方文档（概览）：`https://modelcontextprotocol.io/`
2. MCP 规范（Spec）：`https://github.com/modelcontextprotocol/spec`
3. MCP Go SDK：`https://github.com/modelcontextprotocol/go-sdk`
# 第八章：沙盒与安全防御（Guardrails）

> **前置**：ch06（代码基线；RAG 未集成）｜**本章新增**：Docker 沙盒、人工确认、ESC 取消｜**下一章**：ch09

> **近期社区动态（2026-05 → 2026-11）**：见 [docs/COMMUNITY-2026.md](../docs/COMMUNITY-2026.md)（本章对应小节）。

欢迎来到第八章！在前面的章节中，我们已经构建了一个功能完善的 AI Agent，具备了工具调用、上下文管理、记忆系统和 RAG 能力。**本章代码基于 ch06 的上下文/记忆能力；RAG 在 ch07 中实现，尚未集成进本章的 Agent。**

然而，**当 Agent 能够执行命令、读写文件时，安全性就变得至关重要**。本章介绍如何为 Agent 设计安全防护机制，包括 Docker 沙盒隔离和人工确认机制。

---

## 🎯 你将学到什么

1. **Docker 沙盒隔离**：在容器中运行 bash 命令，保障本地环境安全
2. **智能工具选择**：自动检测 Docker 可用性，优雅降级到普通 bash
3. **人工确认（Human-in-the-loop）**：在执行危险命令前请求用户确认
4. **确认状态管理**：Allow/Reject/Always Allow 三种确认选项
5. **Context 取消机制**：用户按 ESC 提前终止 Agent Loop

---

## 🛠 核心功能

### 1. Docker 沙盒隔离

**问题**：Agent 执行的 bash 命令可能对本地系统造成破坏性影响。

**解决方案**：使用 Docker 容器隔离执行环境，限制命令的影响范围。

**沙盒架构**：

```
┌─────────────────────────────────────────────────┐
│  Host Machine                                    │
│                                                  │
│  ┌────────────────────────────────────────────┐ │
│  │  Docker Container: babyagent-sandbox       │ │
│  │  Image: alpine:3.19                        │ │
│  │                                            │ │
│  │  /workspace (mounted)  ←→  Project Dir    │ │
│  │                                            │ │
│  │  $ bash commands execute here              │ │
│  └────────────────────────────────────────────┘ │
│                                                  │
│  docker exec container sh -c "command"           │
└─────────────────────────────────────────────────┘
```

**核心特性**：

- **工作区隔离**：每个工作区使用独立的容器（容器名包含项目名称）
- **Lazy Initialization**：首次使用时自动启动容器
- **自动创建**：容器不存在时自动创建并配置
- **卷挂载**：将项目目录挂载到容器的 `/workspace`
- **优雅降级**：Docker 不可用时自动切换到普通 bash
- **保持运行**：容器使用 `sleep infinity` 保持运行状态

**容器命名规则**：

- 项目目录 `/home/user/project-a` → 容器名 `babyagent-sandbox-project-a`
- 项目目录 `/home/user/project-b` → 容器名 `babyagent-sandbox-project-b`

这样每个工作区都有独立的沙盒环境，互不干扰。

**工具工厂模式**：

系统会自动检测 Docker 是否可用：
- Docker 可用 + 工作目录非空 → 使用 `DockerBashTool`
- Docker 不可用或工作目录为空 → 使用普通 `BashTool`

---

### 2. 工具调用确认

**问题**：Agent 可能执行危险操作（如 `rm -rf`），需要人工干预。

**解决方案**：为工具配置确认选项，执行前展示确认框。

```
┌─────────────────────────────────────────┐
│ 工具确认                                 │
│ bash(cowsay "Hello")                    │
│                                         │
│   ┌─────────────────────────┐           │
│   │   ▶ 允许                │           │
│   │     拒绝                │           │
│   │     始终允许            │           │
│   └─────────────────────────┘           │
│                                         │
│ ↑↓ 选择  Enter 确认  Esc 拒绝           │
└─────────────────────────────────────────┘
```

**三种确认选项**：
- **允许**：执行本次工具调用
- **拒绝**：拒绝本次调用，终止 Agent Loop
- **始终允许**：执行并记录，当前会话同类工具不再确认

---

### 3. 提前终止 Loop（ESC）

**需求**：用户可以随时按 ESC 终止 Agent Loop。

**实现机制**：
1. TUI 创建可取消的 context
2. 用户按 ESC → 发送取消信号
3. Agent 检测到取消 → 跳过 policies/memory，保存当前状态

---

## 📖 代码结构速览

### Agent (`ch08/agent.go`)
- `ToolConfirmConfig`：工具确认配置
- `alwaysAllowTools`：当前会话的"始终允许"记录
- `findTool()`：统一的工具查找方法（支持本地和 MCP 工具）
- `RunStreaming()`：支持确认和取消的流式请求

### Docker Tool (`ch08/tool/docker_bash.go`)
- `DockerBashTool`：Docker 容器化 bash 工具
- `ensureSandboxContainer()`：确保容器运行（启动或创建）
- Lazy 初始化：首次使用时启动容器

### Tool Factory (`ch08/tool/factory.go`)
- `CreateBashTool()`：智能 bash 工具创建函数
- `checkDockerAvailable()`：检测 Docker 是否可用
- 自动选择合适的 bash 工具实现

### VO (`ch08/vo.go`)
- `ConfirmationAction`：确认动作枚举（Allow/Reject/AlwaysAllow）
- `ToolConfirmationVO`：确认请求数据结构
- `MessageTypeToolConfirm`：确认消息类型

### TUI (`ch08/tui/tui.go`)
- `stateAwaitingConfirmation`：等待用户确认状态
- `confirmOptions`：确认选项列表
- `selectedConfirmIdx`：当前选中的选项索引
- `renderConfirmBox()`：渲染确认框 UI

---

## 💡 使用示例

### 运行 TUI

```bash
go run ./ch08/main
```

### Docker 沙盒示例

**首次运行**：
```
你: 列出当前目录的文件

Agent: [检测到 Docker，启动沙盒容器...]
      [创建容器 babyagent-sandbox，镜像 alpine:3.19]

Agent: [在容器中执行命令]
      docker exec babyagent-sandbox sh -c "ls -la"

输出: drwxr-xr-x  workspace/
      -rw-r--r--  main.go
      -rw-r--r--  README.md
      ...
```

### 工具确认示例

```
你: 用 bash 说 hello

Agent: 准备调用 bash 工具

[确认框弹出]
  ┌─────────────────────────────────┐
  │ 工具确认                         │
  │ bash(cowsay "Hello")            │
  │                                 │
  │   ▶ 允许                         │
  │     拒绝                         │
  │     始终允许                     │
  └─────────────────────────────────┘
  │ ↑↓ 选择  Enter 确认  Esc 拒绝    │

[用户按 ↓ 选择"始终允许"，Enter 确认]

Agent: [在容器中执行命令]
      docker exec babyagent-sandbox sh -c "cowsay Hello"

输出: ____
       / Hello \
       --------
        \  /
         \/

[后续 bash 调用不再需要确认]
```

### ESC 取消示例

```
你: 帮我删除所有日志文件

Agent: 准备调用 bash

[用户按 ESC]

提示: 用户取消了 agent loop，消息已保留。

你: 继续执行删除操作

[Agent 从上次消息继续，不需要重新上下文]
```

---

## 🔧 配置说明

### 工具确认配置

在 `ch08/tui/main.go` 中配置需要确认的工具：

- `tool.AgentToolBash`：bash 命令需要确认
- `tool.AgentToolWrite`：写文件需要确认
- `tool.AgentToolEdit`：编辑文件需要确认
- `tool.AgentToolRead`：读取文件通常不需要确认

> **实现状态**：当前入口是 `ch08/main/main.go`，只注册 `bash` 与 `load_storage`；`write` / `edit` / `read` 的确认配置是扩展设计，尚未接入。下面保留这段说明，方便你按同样方式扩展。

### MCP 工具确认

MCP 工具使用完整工具名（如 `babyagent_mcp__filesystem__write_file`）作为 key 进行配置。

### Docker 沙盒配置

默认配置（可在 `ch08/tool/docker_bash.go` 中修改）：
- 容器名称：`babyagent-sandbox-{项目名}`（每个工作区独立）
- 镜像：`alpine:3.19`
- 工作目录：自动挂载当前项目目录到 `/workspace`

---

## ⚠️ 注意事项

1. **Docker 要求**：使用 Docker 沙盒需要本地安装并启动 Docker
2. **容器持久化**：容器使用 `unless-stopped` 重启策略，重启后自动运行
3. **确认状态与会话绑定**：`alwaysAllowTools` 在 `ResetSession()` 后清空
4. **ConfirmReject 的语义**：拒绝会把 "user rejected tool call" 回填给模型并继续 loop；只有 ESC 会取消本轮。旧描述“拒绝即终止”与当前代码不一致。

---

## 📚 延伸阅读

### 功能扩展方向

**更细粒度的确认策略**
- 按命令前缀：`rm*` 开头的命令需要确认
- 按文件路径：修改 `/etc/*` 文件需要确认
- 按参数大小：大文件操作需要确认
- 按时间窗口：第一次调用确认，N 分钟内免确认

**审计日志**
- 记录所有工具调用和用户确认决策
- 便于安全审计和问题排查

**权限级别**
- Safe：无需确认（如读取文件）
- Warning：需要确认（如写入文件）
- Danger：需要二次确认（如删除操作）

**更强大的沙盒**
- 网络隔离：禁止容器访问外网
- 资源限制：CPU、内存使用限制
- 只读挂载：某些目录只读挂载，防止篡改

---

### Docker 与容器安全

**Docker 安全最佳实践**
- 使用非 root 用户运行容器
- 限制容器资源（CPU、内存）
- 只读文件系统
- 网络隔离与防火墙规则

**容器逃逸防护**
- 避免特权模式（--privileged）
- 谨慎使用卷挂载
- AppArmor/Seccomp 配置

---

### AI Agent 安全

**Prompt Injection**：提示词注入攻击与防御
- 研究案例：indirect prompt injection
- 防御策略：输入验证、输出清洗、分隔符设计

**工具调用安全**：限制 Agent 能力范围
- 白名单/黑名单策略
- 沙箱执行环境
- 审计与监控

**Human-in-the-Loop**：人机协同的安全设计
- 关键操作人工确认
- 可解释性与透明度
- 紧急停止机制

---

### 相关工具与框架

- **E2B**：专为 AI Agent 设计的沙箱环境
- **Docker Sandboxing**：使用 Docker 构建隔离执行环境
- **gVisor**：用户空间内核，更强的容器隔离
- **Firecracker**：轻量级虚拟机，微秒级启动

---

### 推荐资源

- **OWASP LLM Top 10**：大语言模型应用的安全风险清单
- **Anthropic Constitutional AI**：通过原则约束 AI 行为
- **OpenAI Safety Guidelines**：OpenAI 的安全实践指南
- **Claude's Guardrails**：Claude 的安全防护机制设计
# 第六章：记忆机制（Memory System）

> **前置**：ch05｜**本章新增**：两层记忆（Global / Workspace）、LLM 驱动的记忆更新与持久化｜**下一章**：ch07（可选）→ ch08

> **近期社区动态（2026-05 → 2026-11）**：见 [docs/COMMUNITY-2026.md](../docs/COMMUNITY-2026.md)（本章对应小节）。

欢迎来到第六章！在第五章的基础上，本章介绍 Agent 开发中另一个重要概念：**记忆系统**（Memory System）。

上下文工程解决了"如何在有限窗口内高效运行"的问题，但 Agent 仍然缺乏长期记忆能力。每当会话重启，Agent 就会"遗忘"之前的所有对话。本章实现一套完整的记忆机制，让 Agent 能够跨会话保持上下文连贯性。

---

## 🎯 本章目标

1. **记忆层级设计**：理解为什么需要两层记忆（Global、Workspace）
2. **Memory 接口设计**：如何设计可插拔的记忆抽象接口
3. **LLM 驱动的记忆更新**：使用 LLM 自动提取和更新记忆内容
4. **持久化存储机制**：如何将记忆持久化到文件系统
5. **记忆事件系统**：在 TUI 中实时展示记忆更新状态
6. **System Prompt 集成**：如何将记忆注入到 System Prompt 中

---

## 🛠 准备工作

本章启动时会读取以下配置文件（见 `ch06/main/main.go`）：

1. `config.json`：应用模型配置（前台模型 + 后台记忆更新模型）
2. `mcp-server.json`：MCP 服务配置（可选）

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

---

## 📌 相对上一章的变化

在 ch05 的上下文引擎上加入 `memory` 包：Global / Workspace 两层记忆、LLM 驱动的记忆更新、文件持久化、记忆事件，以及把记忆注入 system prompt。`CommitTurn` 现在同时负责压缩上下文和更新记忆。

---

### 与 ch05 的复制差异

| 文件 | 差异 |
|---|---|
| `context/engine.go` | 构造参数增加 `memory.Memory`；`CommitTurn` 更新记忆；system prompt 注入 `{memory}` |
| `context/*` 其余文件 | 与 ch05 保持一致（仅 import 路径不同），修复需同步两章 |
| `memory/*` | 本章新增 |
| `storage/filesystem.go` | 本章新增文件持久化；`storage.go` 与 ch05 保持一致 |
| `agent.go` / `tui/` / `main` | 接入记忆事件与记忆注入 |

---

## 📖 核心原理解析

### 1. 为什么需要记忆系统？

Agent 在运行过程中会产生大量有价值的信息：

| 信息类型 | 生命周期 | 示例 |
|---------|---------|------|
| **用户偏好** | 跨会话长期保存 | 用户习惯使用 vim、总是使用 verbose 标志 |
| **项目知识** | 跨会话长期保存 | 项目结构、构建命令、部署流程 |

没有记忆系统的 Agent：
- ❌ 每次会话都要重新介绍项目背景
- ❌ 无法记住用户的编码偏好
- ❌ 无法从历史对话中学习

有记忆系统的 Agent：
- ✅ 能够记住用户的工作习惯
- ✅ 能够积累项目知识
- ✅ 能够跨会话保持连贯性

---

### 2. 两层记忆架构

本章采用**两层记忆架构**，按照信息的生命周期和作用域进行划分：

#### 2.1 Global Memory（全局记忆）

**作用域**：跨所有会话、跨所有项目

**存储内容**：
- 用户偏好（编辑器选择、编码风格、测试习惯）
- 用户背景（专业领域、经验水平）
- 长期模式（反复出现的需求、常见的解决方案）

**存储位置**：`~/.babyagent/memory/MEMORY.md`

**更新频率**：低（只在发现新的跨会话模式时更新）

---

#### 2.2 Workspace Memory（工作区记忆）

**作用域**：当前项目/工作区

**存储内容**：
- 项目结构（目录组织、关键文件）
- 技术栈（语言、框架、依赖）
- 项目约定（命名规范、代码风格）
- 构建与部署（编译命令、测试命令、部署流程）
- 已知问题和解决方案

**存储位置**：`{workspace}/.babyagent/memory/MEMORY.md`

**更新频率**：中（在项目内发现新知识时更新）

---

### 3. Memory 接口设计

本章通过 `Memory` 接口实现可插拔的记忆系统：

```go
type Memory interface {
    String() string
    Update(ctx context.Context, newMessages []shared.OpenAIMessage) error
}
```

**设计理念**：
1. **简单抽象**：只有两个方法，易于实现和测试
2. **解耦更新器**：通过 `MemoryUpdater` 接口将更新逻辑分离
3. **可持久化**：通过 `Storage` 接口支持多种存储后端

相关代码：`ch06/memory/memory.go`

---

### 4. LLM 驱动的记忆更新

记忆更新是一个复杂的信息提取任务，本章使用 LLM 来自动完成：

```go
type MemoryUpdater interface {
    Update(ctx context.Context, oldMemory MemoryContent, newMessages []shared.OpenAIMessage) (MemoryContent, error)
}
```

**更新流程**：
1. 将当前记忆和新一轮对话消息组合成 Prompt
2. 调用 LLM 分析并生成更新后的记忆
3. 解析 LLM 返回的 XML 标签，更新两层记忆（Global / Workspace）
4. 将 Global/Workspace 记忆持久化到文件系统

**Prompt 设计要点**：
- 明确两层记忆的定义和作用域
- 指导 LLM 使用 Markdown 格式
- 强调简洁性和信息保留
- 提供示例帮助 LLM 理解期望输出

> **实现状态**：本章代码实现两层记忆（Global + Workspace），并使用 XML 标签解析 LLM 输出。文档中早期的“三层记忆 / JSON”保留作为扩展思路；要增加第三层，需要在 `ch06/memory` 中扩展存储与 prompt。

相关代码：`ch06/memory/update.go`

---

### 5. 记忆事件系统

与 Policy 事件系统类似，本章实现了记忆更新的事件通知机制：

```go
func (c *Engine) SetMemoryEventHook(hook func(running bool, err error)) {
    c.onMemoryEvent = hook
}
```

**事件时机**：
1. 记忆更新开始：`running=true, err=nil`
2. 记忆更新结束：`running=false, err=更新结果`

**TUI 展示**：
- 运行中：`记忆更新: (运行中...)`
- 成功完成：`记忆更新: (已完成)`
- 更新失败：`记忆更新: (已失败)`

颜色与 Policy 一致（黄色 228），在 TUI 中清晰可见。

相关代码：`ch06/context/engine.go`、`ch06/vo.go`、`ch06/tui/entry.go`

---

### 6. System Prompt 集成

记忆最终需要注入到 System Prompt 中才能被 LLM 使用：

```go
func (c *Engine) BuildSystemPrompt() string {
    replaceMap := make(map[string]string)
    replaceMap["{memory}"] = c.memory.String()
    // ... 其他替换
}
```

**记忆模板格式**：

```markdown
### Global Memory
Here is the memory about the user among all conversations:
{global_memory}

### Workspace Memory
The memory of the current workspace is:
{workspace_memory}
```

这样 LLM 在每次请求时都能看到最新的记忆内容。

相关代码：`ch06/context/engine.go`

---

## 💻 代码结构速览

### Memory 包
- `ch06/memory/memory.go`：`Memory` 接口与 `MultiLevelMemory` 实现
- `ch06/memory/update.go`：`MemoryUpdater` 接口与 `LLMMemoryUpdater`

### Context 包
- `ch06/context/engine.go`：集成记忆系统的上下文引擎
- `ch06/context/policy.go`：`Policy` 接口（继承自第五章）
- `ch06/context/policy_*.go`：各种策略实现（继承自第五章）

### 其他核心模块
- `ch06/storage/storage.go`：存储接口
- `ch06/storage/filesystem.go`：文件系统存储实现
- `ch06/agent.go`：集成记忆系统的 Agent
- `ch06/vo.go`：消息 VO（新增 `MemoryVO`）
- `ch06/tui/entry.go`：TUI 日志条目（新增记忆相关函数）
- `ch06/tui/tui.go`：TUI 界面（新增记忆事件处理）

---

## 🚀 动手运行

进入项目根目录，执行：

```bash
go run ./ch06/main
```

在 TUI 中可尝试：

**1. 观察记忆更新**
```text
你好，我习惯使用 vim 编辑器
```
观察 TUI 中是否出现 `记忆更新: (已完成)` 的提示。

**2. 检查持久化存储**
```bash
# 查看 Global Memory
cat ~/.babyagent/memory/MEMORY.md

# 查看 Workspace Memory
cat $(pwd)/.babyagent/memory/MEMORY.md
```

**3. 验证跨会话记忆**
```text
# 第一次会话
我正在开发一个 Go 项目，使用 Gin 框架

# 重启 TUI 后
请介绍一下我的项目
```
Agent 应该能够从记忆中读取项目信息。

**4. 测试全局记忆和工作区记忆的区分**
```text
# 项目 A 中
我习惯使用 vim 编辑器
项目 A 使用的是 React 框架

# 切换到项目 B
项目 B 使用的是 Go 语言

# 回到项目 A
请介绍我的项目和技术栈
```
Agent 应该能够区分全局记忆（vim 偏好）和工作区记忆（项目 A 用 React，项目 B 用 Go）。

---

## ⚠️ 注意事项

1. **记忆更新的成本**：每轮对话都会调用 LLM 更新记忆，会增加 API 费用
2. **记忆质量依赖 LLM**：记忆提取的准确性取决于更新用的 LLM 能力
3. **存储路径权限**：确保 `~/.babyagent` 和工作区目录有写权限
4. **敏感信息**：记忆中可能包含敏感信息，注意保护存储文件
5. **记忆更新时机**：当前实现在每轮对话结束后更新，可根据需要调整频率
6. **策略必须先评测再上线**：无论是注入策略、更新策略还是衰减策略，都应先通过可复现实验验证收益，不能仅凭人工判断视为优化并直接上线

---

## 🎓 进阶记忆机制设计（可选阅读）

本章实现的是一个基础的两层记忆系统。在实际生产环境中，Agent 的记忆机制会更加复杂和精细。下面介绍几种常见的进阶记忆机制设计思路。

### 1. 语义召回（Semantic Retrieval）

**问题**：随着对话量增加，记忆内容会不断膨胀，直接将全部记忆注入 System Prompt 会导致 Token 消耗过大，且信息噪声过多，影响 LLM 关注重点。

**解决方案**：使用向量数据库实现语义召回。将每个记忆片段通过 Embedding 模型转换为向量，存储到向量数据库中。当用户提出问题时，将问题也转换为向量，检索最相关的 Top-K 记忆片段，只将这些相关记忆注入 System Prompt。

**关键概念**：
- **Embedding 模型**：将文本转换为高维向量表示
- **向量数据库**：专门存储和检索向量的数据库（如 Pinecone、Milvus、Chroma）
- **余弦相似度**：衡量两个向量之间相似度的指标
- **Top-K 检索**：返回相似度最高的 K 个结果

---

### 2. 跨天记录与时间衰减（Time-based Memory Decay）

**问题**：用户的偏好和项目情况会随时间变化，过时的记忆可能误导 LLM（例如用户从 vim 换到了 VSCode）。

**解决方案**：为每条记忆添加时间戳和权重，根据时间推移逐步降低记忆的权重。过时的记忆虽然仍保留在存储中，但在注入 System Prompt 时权重较低，甚至被过滤掉。

**衰减策略**：
- **线性衰减**：权重随时间线性下降，直到归零
- **指数衰减**：权重随时间指数下降，早期快速衰减，后期缓慢
- **时间窗口**：只保留最近 N 天内的记忆，超过窗口的记忆自动归档

---

### 3. 分类存储与结构化记忆（Structured Memory）

**问题**：当前实现将所有记忆存储在一个 Markdown 文件中，难以精确检索特定类型的记忆，记忆之间的关联关系也会丢失。

**解决方案**：将记忆按类型结构化存储到多个文件中。例如：
- **preferences/**：用户偏好（编辑器、编码风格、语言偏好）
- **project/**：项目知识（结构、技术栈、命令）
- **conversations/**：对话摘要（按日期归档）
- **entities/**：实体信息（文件、API、函数）

每条记忆可包含元数据：创建时间、更新时间、置信度、来源等。

**优势**：
- 可以针对不同类型采用不同的更新策略
- 支持更精确的检索（如只检索编辑器偏好）
- 便于记忆的可视化和人工编辑

---

### 4. 分层记忆架构（Hierarchical Memory）

**问题**：Global/Workspace 两层可能不够细粒度，需要在不同抽象层次上管理记忆。

**解决方案**：采用多层记忆架构：

1. **Session Memory（会话记忆）**：当前会话的完整对话历史，会话结束时归档
2. **Workspace Memory（工作区记忆）**：当前项目的知识，跨会话持久化
3. **Global Memory（全局记忆）**：用户偏好、长期模式，跨所有项目和会话

层与层之间通过压缩和提取进行转换：
- Session → Workspace：会话结束时提取项目相关知识
- Workspace → Global：检测到跨项目模式时提取用户偏好

---

### 5. 记忆冲突检测与解决（Memory Conflict Resolution）

**问题**：LLM 可能提取出相互矛盾的记忆，用户也可能明确纠正之前的记忆。

**解决方案**：实现记忆版本管理和冲突解决机制：

1. **版本控制**：每条记忆有版本号，新版本覆盖旧版本
2. **来源标记**：区分记忆来源（LLM 提取、用户明确说明、用户纠正）
3. **优先级规则**：用户纠正 > 用户明确说明 > LLM 提取
4. **置信度评分**：LLM 提取时同时给出置信度，低置信度记忆需确认

---

### 6. 记忆可视化与人工干预

**问题**：记忆是隐式的，用户无法查看和修改，LLM 可能产生错误的记忆提取。

**解决方案**：提供记忆管理界面，让用户能够：
- 浏览所有存储的记忆
- 查看每条记忆的更新时间和置信度
- 手动编辑或删除错误的记忆
- 标记过时的记忆
- 导出和导入记忆备份

---

### 7. 记忆注入策略优化

**当前方式**：每次都注入全部记忆。

**优化策略**：
- **动态注入**：根据用户查询的相关性决定注入哪些记忆
- **渐进注入**：先注入核心记忆，LLM 需要时再补充
- **压缩注入**：长记忆进行摘要后再注入
- **分类注入**：根据任务类型选择相关的记忆类别

---

### 实现路线建议

如果想从本章的基础系统扩展到进阶系统，建议按以下顺序进行：

| 优先级 | 功能 | 复杂度 | 价值 |
|-------|------|-------|------|
| 1 | 结构化存储 | 低 | 高 |
| 2 | 时间衰减 | 中 | 中 |
| 3 | 记忆可视化 | 中 | 高 |
| 4 | 语义召回 | 高 | 高 |
| 5 | 分层架构 | 中 | 中 |
| 6 | 冲突解决 | 中 | 低 |

---

## 🚫 故意不实现

- 不做 RAG（ch07，可选分支）。
- 记忆在每轮结束时整体更新一次，未做增量合并或冲突检测。
- 不做多用户隔离；记忆文件属于当前 OS 用户与当前工作区。

---

## ➡️ 下一章

ch07 是**可选**的 RAG 分支；ch08 会回到本章基线，讲 Docker 沙盒与人工确认。

---

## 📚 扩展阅读与参考资料

1. **[DeepAgent Long-term memory Documentation](https://docs.langchain.com/oss/python/deepagents/long-term-memory)**
   - DeepAgent 框架的记忆模块设计参考

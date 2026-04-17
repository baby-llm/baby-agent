# BabyAgent - 后端工程师的 AI Agent 教学项目 (Go 语言版)

🏠 **项目地址：** https://github.com/baby-llm/baby-agent

🚀 **从后端视角出发，用 Go 语言手写一遍 Agent 框架的关键原理。**

本项目专为没有 LLM 背景但具备基础 Golang 经验的后端工程师设计。我们将跳过复杂的数学推导，优先从工程实现理解核心概念：你会看到每一步为什么这么设计、替代方案是什么、以及它们在真实系统里会遇到哪些坑。

> **重要说明（请先读）**
>
> - 本项目是**教学仓库**：目标是用可运行的代码把 Agent、Tool Calling、Agent Loop、MCP、Memory、RAG 等机制讲清楚。
> - 本项目**不是**“开箱即用的生产框架”，也**不建议直接用于生产环境的二次开发**。
> - 如果你希望落地到真实业务：这里更适合作为**原理参考 + 原型验证**的起点，你需要根据自己的场景补齐工程化能力与安全边界。

---

## 🏗 核心技术栈

*   **Language:** Go 1.24+
*   **LLM Concepts:** Chat Completions API, SSE 流式传输, Function Calling, ReAct Agent Loop
*   **Advanced AI:** 上下文工程, Memory 系统, Agentic RAG, 技能系统（Skills）, 显式规划（TodoWrite）
*   **System Design:** MCP 协议,  Guardrails 安全防护, Web 服务化
*   **Engineering:** LLM 评测, 可观测性（Trace/Metrics/Log）

---

## 🗺 学习路径图 (按知识循序渐进)

我们将按照"从基础调用到复杂 Agent"的顺序逐步深入，带你完成从零到复杂 Agent 的蜕变。每一阶段都包含可运行的代码示例。

### 第一章：初识 LLM（Raw HTTP 与 OpenAI SDK）

**目标**：拨开 SDK 的迷雾，直视大模型调用的本质。

*   **协议本质**：掌握 `chat/completions` 接口的最小化请求和响应结构
*   **流式输出解析**：深入了解 SSE（Server-Sent Events）协议，实现"打字机"效果
*   **工程实践**：对比 Raw HTTP 和 OpenAI Go SDK 的使用方式
*   **LLM 原理**（可选）：Transformer、训练过程、Token 机制、生成原理

### 第二章：赋予 AI "手脚"（Tool Calling 和 Agent）

**目标**：让 LLM 从"只会聊天"升级为"能动手做事"的 Agent。

*   **Function Calling 协议**：如何向模型声明工具、如何解析工具调用
*   **Agent Loop**：工具调用 → 反馈 → 再推理的闭环流程
*   **ReAct/Tool-Loop 原理**：Reason + Act 的思维-行动交替模式
*   **本地工具封装**：读取文件、写入文件、编辑文件、执行 shell 命令
*   **Chat Template**（可选）：从 API 到模型输入的转换机制

### 第三章：让 Agent "更能看见"（Reasoning 展示、TUI）

**目标**：可视化展示推理与工具调用过程，兼容推理模型。

*   **推理字段兼容**：解析流式响应中的 `reasoning_content` 字段
*   **TUI 可视化**：使用 Bubble Tea 实现轻量的可视化输出
*   **流式输出与 UI 协程**：通过 channel 将增量消息传递给 UI
*   **推理模型 vs 非推理模型**（可选）：Chat Template 差异、Token 消耗特点

### 第四章：让 Agent 接入 MCP 生态

**目标**：接入 MCP（Model Context Protocol）工具生态，扩展 Agent 能力。

*   **MCP 原理**：Client/Server/Tool 三类角色，解决"模型 × 工具"爆炸组合问题
*   **协议交互流程**（可选）：JSON-RPC 2.0、初始化、工具发现、工具调用
*   **工具管理**：本地工具与 MCP 工具的无缝共存
*   **命名空间化**：避免工具冲突的命名策略

### 第五章：上下文工程（Context Engineering）

**目标**：实现完整的上下文管理策略，在有限窗口内高效运行。

*   **上下文窗口问题**：理解多轮对话后的窗口限制、成本增加、性能下降
*   **截断策略（Truncation）**：安全删除旧消息，保留最近关键对话
*   **卸载策略（Offloading）**：将长消息存储到外部，保留恢复提示
*   **摘要策略（Summarization）**：使用 LLM 对历史对话进行压缩
*   **策略模式设计**：可扩展、可组合的策略接口
*   **策略事件系统**：在 TUI 中实时展示策略执行状态
*   **Token 计数**：使用 tiktoken-go 实时计算上下文 token 数量

### 第六章：记忆机制（Memory System）

**目标**：让 Agent 具备长期记忆能力，跨越会话保持上下文连贯性。

*   **两层记忆架构**：Global Memory（跨会话）、Workspace Memory（项目级）
*   **Memory 接口设计**：可插拔的记忆抽象接口
*   **LLM 驱动的记忆更新**：使用 LLM 自动提取和更新记忆内容
*   **持久化存储机制**：将 Global 和 Workspace 记忆持久化到文件系统
*   **记忆事件系统**：在 TUI 中实时展示记忆更新状态
*   **System Prompt 集成**：将记忆注入到 System Prompt 中供 LLM 使用

### 第七章：Agentic RAG（检索增强生成）

**目标**：让 Agent 能够自主决策检索时机、查询内容和数量，实现真正的 Agentic RAG。

*   **Agentic RAG 概念**：理解 Agent 如何自主决定何时检索、检索什么、检索多少
*   **代码索引（Embedding）**：使用 Embedding 模型将代码片段向量化
*   **向量存储**：使用 pgvector 存储和检索向量，支持增量更新和去重
*   **文档切片策略**：LineChunker 和 ParagraphChunker 两种切分方式
*   **召回（Recall）**：基于向量相似度的语义检索
*   **重排序（Rerank）**：使用 Rerank 模型对召回结果重新排序
*   **搜索工具实现**：Semantic Search Tool 的完整实现

### 第八章：沙盒与安全防御（Guardrails）

**目标**：为 Agent 设计安全防护，防止 AI 执行危险操作。

*   **Docker 沙盒隔离**：在容器中运行 bash 命令，自动检测 Docker 并优雅降级
*   **人工确认（Human-in-the-loop）**：在执行危险命令前请求用户确认
*   **确认状态管理**：Allow/Reject/Always Allow 三种确认选项
*   **Context 取消机制**：用户按 ESC 提前终止 Agent Loop

### 第九章：Agent 技能插件（Skills）

**目标**：让 Agent 学会在不同场景下使用工具的最佳实践。

*   **技能即提示**：技能是描述性文字，指导模型如何使用工具
*   **渐进式加载**：元数据注入 system prompt，完整内容按需加载
*   **Markdown 技能文件**：使用 YAML front matter + Markdown 正文定义技能
*   **技能管理器**：自动发现和加载技能元数据
*   **load_skill 工具**：LLM 按需加载完整技能内容

### 第十章：Web 服务化与 SSE 流式传输

**目标**：将 CLI Agent 剥离 TUI，封装成 HTTP 服务，让浏览器也能像 ChatGPT 一样与 Agent 流式对话。

*   **Agent 与传输层解耦**：Agent 内部用 `StreamEvent` channel 输出事件，不感知任何 HTTP 概念；HTTP 层负责把事件转成 SSE 格式推给客户端
*   **SSE 流式传输**：Server-Sent Events 服务端实现，Goroutine + Channel 的并发架构
*   **会话与消息持久化**：SQLite 存储 Conversation / ChatMessage，`Rounds` 字段序列化完整 LLM 消息
*   **树形对话历史**：每条消息记录 `parent_message_id`，`buildHistory` 沿祖先链追溯重建 LLM context

### 第十一章：Agent 可观测性（Observability）🚧

**目标**：让 Agent 的每次执行都透明、可追溯，从「能用」走向「可运维」。

*   **Trace/Span 模型**：一次 Query = 一条 Trace，一次 Tool Call = 一个 Span，构建完整的执行调用树
*   **Agent 专属指标**：First Token Time（首 Token 延迟）、Tokens Per Minute（生成速度）、Token Usage（消耗统计）
*   **Span 属性设计**：为 LLM 调用、工具执行等操作设计有意义的可查询属性
*   **可观测性后端**：OpenTelemetry + Jaeger/Tempo + Prometheus + Grafana 完整链路

---

### 第十二章：显式规划（TodoWrite 与 Nag Reminder）

**目标**：让 Agent 在多步骤任务里显式维护当前计划，并在 Web 界面里可视化展示执行进度。

*   **TodoWrite 工具**：让模型将当前任务计划写成结构化 todo 列表
*   **Nag Reminder**：当 Agent 忘记写计划或长期不更新计划时，注入轻量提醒
*   **最小状态机**：`pending / in_progress / completed` 三态足够表达当前进度
*   **计划快照持久化**：将 todo 快照存入消息历史，支持刷新恢复与分支对话
*   **Web 计划面板**：在聊天界面右侧展示当前 todo 列表

---

### 第十三章：Agent 评测与自动化测试（LLM Eval）🚧

**目标**：摒弃"靠肉眼看效果"的黑盒测试，构建自动化评测流水线。

*   **Mock LLM 接口**：测试核心调度逻辑，避免真实 API 调用
*   **评测数据集**：构建标准测试用例
*   **自动化评测指标**：响应时间、Token 消耗、准确率等
*   **Prompt 优化量化**：对比不同 Prompt 版本的实际收益

---

## 🚧 进行中与规划中

以上标注 🚧 的章节为规划中内容，正在持续更新中。敬请期待！

---

## 🛠 开发环境准备

1.  **安装 Go:** 确保本地已安装 Go 1.24 或更高版本。
2.  **获取 API Key:** 你需要一个 LLM 供应商的 API Key（如 OpenAI, 或国内的 DeepSeek/GLM）。
3.  **配置文件:**
    ```bash
    cp .env.example .env
    # 编辑 .env 文件，填入你的 API_KEY
    ```

---

## 🎯 为什么选择 Go 开发 Agent？

*   **语法简单:** Go 简单的语法和直观的代码流程，能清晰描述这个教学项目的原理与实现。
*   **类型安全:** 在处理复杂的 Tool 定义和 JSON 解析时，Go 的强类型能减少 80% 的运行时错误。
*   **并发优势:** Agent 往往需要并行调用多个工具或检索源，Go 的 Goroutine 是天然的利器。
*   **部署简单:** 无需复杂的 Python/Node.js 依赖环境，单个二进制文件即可上线。

---

## 📂 项目结构说明

```
baby-agent/
├── ch01/           # ✅ 第一章：初识 LLM（Raw HTTP 与 SDK）
├── ch02/           # ✅ 第二章：Function Calling 与 Agent Loop
├── ch03/           # ✅ 第三章：推理展示与 TUI 可视化
├── ch04/           # ✅ 第四章：MCP 生态接入
├── ch05/           # ✅ 第五章：上下文工程
├── ch06/           # ✅ 第六章：记忆机制
├── ch07/           # ✅ 第七章：Agentic RAG
├── ch08/           # ✅ 第八章：沙盒与安全防御
├── ch09/           # ✅ 第九章：Agent 技能插件
├── ch10/           # ✅ 第十章：Web 服务化与 SSE 流式传输
├── ch11/           # 🚧 第十一章：Agent 可观测性（规划中）
├── ch12/           # ✅ 第十二章：显式规划（TodoWrite 与 Nag Reminder）
├── shared/         # 共享代码（配置、MCP 等）
├── .env            # 环境变量配置
└── README.md       # 本文件
```

---

## 🚀 快速开始

各章节均可独立运行，按需学习：

```bash
# 第一章：体验流式输出
go run ./ch01/main --stream -q "用 Go 语言写一个 Hello World"

# 第二章：工具调用示例
go run ./ch02/main -q "请读取 README.md 并总结项目目标"

# 第三章：TUI 可视化
go run ./ch03/main

# 第四章：MCP 工具调用
go run ./ch04/main

# 第五章：上下文管理
go run ./ch05/main

# 第六章：记忆系统
go run ./ch06/main

# 第八章：安全防御
go run ./ch08/main

# 第九章：技能插件
go run ./ch09/main

# 第十章：Web 服务（监听 :8080）
go run ./ch10/main

# 第十二章：显式规划 Web 服务（监听 :8080）
go run ./ch12/main
```

第七章是独立的索引和工具实现，可参考 `ch07/README.md` 中的使用示例。

---

## 📚 学习建议

1.  **循序渐进**：按章节顺序学习，每章建立在前一章的基础上
2.  **动手实践**：运行每个章节的示例代码，观察实际效果
3.  **可选内容**：标记"（可选阅读/可选阅读）"的小节是进阶内容，初学可跳过
4.  **原理理解**：理解 Chat Template、Token 机制、MCP 协议等底层原理有助于排查问题

---

## 🤝 参与贡献

我们非常欢迎社区贡献！如果你有更好的 Agent 设计模式或有趣的工具实现，请随时提交 PR。

## 📄 开源协议

本项目采用 [Apache License 2.0](LICENSE) 协议。

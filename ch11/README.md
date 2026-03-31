# 第十一章：Agent 可观测性（Observability）

欢迎来到第十一章！在第十章中，我们把 Agent 服务化，让它能够通过 HTTP 接口接受请求、流式返回结果。

但随之而来一个新问题：**服务跑起来了，我怎么知道它跑得好不好？**

- 用户问了一个问题，Agent 在内部经历了什么？
- 某次回复为什么这么慢？是 LLM 慢、还是某个工具调用卡住了？
- 上个月总共消耗了多少 token？费用是多少？
- 第一个 token 多久才出来？用户体验是否达标？
- 服务当前有多少并发请求？能支撑多少 QPS？

这些问题，光靠日志打印是回答不了的。本章引入**可观测性（Observability）**，系统地建立对 Agent 服务的监控能力。

> **本章的可观测性分为三个层次**：
> - **通用服务指标**：QPS、并发、Duration——任何后端服务都需要的基础健康指标
> - **LLM Agent 专属指标**：First Token Time、Tokens Per Second、Token Usage——Agent 独有的性能与成本指标
> - **Agent 执行轨迹**：思维链、工具调用、上下文压缩、记忆更新的完整过程记录——Agent 区别于普通服务的核心可观测维度

---

## 🎯 你将学到什么

1. **为什么 Agent 的通用指标需要特殊对待**：Duration 动辄数十秒，颠覆了传统后端的 QPS/并发/Duration 换算直觉
2. **Agent 专属指标**：First Token Time、Tokens Per Second、Token Usage 的定义、采集方式与典型基线
3. **Agent 执行轨迹**：什么是 Agent 轨迹，它与云原生 Trace 的区别，以及如何设计轨迹数据结构
4. **可观测性工具栈**：OpenTelemetry + Jaeger + Prometheus + Grafana 的分工与部署

---

## ⏱ Agent 与传统后端的最大区别：Duration

在深入各类指标之前，有一个根本性的区别必须先理解。

传统后端服务的请求 Duration 通常在**毫秒级**：一个 REST API 响应 50ms，一个数据库查询 10ms。

**Agent 服务的 Duration 是秒到分钟级**：一次包含工具调用的 Agent 请求，轻则 10-30s，重则数分钟。

这个差异不仅仅是数量级的区别，它从根本上改变了 QPS、并发数、Duration 三者之间的关系。

### QPS、并发数、Duration 的换算

根据利特尔定律（Little's Law）：

```
并发数（Concurrency） = QPS × Duration
```

对于传统后端（Duration = 0.05s）：
```
并发数 = 100 QPS × 0.05s = 5
```
100 QPS 的服务，同时只需处理 5 个并发请求，服务器压力很轻。

对于 Agent 服务（Duration = 30s）：
```
并发数 = 10 QPS × 30s = 300
```
只有 10 QPS 的 Agent 服务，同时需要维持 300 个并发请求的状态！每个请求都在等待 LLM 流式返回、执行工具、积累上下文……

**这意味着**：

| 对比项 | 传统后端 | Agent 服务 |
|--------|---------|-----------|
| 典型 Duration | 10-200ms | 10-120s |
| 10 QPS 对应并发 | 0.1-2 个 | 100-1200 个 |
| 主要瓶颈 | CPU、DB 连接 | 内存（维护请求状态）、LLM 并发配额 |
| 扩容方向 | 横向扩机器 | 提高 LLM API 并发限额、优化内存 |
| Duration 告警阈值 | > 500ms 告警 | > 120s 才算异常 |

> **实践含义**：如果你用传统后端的思维来评估 Agent 服务——看到 QPS 只有 5，就以为服务很空闲——那你就完全误判了。5 QPS 的 Agent 服务，可能同时有 150 个请求在飞，内存和 LLM API 配额都在高负荷运行。

### 推导公式总结

```
# 根据 QPS 和 Duration 估算所需并发容量
所需并发数 = QPS × 平均 Duration(s)

# 根据并发上限反推最大 QPS
最大 QPS = 并发上限 / 平均 Duration(s)

# 示例：LLM API 并发上限 50，平均 Duration 20s
最大 QPS = 50 / 20 = 2.5 QPS
```

---

## 📊 第一类：通用服务指标

这一类指标是**所有 HTTP 服务都需要监控的基础健康指标**，Agent 服务也不例外。但结合上面的分析，在阈值设定和告警策略上需要针对 Agent 的特点做调整。

### 1. QPS（每秒请求数）

**定义**：每秒钟收到的新 Agent 请求数量。

**采集**：在 HTTP 中间件层对每个进入的请求计数。

```
QPS = 单位时间内的请求数 / 时间窗口(s)
```

**告警建议**：
- QPS 突然归零 → 服务可能已挂
- QPS 突然暴增 → 可能被异常调用，检查是否有客户端 bug 或攻击

**Agent 特殊性**：Agent 服务的 QPS 通常很低（个位数到两位数），不要被低 QPS 迷惑，此时并发数可能已经很高。

### 2. 当前并发数（In-flight Requests）

**定义**：当前正在处理中（尚未返回）的请求数量。

**采集**：使用计数器，请求进入时 +1，请求完成时 -1。

```
当前并发数 = 进入请求总数 - 完成请求总数
```

**为什么 Agent 服务要重点监控并发数**：

Agent 服务的每个进行中请求都在消耗资源：
- 内存：维护 LLM 上下文、工具执行状态、流式缓冲区
- LLM API 配额：许多 LLM 服务商对并发调用有上限（如 Anthropic 的 Tier 限制）
- 网络连接：长时间的 SSE 连接

当并发数接近 LLM API 的并发配额上限时，新请求会开始排队，First Token Time 急剧上升。

**告警建议**：设置并发数上限告警，而非 QPS 告警。

### 3. Duration（请求持续时间）

**定义**：从收到请求到完整返回响应的总时间。

**Agent 服务 Duration 的构成**：

```
总 Duration = Σ(LLM 调用时间) + Σ(工具执行时间) + 上下文构建时间 + 内存更新时间
```

**监控建议**：

对于 Agent 服务，更有意义的是**分 Percentile 监控**：

| Percentile | 含义 | Agent 典型值 |
|-----------|------|------------|
| P50（中位数） | 一半请求比这快 | 15-30s |
| P95 | 95% 的请求比这快 | 60-90s |
| P99 | 99% 的请求比这快 | 120-180s |

> 不要用平均值（Mean）来衡量 Duration。Agent 的 Duration 分布极度不均匀（简单问题 5s，复杂任务 3 分钟），平均值会掩盖大量慢请求。

**错误率**：请求 Duration 内发生的错误（LLM API 失败、工具执行出错、超时）需要单独统计和分类。

---

## 🤖 第二类：LLM Agent 专属指标

这一类指标是 **Agent 服务独有的**，传统 APM 工具无法直接采集，需要在 Agent 内部主动埋点上报。

### 1. First Token Time（首 Token 延迟）

**定义**：从发送 LLM 请求到收到第一个流式 token 的时间。

**为什么是最重要的用户体验指标**：

对于流式接口，用户感受到的「响应速度」不是总耗时，而是**多久开始看到内容**。屏幕上出现第一个字符的那一刻，用户的焦虑立刻消散。

```
用户发送请求
    │
    │  ← First Token Time（关键！用户焦虑期，屏幕一片空白）
    │
收到第一个 token（"好"）→ 用户看到内容开始出现，焦虑消失
    │
    │  ← Streaming Duration（内容陆续出现，体验良好）
    │
收到最后一个 token（完整回复）
```

**采集方式**：

```
First Token Time = 收到第一个非空 chunk 的时间戳 - 发送请求的时间戳
```

在 LLM 流式调用中，记录发送请求前的时间戳，然后在第一次收到有内容的 chunk 时记录另一个时间戳，两者相减。

**典型基线**：

| 范围 | 用户体验 | 建议行动 |
|------|---------|---------|
| < 1s | 优秀，几乎无感知等待 | 保持 |
| 1-3s | 可接受 | 监控趋势 |
| 3-5s | 用户开始焦虑 | 加 Loading 动画，排查排队问题 |
| > 5s | 用户体验严重受损 | 紧急排查，考虑降级策略 |

**影响 First Token Time 的因素**：
- LLM 服务商的排队延迟（并发请求多时）
- 网络往返时间
- 模型推理到第一个 token 的时间（与 prompt 长度正相关）

### 2. Tokens Per Second（生成速度）

**定义**：LLM 在流式输出阶段，每秒生成的 output token 数量（也称 TPS，Token Generation Speed）。

> **注意区分 TPS 和 TPM**：Tokens Per Minute（TPM）通常是 LLM 服务商**限流窗口**的计量单位（如 Anthropic、OpenAI 的 Rate Limit 以 TPM 为单位），它统计的是一分钟内总消耗的 token 数（包括 prompt + completion），用于控制 API 配额。而我们这里的 Tokens Per Second 是衡量**单次请求流式生成速度**的体验指标，两者含义完全不同，不要混淆。

**计算公式**：

```
Tokens Per Second = completion_tokens / streaming_duration(s)
```

**为什么要区分 First Token Time 和 Tokens Per Second**：

当用户反映「回复很慢」时，需要区分两种不同的慢：

```
情况 A：First Token Time 长（等了 8s 才出现第一个字）
        → 原因：LLM API 排队、网络问题、prompt 过长
        → 解法：减少 prompt 长度、切换更快的模型、增加 API 配额

情况 B：Tokens Per Second 低（内容出现了，但一字一顿很慢）
        → 原因：模型本身吞吐低、服务端限速、长 completion
        → 解法：切换吞吐更高的模型、开启 batch streaming
```

**典型参考值**（随模型和负载变化）：

| 模型 | 典型 Tokens/s |
|------|-------------|
| GPT-4o | 33-67 |
| Claude Sonnet 4.6 | 50-83 |
| Claude Haiku | 83-133 |
| 本地小模型（7B） | 8-25 |

### 3. Token Usage（Token 消耗）

**定义**：每次 LLM 调用消耗的 prompt token 和 completion token 数量。

**为什么重要**：

Token 消耗直接等于费用。Agent 的上下文是累积的——每轮对话后，prompt 包含越来越多的历史消息，token 消耗呈**阶梯式增长**：

```
第1轮：prompt=500, completion=200
第2轮：prompt=700（含第1轮历史）, completion=300
第3轮：prompt=1100（含前两轮历史）, completion=400
...
第10轮：prompt=5000+, completion=500
```

不监控 token，费用很容易在没有任何告警的情况下失控。

**分维度统计**：

| 维度 | 用途 |
|------|------|
| 按 conversation_id | 定位「超长对话」，发现上下文管理问题 |
| 按 user_id | 发现异常使用行为 |
| 按 model | 对比不同模型的成本效益 |
| 按时间段 | 估算月度账单，做容量规划 |
| prompt vs completion 分开统计 | 判断 token 主要花在了输入还是输出 |

**费用估算公式**：

```
每次请求费用 = prompt_tokens × 输入单价 + completion_tokens × 输出单价

月度费用估算 = 日均请求数 × 30 × 每次请求平均费用
```

### 4. 工具调用统计

**定义**：各类工具的调用次数、成功率、耗时分布。

**关键指标**：
- 各工具调用次数占比
- 各工具失败率
- 各工具 P95 耗时

**常见问题信号**：

| 现象 | 可能原因 |
|------|---------|
| 某工具失败率 > 10% | 工具实现有 bug，或外部依赖不稳定 |
| bash 工具 P95 耗时 > 30s | 脚本有死循环或阻塞操作 |
| 同一工具在一次请求中被调用 > 5 次 | LLM 在「绕圈」，prompt 设计问题 |

### 5. Agent Loop 深度

**定义**：一次 Query 中，LLM 与工具交互的轮次数（即完整的「LLM 调用→工具执行」循环次数）。

**为什么重要**：

Loop 深度直接决定请求的总 Duration 和总 token 消耗。Loop 深度 = 1 意味着 LLM 直接回复；Loop 深度 = 5 意味着经历了 4 轮工具调用。

异常高的 Loop 深度（> 8）通常意味着：
- LLM 没有有效推进任务，在反复尝试同一个工具
- 工具返回结果不够清晰，LLM 看不懂
- 任务本身超出了 Agent 的能力边界

---

## 🔍 第三类：Agent 执行轨迹

这是 Agent 可观测性中**最独特的部分**，也是最容易和云原生概念混淆的地方。

### 什么是 Agent 执行轨迹？

> **注意区分两个「Trace」**：
>
> - **云原生的 Trace（分布式链路追踪）**：记录一个请求在多个微服务之间的调用链路，用 Span 描述每段操作，关注延迟和错误传播。
>
> - **Agent 执行轨迹**：记录一个 Agent 请求的完整「思维-行动」过程——LLM 想了什么、调了什么工具、上下文怎么压缩的、记忆是否更新了。这是 Agent **业务语义层面**的追踪，不只是时间维度。

Agent 执行轨迹要回答的核心问题是：**这次请求里 Agent 做了什么决策，为什么这么做？**

### 轨迹的组成

一次完整的 Agent 执行轨迹，包含以下类型的事件，按时间顺序记录：

```
[轨迹开始] query: "帮我写一个 Go HTTP 服务并运行测试"
    │
    ├── [事件 1] 上下文构建
    │     · 历史消息数: 6 条
    │     · 当前 token 数: 2048 / 8192 (25%)
    │     · 触发策略: 无（未超阈值）
    │
    ├── [事件 2] LLM 思考（第 1 轮）
    │     · 推理内容: "用户需要一个 HTTP 服务...我先写代码，然后运行测试..."
    │     · 决策: 调用 bash 工具写文件
    │     · prompt_tokens: 1024, completion_tokens: 312
    │
    ├── [事件 3] 工具调用: bash
    │     · 命令: "cat > main.go << 'EOF'..."
    │     · 状态: 成功, exit_code: 0
    │     · 耗时: 0.3s
    │
    ├── [事件 4] LLM 思考（第 2 轮）
    │     · 推理内容: "文件写好了，现在运行测试..."
    │     · 决策: 调用 bash 工具运行测试
    │     · prompt_tokens: 1536, completion_tokens: 89
    │
    ├── [事件 5] 工具调用: bash
    │     · 命令: "go test ./..."
    │     · 状态: 成功, exit_code: 0
    │     · 耗时: 18.4s
    │     · 输出摘要: "ok  	main	18.392s"
    │
    ├── [事件 6] LLM 最终回复（第 3 轮）
    │     · 内容摘要: "我已经完成了...包含路由、处理器..."
    │     · prompt_tokens: 2048, completion_tokens: 891
    │
    ├── [事件 7] 上下文策略执行
    │     · token 使用: 3891 / 8192 (47%)
    │     · 触发策略: 无
    │
    └── [事件 8] 记忆更新
          · 提取到新事实: 2 条
          · "用户使用 Go 开发 HTTP 服务"
          · "用户需要同时写代码和测试"
[轨迹结束] 总耗时: 47s, 总 tokens: 4939
```

### 轨迹事件类型

| 事件类型 | 触发时机 | 核心记录内容 |
|---------|---------|------------|
| `query_start` | 收到用户请求 | 请求摘要、conversation_id、历史轮次数 |
| `context_build` | 构建 LLM 上下文时 | 当前 token 数、消息数、上下文使用率 |
| `llm_thinking` | LLM 流式返回完成时 | 推理内容摘要、决策（回复/调用工具）、token 消耗 |
| `tool_call` | 工具执行前后 | 工具名、输入摘要、输出摘要、状态、耗时 |
| `context_policy` | 上下文策略触发时 | 策略名（truncate/offload/summarize）、操作前后 token 数 |
| `memory_update` | 记忆更新时 | 提取的事实数量、更新的记忆类型（全局/工作区） |
| `query_end` | 请求完成时 | 总耗时、总 token、loop 深度、最终状态 |

### 轨迹与云原生 Trace 的关系

两者可以共存，各有侧重：

| 维度 | 云原生 Trace（OTel/Span） | Agent 执行轨迹 |
|------|--------------------------|--------------|
| **关注点** | 时间：哪步慢了？ | 决策：Agent 怎么思考的？ |
| **存储位置** | Jaeger / Tempo | 数据库 / 日志系统 |
| **查询方式** | 按 trace_id、按延迟 | 按 conversation、按工具、按策略 |
| **主要受益者** | SRE、运维工程师 | AI 工程师、产品经理 |
| **数据量** | 结构化，较轻量 | 半结构化，包含文本内容，较重 |

**实践建议**：将两者关联——在 Agent 轨迹事件中记录对应的 `span_id`，在云原生 Span 的属性中记录 `agent_event_id`，这样可以在 Jaeger 里看到时序视图，在轨迹系统里看到语义视图，互相跳转。

### 轨迹的数据模型

```go
// AgentTrace 一次 Agent 请求的完整执行轨迹
type AgentTrace struct {
    TraceID        string       // 与云原生 trace_id 关联
    ConversationID string
    MessageID      string
    Query          string       // 用户原始请求（注意脱敏策略）
    StartTime      time.Time
    EndTime        time.Time
    Status         string       // ok | error | cancelled
    Events         []TraceEvent // 按时间顺序的事件列表
    Summary        TraceSummary // 汇总统计
}

// TraceEvent 轨迹中的单个事件
type TraceEvent struct {
    EventID   string
    Type      string    // query_start | llm_thinking | tool_call | context_policy | memory_update | query_end
    Timestamp time.Time
    Duration  time.Duration
    Attrs     map[string]any // 事件特有属性
}

// TraceSummary 轨迹汇总
type TraceSummary struct {
    TotalDuration      time.Duration
    LoopDepth          int
    LLMCallCount       int
    ToolCallCount      int
    TotalPromptTokens  int
    TotalCompletTokens int
    ContextPolicies    []string // 触发了哪些策略
    MemoryUpdated      bool
}
```

---

## 🗂 可观测性数据分类汇总

| 类别 | 指标 | 存储位置 | 查询场景 |
|------|------|---------|---------|
| **通用服务** | QPS、并发数、Duration、错误率 | Prometheus + Grafana | 服务健康监控、容量规划 |
| **LLM 专属** | First Token Time、Tokens/s、Token Usage | Prometheus（指标） + Span属性 | 体验优化、成本控制 |
| **Agent 轨迹** | 思维链、工具序列、策略触发、记忆更新 | 数据库 / 结构化日志 | 行为分析、问题复现、质量改进 |
| **分布式追踪** | Span 树、各步骤延迟 | Jaeger / Tempo | 性能瓶颈定位 |

---

## 🏗 可观测性架构

### 数据流向

```
Agent 代码
  │
  ├── 业务指标上报 ──────────────────► Prometheus ──► Grafana（指标看板）
  │   (QPS, 并发, Duration,
  │    First Token Time, Token Usage)
  │
  ├── OTel SDK（Span 埋点）──────────► OTel Collector ──► Jaeger（延迟分析）
  │   (agent.query, llm.call,
  │    tool.call, context.build)
  │
  └── 轨迹事件写入 ──────────────────► 数据库 / 日志系统（轨迹查询与回放）
      (llm_thinking, tool_call,
       context_policy, memory_update)
```

### 组件职责

| 组件 | 职责 |
|------|------|
| **OpenTelemetry SDK** | 在代码中创建 Span、记录属性、上报时序数据 |
| **OTel Collector** | 接收、处理、转发遥测数据，支持采样和批量发送 |
| **Jaeger / Tempo** | Trace 后端，按 trace_id 查询完整链路，Flame Graph 可视化 |
| **Prometheus** | Metrics 后端，QPS/并发/延迟等指标的时序聚合与告警 |
| **Grafana** | 统一看板，整合 Trace 和 Metrics |
| **数据库（SQLite/PostgreSQL）** | 存储 Agent 执行轨迹，支持按 conversation、工具、策略查询 |

### 本地开发环境（Docker Compose）

```yaml
services:
  otel-collector:
    image: otel/opentelemetry-collector-contrib
    ports:
      - "4317:4317"   # OTLP gRPC
      - "4318:4318"   # OTLP HTTP

  jaeger:
    image: jaegertracing/all-in-one
    ports:
      - "16686:16686" # Jaeger UI

  prometheus:
    image: prom/prometheus
    ports:
      - "9090:9090"

  grafana:
    image: grafana/grafana
    ports:
      - "3000:3000"
```

---

## 🔍 从数据到洞察：典型场景

### 场景一：定位慢请求

用户反馈某次回复很慢。结合两种追踪数据：

**第一步：Jaeger 定位时间瓶颈**

```
[agent.query]  ████████████████████████████████████ 47s
  context.build  ▌ 12ms
  llm.call #1    ████████ 8s
  tool.call bash ████████████████████ 20s  ← 耗时最长
  llm.call #2    ████████████ 15s
  memory.update  ██ 2s
```

**第二步：Agent 轨迹定位业务原因**

打开对应 conversation 的轨迹，找到 `tool_call` 事件，查看工具输入：

```
[tool_call] bash
  命令: "curl -L https://example.com/large-file.tar.gz -o /tmp/archive.tar.gz"
  耗时: 20.1s
  输出: "...正在下载，已完成 87%..."
```

原因找到：Agent 在下载大文件。可以考虑限制工具超时时间或提示 LLM 避免下载大文件。

### 场景二：分析 Token 费用异常

Grafana 告警：今日 Token 消耗是昨日的 3 倍。

**第一步：按 conversation 分组**

```promql
topk(5, sum by (conversation_id) (agent_prompt_tokens_total))
```

发现 conversation `abc-123` 消耗了异常多的 token。

**第二步：查看该 conversation 的轨迹**

轨迹中发现上下文策略事件：
```
[context_policy] 未触发任何策略
  · 当前 token: 78000 / 128000 (60%)
  · 历史消息: 142 条
```

原来这个 conversation 已经进行了 71 轮对话，但截断策略的触发阈值设置为 80%，还没触发。将阈值调低到 60% 后，成本恢复正常。

### 场景三：发现 Agent 行为异常

First Token Time P95 从 1.2s 上升到 8s，但工具调用耗时没变。

查看 Agent 轨迹，发现每次 `context_build` 事件的 token 数都在快速增长：

```
[context_build]
  · 当前 token: 45000 / 128000 (35%)
  · 消息数: 89 条
```

原来 offload 策略将长消息卸载到外部，但每次构建上下文时都需要读取这些卸载内容，导致 prompt 急剧膨胀。优化策略后 First Token Time 恢复正常。

---

## 🧱 埋点设计原则

### 1. 敏感数据脱敏

轨迹数据会持久化存储，注意：
- **记录长度，不记录原文**：`query_length: 128` 而非 `query: "..."`
- **工具输出只记录摘要**：前 200 字符或关键字段（exit_code、文件名）
- **推理内容可以记录**：LLM 的思考过程通常不含用户敏感数据，且对分析非常有价值

### 2. 采样策略

不是每个请求都需要记录完整轨迹，高流量时按需采样：

- **错误请求**：100% 记录（用于复现和排查）
- **慢请求**（Duration > P95）：100% 记录
- **正常请求**：按 10-20% 采样

### 3. 轨迹保留期

| 数据类型 | 建议保留期 |
|---------|---------|
| Prometheus 指标 | 30 天 |
| Jaeger Trace | 7 天 |
| Agent 执行轨迹 | 90 天（用于回溯分析）|
| 错误和慢请求轨迹 | 180 天 |

---

## 📐 可观测性与评测的关系

可观测性（本章）和评测（下一章）是相辅相成的：

| | 可观测性 | 评测 |
|--|---------|------|
| **关注点** | 线上系统的实时运行状态 | 离线的质量基准 |
| **数据来源** | 生产流量 | 构造的测试用例 |
| **回答的问题** | 「现在跑得怎么样？」 | 「改了代码后质量有没有下降？」 |

**Agent 轨迹是评测数据的天然来源**：

```
线上轨迹发现：某类问题 loop 深度总是 > 8，工具失败率高
      ↓
把这类问题 + 其轨迹提取出来，加入评测集
      ↓
优化 prompt 或工具实现后，先通过评测验证 loop 深度下降
      ↓
再上线，通过可观测性验证线上效果
```

---

## 📖 代码结构预览

```
ch11/
├── agent/
│   ├── agent.go          # Agent 核心，RunStreaming 内部记录轨迹事件
│   ├── stream.go         # StreamEvent 类型
│   └── tool/
│       ├── tool.go
│       └── bash.go
├── observe/
│   ├── tracer.go         # OTel Tracer 初始化，配置 Jaeger exporter
│   ├── span.go           # Span 工厂：StartAgentSpan、StartLLMSpan、StartToolSpan
│   ├── metrics.go        # Prometheus 指标定义：QPS、并发、Duration、Token 指标
│   ├── middleware.go     # Gin 中间件：并发计数、请求计时、trace_id 注入
│   └── trace.go          # Agent 执行轨迹：事件类型、写入、查询
├── server/
│   ├── db.go             # 数据模型，新增 AgentTrace 表
│   ├── history.go
│   ├── service.go        # CreateMessage 中启动 Trace、记录轨迹事件
│   └── controller.go     # 新增轨迹查询接口
├── vo/
│   ├── vo.go
│   └── sse.go
└── main/
    └── main.go           # 初始化 OTel Provider、Prometheus handler
```

---

## 与前几章的关系

ch11 在 ch10 服务化的基础上，专注解决**透明度**这一工程问题。从用户视角看，Agent 的行为不变；从运维视角看，每次 Agent 执行都有了完整的可追溯记录。

```
ch09  Agent 能力（工具、记忆、技能）
  ↓
ch10  服务化（HTTP、SSE、持久化）
  ↓
ch11  可观测性（通用指标 + LLM 专属指标 + 执行轨迹）← 当前章节
  ↓
ch12  评测（离线质量保障，使用轨迹数据构建测试集）
```

可观测性是 Agent 从「能用」走向「可运维、可改进」的关键一步。只有当你能清楚地看到 Agent 在做什么，才有可能系统地改进它。

---

## 📚 扩展阅读

### 可观测性基础

- [OpenTelemetry 官方文档](https://opentelemetry.io/docs/) — OTel 的核心概念、SDK 使用指南与各语言示例
- [Google SRE Book - Monitoring Distributed Systems](https://sre.google/sre-book/monitoring-distributed-systems/) — 工程师必读，讲清楚了「四个黄金信号」（延迟、流量、错误、饱和度）

### LLM 可观测性

- [Anthropic Rate Limits 文档](https://docs.anthropic.com/en/api/rate-limits) — 理解 TPM（Tokens Per Minute）限流窗口的官方说明
- [Grafana LLM Observability](https://grafana.com/solutions/llm-observability/) — Grafana 官方的 LLM 可观测性方案，含开箱即用的 Dashboard 模板
- [Langfuse](https://langfuse.com/) — 专为 LLM 应用设计的开源可观测性平台，支持 Trace、Evaluation、Prompt Management

### 工具与框架

- [Jaeger 官方文档](https://www.jaegertracing.io/docs/) — 分布式链路追踪后端，支持 Flame Graph 和 Trace 对比
- [Prometheus Best Practices](https://prometheus.io/docs/practices/naming/) — Metrics 命名规范，写出好查询的基础
- [OpenTelemetry Go SDK](https://github.com/open-telemetry/opentelemetry-go) — Go 语言 OTel SDK，本章代码实现的依赖库
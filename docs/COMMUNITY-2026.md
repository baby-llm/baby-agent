# 社区近期动态（2026-05 → 2026-11）

本页只收录 **2026-05-01 至 2026-11-02** 期间公开发布、且能确认日期的更新。更早的经典资料仍放在各章“扩展阅读”里，不混入本页。

用途：给本教程的原理章节补上“社区现在怎么做”的参照，并明确哪些是本仓库已实现、哪些只是方向。

---

## ch04 / ch10：MCP 与流式传输

| 日期 | 更新 | 对本仓库的含义 |
|---|---|---|
| 2026-07-28 | [MCP 规范 2026-07-28 修订](https://modelcontextprotocol.io/specification/2026-07-28/changelog)：移除协议 session 与 `Mcp-Session-Id`、移除 GET 流、移除 SSE resumability（`Last-Event-ID`/event id），改为无状态请求 + `subscriptions/listen` | 本仓库 ch04 基于 go-sdk v1.5.0 的旧版 Streamable HTTP；升级到实现该修订的 SDK（如 go-sdk v1.7.0）时，不能再假设可恢复的 session/SSE。ch10 的“断流不 resume、客户端重新发起”正好符合新方向 |
| 2026-06-25 | [Vercel AI SDK 7](https://vercel.com/changelog/ai-sdk-7)：工具审批（可选 HMAC 签名）、durable `WorkflowAgent`、分层超时、MCP Apps、`@ai-sdk/otel` | ch10 目前只有 SSE + SQLite，没有 durable run、审批签名与分层超时；这些是下一步可补齐的工程能力 |

**落地提示**

- MCP 2026-07-28 之后，`Mcp-Session-Id`、`Last-Event-ID` 不再是可依赖的恢复机制；客户端必须在断流后重新发起新请求。
- 本仓库的 `shared.McpServerConfig` 已支持 `headers` 占位符替换；HTTP MCP server 的鉴权头可直接写在配置里。
- 升级 go-sdk 前先确认 `CommandTransport`/`StreamableClientTransport` 的 API 变化，并重新跑 ch04 的 MCP 示例。

---

## ch05 / ch06：上下文工程与记忆

2026 年主流平台把“压缩/清理/记忆”从应用层策略上移到了模型 API 层：

- Anthropic 平台提供 context editing（清理旧 tool result / thinking）与 compaction（服务端摘要，返回可回放的 compaction block），以及 managed memory stores。
- 方向很明确：**应用层负责决定“保留什么”，平台负责执行压缩**；不要把摘要策略硬编码成一次性删除。

本仓库的对应关系：

- ch05 的 `Truncate / Offload / Summary` 是本地最小实现，适合理解边界条件（tool-call 配对、摘要保留、token 统计）。
- ch06 的 Global/Workspace 两层文件记忆是本地最小实现；生产环境可替换为托管 memory store，但“哪些信息值得记、何时更新”仍需要应用层判断。
- 本章修复了两个容易踩的坑：摘要边界不能切断 `assistant(tool_calls)` 与 `tool` 的配对；记忆解析失败时应保留旧值，而不是清空整层。

> 具体 beta 版本与计费以 Anthropic 官方文档为准；本页只记录方向，不把未核实的版本号写成事实。

---

## ch07：RAG 与 pgvector

| 日期 | 更新 | 对本仓库的含义 |
|---|---|---|
| 2026-06-17 / 06-30 / 07-29 | pgvector 0.8.3 / 0.8.4 / 0.8.6：修复 HNSW vacuum 相关索引损坏与内存问题 | 生产环境应使用 ≥0.8.6；本章代码的 IVFFlat 索引不受这些 HNSW 修复影响，但升级前要读 release notes |
| 2026-10-01 | [pgvector 0.8.7](https://github.com/pgvector/pgvector/releases/tag/v0.8.7)：HNSW 异步 I/O、`sparsevec` 插入性能改进 | HNSW 更适合“写入后立即查询、不想训练 lists”的场景；本章默认 IVFFlat 是为了教学简单 |
| 2026 年 | `halfvec`（16-bit）成为常见默认：内存约减半，召回损失通常 <1% | 大规模索引时可把 `vector(n)` 换成 `halfvec(n)`；本章未实现，属于扩展练习 |

**落地提示**

- pgvector 核心不提供 BM25 或混合检索；RRF 需要自己写 SQL/CTE，或使用外部搜索引擎。
- 本章的 `semantic_search` 已做“召回 TopK*2 → rerank → 截断 TopK”；生产环境建议把召回窗口放大到 100+，再 rerank 到 10-20。
- 索引器默认跳过 `.env`、`config*.json`、私钥与 `.babyagent/`；这不是安全边界，索引前仍要检查工作区。

---

## ch08：Agent 安全与 Guardrails

| 日期 | 更新 | 对本仓库的含义 |
|---|---|---|
| 2026-08-04 | [OWASP GenAI LLM Top 10 2026](https://genai.owasp.org/resource/owasp-genai-llm-top-10-2026/)：Prompt Injection 仍为 #1；Excessive Agency 升至 #3；Hidden Context Exposure 更名并扩展到 tool schema/检索内容/记忆；Vector & Embedding Weaknesses 为 #9 | 本章的 Docker 沙盒 + 人工确认是“限制爆炸半径”的最小实现，不试图检测注入。生产环境还需要最小权限、短时凭证、不可逆操作人工确认、输出端编码 |

**落地提示**

- 不要把“模型能识别恶意指令”当作安全边界；授权决策必须在模型之外。
- 工具描述、检索文档、记忆都应视为不可信输入；本章的 `load_skill` 已加路径校验，但 MCP 工具的描述仍来自外部 server。
- 对不可逆操作，确认框应展示原始动作（命令、SQL、目标路径），而不是模型生成的摘要。

---

## ch09：Agent Skills

| 日期 | 更新 | 对本仓库的含义 |
|---|---|---|
| 2026-08-19 | [agentskills.io specification](https://agentskills.io/specification)：`name` 必须 1-64 字符、小写字母/数字/连字符、不能以连字符开头/结尾或连续连字符，并与父目录名一致；渐进式披露分为 metadata → instructions → resources 三层 | 本章已实现 metadata 注入 + 按需 `load_skill` + scripts/references 发现；已按规范校验 `name` 与目录名，并拒绝路径穿越 |

**落地提示**

- 技能正文建议控制在 500 行/5000 tokens 以内，把长参考资料放到 `references/`。
- `description` 决定“何时被加载”，应写清触发场景，而不是泛泛的“帮助处理任务”。
- 多技能场景下，metadata 的 token 成本会累积；本章的 `FormatForPrompt` 只注入 ID + description。

---

## ch11：可观测性

| 日期 | 更新 | 对本仓库的含义 |
|---|---|---|
| 2026-06-12 | [OTel semantic-conventions v1.42.0](https://github.com/open-telemetry/semantic-conventions/releases/tag/v1.42.0)：`gen_ai.*` 属性/指标/事件/span 全部从核心 semconv 拆到 `semantic-conventions-genai` | 本章代码使用 `semconv/v1.37.0` 并手写 `gen_ai.*` 属性；升级时应改用 GenAI 独立模块，避免继续从核心包导入 |
| 2026-08-03 | [OTel Go v1.45.0](https://github.com/open-telemetry/opentelemetry-go/releases/tag/v1.45.0)：`WithEndpointURL` 不再自动追加默认 signal 路径 | 升级 exporter 时要检查 collector URL；本章默认 OTLP HTTP endpoint 需显式写全路径 |
| 2026-08-25 / 10-02 | OTel Go v1.46.0 / v1.47.0：v1.47 起要求 Go 1.26；v1.46 是最后支持 Go 1.25 的版本 | 本仓库 `go 1.25.0`，暂时不能升到 v1.47；先停留在 v1.46 或继续用 v1.44 |
| 2026-10-08 | [Prometheus client_golang v1.25.0](https://github.com/prometheus/client_golang/releases/tag/v1.25.0)：最低 Go 1.26 | 本仓库使用 v1.24.1 + Go 1.25；升级前先升 Go 工具链 |
| 2026-10-20 | Grafana Agent Observability GA：OTel 原生支持 agent 活动、成本与行为 | 本章的 OTel + Prometheus + Grafana 路径是官方支持方向，不需要专有 agent SDK |
| 2026-10-27 / 10-31 | GenAI semconv 增加 A2A span 约定与 TTFC（首 token 时间）说明 | 本章的 `StartLLMSpan`/`StartToolSpan` 与 `FirstToken` 直方图可对齐这些命名与分桶 |

**落地提示**

- 迁移顺序：先升级 OTel Go（注意 Go 版本与 exporter URL），再把 `gen_ai.*` 导入切到 `semantic-conventions-genai`，最后对齐 A2A/TTFC 命名。
- 本章的 `AgentTrace` 是业务语义轨迹，不能替代 OTel span；两者应通过 `trace_id` 关联，而不是二选一。

---

## ch12：评测

| 日期 | 更新 | 对本仓库的含义 |
|---|---|---|
| 2026-06-03 | [OpenAI Evals 迁移到 Promptfoo](https://developers.openai.com/cookbook/examples/evaluation/moving-from-openai-evals-to-promptfoo)：托管 Evals 逐步下线，官方给出迁移路径 | 本章“确定性 Mock + 版本化数据集”不是权宜之计，而是当前更可控的路线；托管 grader 不应成为唯一依赖 |
| 2026-05-13 / 08-09 | DeepEval 4.0 / 4.1.7：面向 coding agent 的评测、trace 检查、`flaky` 标记 | 评测报告应区分“真实回归”和“不稳定用例”；本章目前只做确定性断言，可在此基础上加 `flaky` 概念 |
| 2026-10-06 / 10-23 | promptfoo 0.124 / 0.125：provider 改为显式 opt-in、ROUGE 断言、trace span 过滤、safe mode、重复测试稳定性 | 如果后续把 ch12 接到 promptfoo，配置要显式列出 provider；稳定性报告比单次通过率更有价值 |
| 2026-08-18 | [LangSmith Tuned Evaluators](https://www.langchain.com/blog/introducing-langsmith-tuned-evaluators-starting-with-perceived-error)：托管 judge，异步、按线程评估 | LLM-as-a-Judge 的版本、延迟与成本是产品决策；本章把它列为扩展方向，不把 judge 分数当绝对事实 |

**落地提示**

- 先保护“调度逻辑”：工具是否被调用、参数是否正确、loop 是否超限；再评估“答案质量”。
- 任何 LLM judge 都要记录模型版本、prompt、原始判定和不确定性；不要把一次评分写进 CI 的硬门槛。
- 评测数据集应从 ch11 的真实 trace 中沉淀，但要去标识、去隐私，并固定版本。

---

## 维护约定

- 新增条目必须带可核实的发布日期与官方链接；日期不明或超过 6 个月的条目不要放进本页。
- 每条都要写“对本仓库的含义”，避免变成链接收藏夹。
- 如果某条更新导致代码需要迁移，在对应章 README 加 `> **迁移提示**`，不要只改本页。

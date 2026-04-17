# 第十二章：显式规划（TodoWrite 与 Nag Reminder）

欢迎来到第十二章！在第十章中，我们已经把 Agent 封装成了 Web 服务，浏览器也可以像 ChatGPT 一样和它流式对话。

但新的问题也随之出现：**一旦任务稍微复杂一点，Agent 很容易“边做边忘”，做着做着就偏题了。**

比如：

- 明明要“先读代码、再修改、最后跑测试”，Agent 却直接开始乱改文件
- 已经完成了第一步，但没有更新当前计划，后面很难判断它做到哪了
- 页面刷新之后，用户看不到刚才那轮任务的计划快照

本章要解决的就是这个问题：让 Agent 在 Web 界面里把“当前计划”显式写出来，并在计划失效时收到轻量提醒。

> **本章的核心目标不是构建完整任务系统，而是让 Agent 在当前会话里“有计划地工作”。**

---

## 🎯 你将学到什么

1. **显式规划**：为什么多步骤任务需要可见的 todo 列表
2. **`todo_write` 工具**：如何让模型主动维护当前计划
3. **Nag Reminder**：如何在不打断体验的情况下提醒 Agent 更新计划
4. **计划快照持久化**：为什么 Web 场景下不能只把 todo 放在进程内存里
5. **分支感知恢复**：如何让不同对话分支拥有各自的计划状态

---

## 🧠 为什么要做显式规划？

普通的 tool-calling Agent 很擅长“下一步做什么”，但不擅长一直记住“整个任务要怎么推进”。

对于这类请求：

```text
请先定位登录失败的原因，再修复它，最后补一条回归测试
```

如果没有显式计划，Agent 往往会出现三类问题：

1. **漂移**：做了一两步之后偏离主目标
2. **失忆**：明明已经完成了一步，但没有更新当前进度
3. **不可见**：用户只能看到推理和工具调用，看不到“整体执行计划”

所以本章新增一层“会话内规划状态”：

- 当前有哪些 todo
- 哪一项正在进行
- 哪些项已经完成

它不追求复杂，只追求**清晰、稳定、能恢复**。

---

## 🧱 本章实现的核心能力

### 1. `todo_write`：显式写计划

Agent 新增一个原生工具：

```text
todo_write
```

它的输入是**整份 todo 快照**，而不是增量 patch：

```json
{
  "items": [
    { "content": "Inspect login flow", "status": "completed" },
    { "content": "Patch handler logic", "status": "in_progress" },
    { "content": "Run regression tests", "status": "pending" }
  ]
}
```

这样设计有几个好处：

- 模型更容易生成
- 服务端更容易校验
- 前端更容易渲染
- 页面刷新后更容易恢复

### 2. 最小状态机

本章只保留 3 种状态：

- `pending`
- `in_progress`
- `completed`

并且限制：

- 最多 8 条 todo
- 最多只能有 1 条 `in_progress`
- `content` 不能为空

这足够讲清楚“计划如何推进”，但不会过早进入完整任务系统的复杂度。

### 3. Nag Reminder：轻量提醒，不是强制中断

提醒不是一个新工具，也不是前端弹窗。

它是在 Agent loop 内部，按条件临时注入的一段软提醒：

- 如果已经进入 tool loop，但还没有 todo，提醒它先写计划
- 如果 todo 已经存在，但明显落后于当前执行进度，提醒它更新计划

提醒是**本轮请求内临时生效**的，不会污染长期历史消息。

### 4. 计划快照持久化

在 TUI 里，把 todo 放在内存里就够了；但在 Web 场景里不够。

因为：

- 下一条消息是新的 HTTP 请求
- 页面可能刷新
- 对话还有分支

所以本章把最新计划快照序列化到 `ChatMessage.PlanState` 中。这样下一轮请求开始时，就能沿祖先链找到最近一次计划快照，并恢复当前 planning state。

### 5. Web 侧可视化

本章前端在聊天区域右侧增加一个只读 todo 面板：

- 流式收到 `todo_snapshot` 事件时，直接覆盖当前面板
- 重新加载历史消息时，从最近一条消息的 `plan_state` 恢复
- 不支持手工拖拽、编辑、排序

这是一个**Agent 计划观察面板**，不是用户任务看板。

---

## 🗂 代码结构

```text
ch12/
├── agent/
│   ├── agent.go            # Agent loop + nag reminder + todo snapshot event
│   ├── reminder.go         # 提醒触发逻辑
│   ├── plan/
│   │   ├── state.go        # PlanningState / PlanItem / PlanStatus
│   │   └── manager.go      # todo 校验与整份快照替换
│   └── tool/
│       ├── bash.go
│       ├── todo.go         # todo_write 工具
│       └── tool.go
├── server/
│   ├── db.go               # ChatMessage 增加 PlanState
│   ├── history.go          # 分支感知恢复 history 与 latest plan
│   ├── service.go          # SSE 输出 todo_snapshot，保存最新快照
│   └── controller.go
├── vo/
│   ├── sse.go              # SSEMessageVO 增加 plan_state
│   └── vo.go               # ChatMessageVO 增加 plan_state
└── main/
    └── main.go
```

前端复用根目录的 `frontend/`，并在 legacy chat 流程中增加 `TodoPanel`。

> 这里刻意没有把 planning state 直接接进 `assistant-ui` runtime。
>
> 原因很简单：本章的教学重点是显式规划本身，而不是 UI runtime 的二次封装。先用更直接的前端路径把核心原理讲清楚，后续再做更复杂的运行时整合更合适。

---

## ▶️ 运行方式

### 1. 启动后端

在项目根目录：

```bash
go run ./ch12/main
```

默认监听：

```text
http://localhost:8080
```

### 2. 启动前端

```bash
pnpm --dir frontend dev
```

打开浏览器访问 Vite 输出的本地地址即可。

### 3. 建议测试的输入

可以尝试这类多步骤任务：

```text
请先阅读当前仓库的 README，找出 ch12 还缺什么，再给出一个修改方案
```

或者：

```text
请先定位登录逻辑，再修复 bug，最后补测试
```

如果 Agent 进入多步执行，你应该能在右侧看到 todo 面板逐步出现并更新。

---

## ✅ 本章验证方式

后端测试：

```bash
go test -v ./ch12/...
```

前端构建：

```bash
pnpm --dir frontend build
```

手工检查：

1. 发起一个多步骤请求
2. 确认右侧 `Current Plan` 面板出现
3. 确认后续 `todo_write` 会更新当前项状态
4. 刷新页面后确认计划仍能恢复
5. 从旧消息分支继续对话时，确认计划跟着分支变化

---

## 🚧 本章刻意没做什么

为了保持教学重点，本章**没有**实现：

- `blocked / cancelled` 等更复杂状态
- todo 排序规则
- 用户直接编辑 todo
- 后台任务与定时调度
- 多 Agent 共用计划
- 真正的任务系统 / 调度系统

这些内容更适合后续做成“完整版规划”章节。

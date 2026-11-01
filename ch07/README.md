# 第七章：Agentic RAG（检索增强生成）

欢迎来到第七章！在前面章节的基础上，本章介绍 Agent 开发中最重要的能力之一：**Agentic RAG**（Agentic Retrieval-Augmented Generation）。

与传统的"手动配置检索"不同，**Agentic RAG 的核心在于让 Agent 自主决定**：
- 何时需要检索（根据问题判断是否需要查阅文档）
- 检索什么内容（自主生成查询语句）
- 检索多少内容（动态调整 Top-K 数量）
- 如何利用检索结果（结合上下文工程进行推理）

当 RAG 能力与第五章的**上下文工程**（Context Engineering）和第六章的**记忆机制**（Memory System）结合时，Agent 将具备强大的知识检索和推理能力，能够在有限的上下文窗口内高效处理复杂任务。

---

## 🎯 你将学到什么

1. **Agentic RAG 概念**：理解 Agent 如何自主决策检索时机、查询内容和数量
2. **文本切分（Chunking）**：如何将长文档切分为适合检索的块
3. **向量嵌入（Embedding）**：如何将文本转换为高维向量表示
4. **向量存储**：如何使用 pgvector 存储和检索向量
5. **重排序（Reranking）**：如何使用重排序模型提升检索质量
6. **搜索工具集成**：如何将语义搜索能力封装为 Agent 工具
7. **代码索引实战**：一个完整的代码仓库索引系统实现

---

## 🛠 准备工作

本章需要使用 PostgreSQL + pgvector 扩展作为向量数据库。

### 安装 PostgreSQL 和 pgvector

请参考官方文档：https://github.com/pgvector/pgvector


### 配置 Embedding 和 Rerank 服务

本章使用兼容 OpenAI API 格式的 Embedding 和 Rerank 服务。你需要配置 `HTTPEmbeddingConfig` 和 `HTTPRerankConfig`。

相关代码：`ch07/rag/embedding.go`、`ch07/rag/rerank.go`

---

## 📖 核心原理解析

### 1. 什么是 Agentic RAG？

**RAG（Retrieval-Augmented Generation，检索增强生成）** 是一种结合信息检索和生成模型的技术架构：

| 组件 | 作用 |
|------|------|
| **检索器（Retriever）** | 根据查询从知识库中找到相关文档片段 |
| **生成器（Generator）** | 基于检索到的文档生成回答 |

**传统 RAG vs Agentic RAG**：

```text
传统 RAG：
用户问题 → 预定义检索逻辑 → 固定查询 → 固定 Top-K → LLM → 回答

Agentic RAG：
用户问题 → Agent 判断是否需要检索 → 自主生成查询 → 动态调整 Top-K → LLM → 回答
              ↓ 必要时迭代检索
```

**Agentic RAG 的优势**：
- ✅ **自主决策**：Agent 根据问题复杂度决定是否需要检索、检索几次
- ✅ **动态查询**：Agent 根据上下文自主生成和优化查询语句
- ✅ **灵活召回**：根据问题类型动态调整召回数量
- ✅ **减少幻觉**：基于检索到的真实信息回答
- ✅ **知识可更新**：更新向量库即可更新知识
- ✅ **与上下文工程结合**：利用第五章的上下文管理策略，将检索结果高效注入上下文

**与上下文工程的协同**：
- 检索结果作为外部知识，可通过**卸载策略**存储，按需加载
- 多轮检索结果可通过**摘要策略**压缩，避免上下文膨胀
- 检索历史可通过**记忆机制**保存，避免重复检索相同内容

---

### 2. 语义搜索需要哪些组件？

构建一个完整的语义搜索系统，需要以下核心组件：

```
┌─────────────────────────────────────────────────────────────┐
│                       语义搜索系统                            │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐   │
│  │   文本切分    │  → │    向量嵌入   │ →   │    向量存储   │   │
│  │  (Chunking)  │    │ (Embedding)  │    │(Vector Store)│   │
│  └──────────────┘    └──────────────┘    └──────────────┘   │
│          ↓                                        ↓         │
│     长文档 → 文档块                        查询 → 相关文档      │
│                                                   ↓         │
│                                             ┌─────────-─┐   │
│                                             │   重排序   │   │
│                                             │(Reranking)│   │
│                                             └────────-──┘   │
│                                                   ↓         │
│                                                精排结果      │
│                                                   ↓         │
│                                             ┌────────────┐  │
│                                             │  搜索工具   │  │
│                                             │  (Tool)    │  │
│                                             └────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

下面我们逐一介绍这些组件。

---

### 3. 文本切分（Chunking）

长文档需要切分成小块才能有效处理。本章实现了两种切分器（`ch07/rag/chunker.go`）：

#### 3.1 为什么需要文档切分？

文档切分并不是可选的优化步骤，而是 RAG 系统能够正常工作的**前提条件**。原因有以下几点：

**① Embedding 模型有输入长度限制**

Embedding 模型（如 `text-embedding-3-small`）的输入长度通常在 512~8192 个 token 之间。一个真实项目中的代码文件或技术文档往往远超这个限制。如果直接对整个文件做 Embedding，内容会被截断，导致后半部分信息完全丢失。

**② 整文档 Embedding 会"稀释"语义**

即使文档没有超长，把 1000 行代码压缩成一个向量，这个向量需要同时表达所有函数、注释、结构的语义，结果是任何查询都和它"有点像"却又"不够像"——相似度分布趋于平坦，检索区分度大幅下降。

```text
整文档 Embedding：
查询"如何处理错误" → 余弦相似度 0.72（噪声太多，不够精确）

切分后 Embedding：
查询"如何处理错误" → error_handling 块相似度 0.91 ✅
                  → add_function 块相似度 0.31   ❌（被过滤）
```

**③ 检索召回的内容要能"放进"上下文窗口**

RAG 的最终目的是把相关内容送给 LLM。如果召回的是整个 10000 行的文件，它本身就会撑爆 LLM 的上下文窗口。切分后，Top-K 召回的是若干小块，总 token 量可控，能精准注入上下文。

**④ 小块精准定位，大文档引入噪声**

用户问"第 42 行那个函数是干什么的"，理想答案只需要 10 行代码。如果整个文件都塞进去，LLM 需要在大量无关内容中"自己找"，既浪费 token，也容易引入干扰。切分让每个检索单元只携带一个语义焦点。

**⑤ 块是增量更新的最小单位**

文件局部修改时，只需要删除并重新索引受影响的块，而不是整个文件。这让增量更新（见后文"增量更新与去重"小节）成为可能，大幅降低重建索引的成本。

**切分粒度的核心权衡**：

```text
块太小 → 单块上下文不足，LLM 看不到完整信息，需要更多次检索
块太大 → Embedding 语义稀释，噪声增多，上下文窗口压力大
```

没有一个"最优"块大小，合适的粒度取决于文档类型、Embedding 模型和下游任务，必须通过实验确定（见注意事项第 7 条）。

---

#### 3.2 行切分（LineChunker）

**原理**：按行切分，同时控制行数和字符数。

**切分逻辑**：
- 逐行累积
- 超过行数或字符数限制时开始新块
- 保留行号信息用于定位

**示例**：

原始代码：
```go
func Add(a, b int) int {
    return a + b
}

func Subtract(a, b int) int {
    return a - b
}

func Multiply(a, b int) int {
    return a * b
}
// ... 更多函数
```

`maxLines=5, maxChars=200` 时切分为：
```go
Chunk #1 (行 1-5):
func Add(a, b int) int {
    return a + b
}

func Subtract(a, b int) int {

Chunk #2 (行 6-10):
    return a - b
}

func Multiply(a, b int) int {
    return a * b
}
// ...
```

**适用场景**：代码文件、结构化文本

#### 3.3 段落切分（ParagraphChunker）

**原理**：按空行分隔的段落切分。

**切分逻辑**：
- 空行表示段落结束
- 多个段落组成一个块
- 保留行号信息

**示例**：

原始 Markdown 文档：
```markdown
## 什么是 RAG？

RAG（Retrieval-Augmented Generation）是一种结合信息检索和生成模型的技术架构。

## RAG 的优势

RAG 能够减少模型幻觉，基于真实信息生成回答。

知识可以随时更新，无需重新训练模型。
```

`maxParagraphs=2` 时切分为：
```text
Chunk #1 (段落 1-2):
## 什么是 RAG？

RAG（Retrieval-Augmented Generation）是一种结合信息检索和生成模型的技术架构。

## RAG 的优势

RAG 能够减少模型幻觉，基于真实信息生成回答。

Chunk #2 (段落 3):
知识可以随时更新，无需重新训练模型。
```

**适用场景**：Markdown 文档、文章

#### 3.4 语法树切分（ASTChunker，未实现）

**原理**：基于抽象语法树（AST）按代码逻辑结构切分。

**切分逻辑**：
- 解析源代码生成 AST
- 按函数、类、方法等语义单元切分
- 保留语法结构和依赖关系

**示例**：

原始代码：
```go
package main

type Calculator struct {
    name string
}

func NewCalculator(name string) *Calculator {
    return &Calculator{name: name}
}

func (c *Calculator) Add(a, b int) int {
    return a + b
}

func (c *Calculator) Subtract(a, b int) int {
    return a - b
}
```

AST 切分后（按函数/方法单元）：
```text
Chunk #1 - Calculator 类型定义:
type Calculator struct {
    name string
}

Chunk #2 - NewCalculator 函数:
func NewCalculator(name string) *Calculator {
    return &Calculator{name: name}
}

Chunk #3 - Add 方法:
func (c *Calculator) Add(a, b int) int {
    return a + b
}

Chunk #4 - Subtract 方法:
func (c *Calculator) Subtract(a, b int) int {
    return a - b
}
```

**优势**：
- 切分结果符合代码逻辑边界
- 保证代码块的语义完整性
- 便于理解代码结构和依赖

**适用场景**：大型代码库、需要理解代码结构的场景

**实现挑战**：
- 需要为每种编程语言实现解析器
- AST 解析性能开销较大
- 跨语言支持复杂度高

#### 3.5 切分器对比

| 切分器 | 优势 | 劣势 | 适用场景 |
|--------|------|------|----------|
| 行切分（LineChunker） | 实现简单、保留行号 | 可能切断语义单元 | 代码文件、结构化文本 |
| 段落切分（ParagraphChunker） | 保持段落完整性 | 段落大小不均 | Markdown 文档、文章 |
| 语法树切分（ASTChunker） | 符合代码逻辑边界、语义完整 | 实现复杂、性能开销大 | 大型代码库 |

#### 3.6 块大小策略

| 策略 | 优势 | 劣势 | 适用场景 |
|------|------|------|----------|
| 小块（512 tokens） | 精确匹配、检索快 | 上下文碎片化 | 精确问答 |
| 大块（2048 tokens） | 上下文完整 | 噪声较多、检索慢 | 总结生成 |
| 滑动窗口 | 保留上下文 | 存储冗余 | 需要连续性的场景 |

---

### 4. 向量嵌入（Embedding）

**Embedding** 将文本转换为高维向量表示，使得语义相似的文本在向量空间中距离更近。

**核心接口**（`ch07/rag/embedding.go`）：
```go
type EmbeddingService interface {
    Embed(ctx context.Context, chunk string) (Vector, error)
}
```

**向量维度**：
- OpenAI `text-embedding-3-small`: 1536 维
- 本章默认配置：512 维（可通过 `HTTPEmbeddingConfig` 配置）

**相似度计算**：
- 余弦相似度（Cosine Similarity）：最常用
- 欧氏距离（Euclidean Distance）
- 点积（Dot Product）

本章使用 pgvector 的余弦相似度算子 `<=>`。

---

### 5. 向量存储（Vector Store）

本章使用 **pgvector** 扩展的 PostgreSQL 作为向量数据库（`ch07/db/pgvector.go`）。

#### 5.1 为什么选择 pgvector？

| 特性 | pgvector | Pinecone/Weaviate |
|------|----------|-------------------|
| 部署成本 | 低（复用 PostgreSQL） | 高（专用服务） |
| 运维复杂度 | 低 | 中 |
| 性能 | 中等 | 高 |
| 适用场景 | 中小规模数据 | 大规模生产环境 |

#### 5.2 核心功能

**VectorStore 接口**（`ch07/rag/type.go`）：
```go
type VectorStore interface {
    InsertBatch(ctx context.Context, vps []VectorPoint) error
    Search(ctx context.Context, queryVector Vector, limit int) ([]VectorPointResult, error)
    DeleteByDocument(ctx context.Context, documentID string) error
    GetDocumentIndexedTime(ctx context.Context, documentID string) (time.Time, error)
    Clear(ctx context.Context) error
    Close() error
}
```

**索引策略**：使用 IVFFlat 索引加速向量搜索（`ch07/db/pgvector.go:100-111`）(HNSW索引快)

---

### 6. 重排序（Reranking）

**问题**：向量相似度不一定等于语义相关性

**解决方案**：使用专门的 Rerank 模型对召回的候选文档重新排序

**Reranker vs Embedding**：

| 特性 | Embedding | Reranker |
|------|-----------|----------|
| 用途 | 召回（快速筛选） | 精排（精细排序） |
| 速度 | 快 | 慢 |
| 准确度 | 中等 | 高 |
| 典型流程 | Top-50/100 | Top-10 |

**两阶段检索**：
```
查询 → Embedding → 向量搜索(Top-50) → Rerank → 最终结果(Top-10)
```

**核心接口**（`ch07/rag/rerank.go`）：
```go
type RerankService interface {
    Rerank(ctx context.Context, query string, candidates []Chunk) ([]Chunk, error)
}
```

相关代码：`ch07/rag/rerank.go`、`ch07/tool/semantic_search.go:86-114`

---

### 7. 搜索工具集成

将语义搜索能力封装为 Agent 工具，让 Agent 可以自主调用（`ch07/tool/semantic_search.go`）。

**工具定义**：
```go
type SemanticSearchTool struct {
    embedService  shared.EmbeddingService
    vectorStore   shared.VectorStore
    rerankService shared.RerankService
}
```

**工具信息**：
```json
{
  "name": "semantic_search",
  "description": "在向量库中进行语义搜索，查找与查询相关的文档片段",
  "parameters": {
    "query": "搜索查询文本",
    "top_k": "返回结果数量，默认为5"
  }
}
```

**Agent 自主决策示例**：
```text
用户：这个项目怎么处理错误的？

Agent 决策过程：
1. 分析问题：需要了解项目的错误处理机制
2. 决定检索：调用 semantic_search 工具
3. 生成查询："error handling"、"错误处理"
4. 动态调整 Top-K：5 个结果
5. 基于检索结果生成回答
```

---

## 💡 实战案例：代码仓库索引系统

理解了语义搜索的核心组件后，我们来实现一个完整的代码仓库索引系统。这个系统将：

1. 遍历代码仓库，找到所有文本文件
2. 将文件切分为合适的块
3. 为每个块生成向量嵌入
4. 存储到向量数据库
5. 支持增量更新和并发索引

### 1. 文件遍历（FileWalker）

**FileWalker** 负责遍历目录查找文本文件（`ch07/index/file_walker.go`）。

**核心功能**：
- 自动排除常见目录（`.git`、`node_modules` 等）
- 按扩展名过滤文件类型
- 支持自定义排除规则和扩展名

**预配置扩展名**：代码文件、配置文件、文档文件等 30+ 种类型

```go
fileWalker := NewFileWalker()
files, err := fileWalker.Walk("/path/to/repo")
```

---

### 2. 索引器（Indexer）

**索引器**负责将代码仓库索引到向量数据库（`ch07/index/indexer.go`）。

**核心流程**：
```
文件遍历 → 切分 → Embedding → 批量插入 → 索引完成
```

**创建索引器**：
```go
embedConfig := rag.DefaultHTTPEmbeddingConfig("your-api-key")
embedService := rag.NewHTTPEmbeddingService(embedConfig)

indexerConfig := index.IndexerConfig{
    RootPath:    "/path/to/repo",
    ChunkerType: rag.ChunkerTypeLine,
    MaxLines:    100,
    MaxChars:    2000,
}

indexer := index.NewIndexer(indexerConfig, store, embedService)
```

---

### 3. 增量更新与去重

**问题**：如何避免重复索引未修改的文件？

**解决方案**：
1. 检查文件是否已索引（`GetDocumentIndexedTime`）
2. 比较文件修改时间和索引时间
3. 未修改则跳过（`IndexActionSkip`）
4. 已修改则删除旧记录并重新索引（`IndexActionReindex`）

**索引结果统计**：
```go
type IndexResult struct {
    TotalFiles     int           // 总文件数
    SuccessFiles   int           // 成功索引的文件数
    FailedFiles    int           // 失败的文件数
    SkippedFiles   int           // 跳过的文件（已索引且未修改）
    ReindexedFiles int           // 重新索引的文件（已修改）
    TotalChunks    int           // 总块数
    Duration       time.Duration // 索引耗时
    Errors         []error       // 错误列表
}
```

相关代码：`ch07/index/indexer.go:148-183`

---

### 4. 并发索引

**问题**：大规模仓库索引速度慢？

**解决方案**：使用 worker pool 并发处理文件

```go
// 串行索引
result, err := indexer.Index(ctx)

// 并发索引（10 个 worker）
result, err := indexer.IndexConcurrent(ctx, 10)
```

**实现要点**：
- 使用 channel 作为任务队列
- 多个 worker 并发处理
- 使用 mutex 保护共享状态
- 默认并发度：10

相关代码：`ch07/index/indexer.go:84-146`

---

## 💻 代码结构速览

### RAG 包（`ch07/rag/`）
- `type.go`：核心类型定义（`Chunk`、`Vector`、`VectorStore` 接口等）
- `chunker.go`：文本切分器（`LineChunker`、`ParagraphChunker`）
- `embedding.go`：Embedding 服务（`HTTPEmbeddingService`）
- `rerank.go`：Rerank 服务（`HTTPRerankService`）

### Index 包（`ch07/index/`）
- `indexer.go`：索引器（`Indexer`、索引流程、增量更新）
- `file_walker.go`：文件遍历器（`FileWalker`）

### DB 包（`ch07/db/`）
- `pgvector.go`：pgvector 实现（`PGVectorStore`、表结构、索引）

### Tool 包（`ch07/tool/`）
- `tool.go`：工具接口定义（`Tool`、工具类型枚举）
- `semantic_search.go`：语义搜索工具（`SemanticSearchTool`）

---

## 🎓 进阶话题

### 1. 语义搜索 vs 全文搜索 vs 混合搜索

本章代码预留了三种搜索工具的定义（`ch07/tool/tool.go`），但目前只实现了语义搜索。

#### 1.1 全文搜索（未实现）

可以使用 PostgreSQL 的全文搜索功能：

```sql
CREATE INDEX idx_document_chunks_content_fts
ON document_chunks
USING gin(to_tsvector('english', content));

SELECT content, document_id, start_pos, end_pos
FROM document_chunks
WHERE to_tsvector('english', content) @@ to_tsquery('english', 'error & handling')
ORDER BY ts_rank(to_tsvector('english', content), to_tsquery('english', 'error & handling')) DESC
LIMIT 10;
```

#### 1.2 混合搜索（未实现）

结合语义搜索和全文搜索：

```go
type HybridSearchTool struct {
    semanticTool *SemanticSearchTool
    fullTextTool *FullTextSearchTool
    alpha        float64 // 语义搜索权重
}

func (h *HybridSearchTool) Search(ctx context.Context, query string, topK int) ([]Result, error) {
    // 1. 并行执行两种搜索
    semanticResults, _ := h.semanticTool.Search(ctx, query, topK*2)
    fullTextResults, _ := h.fullTextSearch.Search(ctx, query, topK*2)

    // 2. 融合排序
    return h.mergeResults(semanticResults, fullTextResults, topK)
}
```

---

### 2. 其他向量存储方案

| 方案 | 特点 | 适用场景 |
|------|------|----------|
| **pgvector** | PostgreSQL 扩展，易部署 | 中小规模、已有 PG 基础设施 |
| **Pinecone** | 托管服务，性能优秀 | 快速原型、大规模生产 |
| **Weaviate** | 开源，支持多种模态 | 需要多模态检索 |
| **Milvus** | 开源，高性能 | 大规模向量检索 |
| **Chroma** | 轻量级，易集成 | 本地开发、小规模应用 |

---

### 3. 元数据过滤

在实际应用中，往往需要在搜索时添加元数据过滤条件：

```go
type SearchFilter struct {
    FileExtensions []string      // 只搜索特定文件类型
    DateRange      [2]time.Time  // 时间范围
    MinLines       int           // 最小行数
}
```

可以在 `VectorStore.Search` 方法中添加过滤参数，在 SQL 查询中添加 WHERE 条件。

---

### 4. 多语言文档支持

对于多语言文档，可以考虑：
1. **语言检测**：使用语言检测模型识别文档语言
2. **多语言 Embedding**：使用支持多语言的模型（如 M3E-base）
3. **语言过滤**：搜索时只检索指定语言的文档

---

### 5. 向量索引算法（可选阅读）

向量索引是加速向量相似度搜索的关键技术。不同的索引算法在查询速度、内存占用、构建成本等方面各有权衡。

#### 5.1 为什么需要向量索引？

**暴力搜索的问题**：
- 时间复杂度：O(n × d)，其中 n 是向量数量，d 是向量维度
- 百万级向量搜索需要数秒
- 无法满足实时查询需求

**向量索引的解决方案**：
- 将向量空间划分为多个区域
- 搜索时只检查最相关的区域
- 将复杂度降低到 O(log n × d) 或更低

#### 5.2 IVF（Inverted File Index）

**原理**：
- 使用聚类算法（如 K-Means）将向量空间划分为多个 Voronoi 单元（桶）
- 每个桶分配一个聚类中心
- 搜索时只查询最近的 n 个桶

**优势**：
- 实现简单，易于理解
- 内存占用可控
- 支持增量添加向量

**劣势**：
- 需要预先训练聚类中心
- 查询速度受桶数量影响
- 可能错过边界附近的向量

**适用场景**：中等规模数据集（百万级）

#### 5.3 HNSW（Hierarchical Navigable Small World）

**原理**：
- 构建多层图结构，类似跳表
- 上层稀疏，下层密集
- 搜索时从顶层开始，逐层向下收敛到最近邻

**优势**：
- 查询速度极快
- 召回率高
- 无需训练，支持增量构建

**劣势**：
- 内存占用较大（需存储完整图）
- 构建时间较长
- 删除向量较复杂

**适用场景**：大规模数据集（千万级以上），对查询延迟敏感

#### 5.4 DiskANN

**原理**：
- 微软开发的磁盘优化索引算法
- 基于 HNSW 但针对磁盘存储优化
- 使用 SSD 随机访问能力，将部分数据存储在磁盘上

**优势**：
- 内存占用极低（可处理十亿级向量）
- 查询速度快（利用 SSD 性能）
- 支持动态更新

**劣势**：
- 实现复杂，依赖专用数据结构
- 需要 SSD 硬件支持
- 开源实现较少

**适用场景**：超大规模数据集（十亿级以上），内存受限场景

#### 5.5 算法对比

| 算法 | 查询速度 | 召回率 | 内存占用 | 构建成本 | 增量更新 | 适用规模 |
|------|----------|--------|----------|----------|----------|----------|
| 暴力搜索 | 慢 | 100% | 低 | 无 | 支持 | < 10万 |
| IVF | 中等 | 中等 | 中等 | 中等 | 支持 | 百万级 |
| IVF-PQ | 快 | 中等 | 低 | 中等 | 支持 | 百万级 |
| HNSW | 极快 | 高 | 高 | 高 | 支持 | 千万级 |
| DiskANN | 快 | 高 | 极低 | 高 | 支持 | 十亿级 |

**备注**：
- IVF-PQ：IVF + Product Quantization，使用乘积量化压缩向量以减少内存
- pgvector 默认支持 IVFFlat 和 HNSW

#### 5.6 选择建议

**数据规模**：
- < 10 万：暴力搜索即可
- 10 万 - 100 万：IVF 或 IVF-PQ
- 100 万 - 1 亿：HNSW
- > 1 亿：DiskANN 或分布式方案

**硬件约束**：
- 内存充足：HNSW
- 内存受限：IVF-PQ 或 DiskANN
- 无 SSD：避免 DiskANN

**查询模式**：
- 高并发低延迟：HNSW
- 离线批处理：IVF
- 动态更新频繁：HNSW 或 IVF

### 6. Coding Agent 为什么不用 Embedding，而用 Grep？

学完本章后，你可能会有一个疑问：既然 RAG 这么强，为什么 Claude Code 这类 Coding Agent 在检索代码时，不建向量索引、不用 Embedding，而是直接让 Agent 调用 `grep`、`find`、`read` 这些文件系统工具？

这是一个非常值得深入思考的问题，答案涉及代码与自然语言文本的本质差异。

#### 6.1 代码的标识符本身就是精确语义

自然语言文档里，"删除用户"和"移除账号"语义相近但字面不同，所以需要 Embedding 把它们映射到相近的向量空间。

代码不一样。代码库里，函数叫 `deleteUser`，调用它的地方也写 `deleteUser`。**标识符就是精确的语义锚点**，grep 能做到 100% 召回、0 误召回。用 Embedding 反而是把一个精确问题变成了一个近似问题。

```text
Embedding 检索"删除用户的函数":
  → deleteUser() 相似度 0.91 ✅
  → removeAccount() 相似度 0.87 ✅（可能是误召回）
  → createUser() 相似度 0.61 ❌（被过滤，但引入了噪声）

Grep 检索 "deleteUser":
  → 精确命中所有调用点，无误召回，无遗漏
```

#### 6.2 索引会过时，文件系统永远最新

向量索引是代码在某一时刻的快照。开发者每次修改文件，索引就变脏了。

- **索引过时**：Agent 检索到的块可能是修改前的旧代码，基于错误信息给出错误建议
- **实时 grep**：每次搜索都直接读文件系统，永远反映代码当前真实状态

对于 Coding Agent 来说，信息的准确性比召回速度更重要。一个基于过时索引的答案比慢一点但正确的答案危害大得多。

#### 6.3 构建和维护索引的成本超过收益

RAG 管道有显著的工程开销：

| 环节 | 代价 |
|------|------|
| 初次建索引 | 对每个文件调用 Embedding API，耗时且有费用 |
| 保持索引新鲜 | 需要 file watcher / git hook 持续触发增量更新 |
| 存储 | 需要运行向量数据库（如 pgvector、Chroma）|
| 调试 | 召回结果不透明，难以解释为什么检索到某个块 |

而 grep 的代价是零：无需预处理、无需外部服务、无需维护，Agent 随时调用随时拿到结果。

#### 6.4 Agentic 搜索比静态检索更强

传统 RAG 是**一次性的**：把查询向量化，从索引里拉 Top-K，完事。

Agent + grep 是**迭代的、有策略的**：

```text
Agent 的真实搜索过程：
1. grep -r "deleteUser" .          → 找到调用点
2. read auth/user.go               → 读函数定义
3. grep -r "import.*auth" .        → 找哪些模块依赖它
4. read api/handler.go             → 读 HTTP 层如何调用
5. grep "deleteUser.*cascade" .    → 检查是否有级联删除
```

这是一种**推理驱动的搜索**，Agent 根据每一步的结果决定下一步搜什么，最终沿着代码的真实依赖链找到完整上下文。向量检索的一次 Top-K 拉取无法做到这一点。

#### 6.5 上下文窗口的扩大改变了权衡

早期 LLM 上下文窗口只有 4K~8K token，必须用 RAG 精确控制送入的内容。现在主流模型已达 128K~200K token，可以直接把整个相关文件塞进上下文，不再需要切片。

RAG 最初是对**小上下文窗口的补偿方案**。上下文窗口越大，"检索精确片段"的必要性就越低，"直接读整个文件"的可行性就越高。

#### 6.6 那么 RAG 在 Coding 场景下还有价值吗？

有，但适用场景更窄：

| 场景 | 推荐方案 |
|------|----------|
| 超大规模代码库（数百万行），无法枚举文件 | Embedding + 向量检索做粗筛，再 grep 精确定位 |
| 文档、注释、Changelog 的自然语言问答 | RAG 效果好，关键词 grep 不适用 |
| 跨语言语义搜索（如搜"实现了 X 功能的函数"）| Embedding 优于精确匹配 |
| 代码库实时变化、响应延迟敏感 | Grep / 文件系统，索引太难保鲜 |

**核心结论**：RAG 解决的是"海量非结构化文本、关键词不精确"的检索问题。代码库的检索问题恰好相反——标识符精确、结构清晰、变化频繁——所以 Agent + 文件系统工具在大多数 Coding 场景下是更务实的选择。

---

## ⚠️ 注意事项

1. **Embedding 模型选择**：不同模型对不同类型文本的效果不同，需要根据实际场景选择
2. **向量维度**：维度越高表达能力越强，但存储和计算成本也越高
3. **切分策略**：需要根据文档类型选择合适的切分策略
4. **批量操作**：大量 Embedding 调用时要注意 API 限流
5. **索引更新**：代码更新后需要重新索引，可以设置定时任务或 git hook
6. **数据库维护**：定期 VACUUM 和 REINDEX 保持性能
7. **策略必须先评测再上线**：切分、召回、重排、索引等策略都要先经可复现实验验证（效果与成本），不能仅凭人工判断当作优化直接上线

---

## 📚 扩展阅读与参考资料

1. **[pgvector GitHub](https://github.com/pgvector/pgvector)**
   - pgvector 官方文档

2. **[LangChain RAG Tutorial](https://python.langchain.com/docs/tutorials/rag/)**
   - LangChain 的 RAG 实现教程

3. **[The Retrieval-Augmented Generation (RAG) Pattern](https://arxiv.org/abs/2005.11401)**
   - RAG 的原始论文

4. **[Vector Database Comparison](https://www.pinecone.io/learn/vector-database/)**
   - 向量数据库对比介绍

5. **[Faiss: A library for efficient similarity search](https://github.com/facebookresearch/faiss)**
   - Facebook 开源的向量索引库，包含多种索引算法实现

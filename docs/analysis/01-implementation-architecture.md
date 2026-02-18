# 1. 当前项目实现架构

## 1.1 系统定位

Mnemon 是一个 **独立的记忆守护进程**（daemon），而非库或插件。它的核心设计哲学是：

- **独立于 LLM 会话生命周期**：LLM 重启、context compact 不影响记忆
- **CLI-in-the-loop**：CLI（如 Claude Code）就是 mnemon 的 LLM 引擎，通过 CLI 调用驱动所有需要 LLM 能力的操作（实体补充、因果评估、语义判断、意图覆盖等）
- **二进制零外部依赖**：纯 Go 二进制 + SQLite，无需 API Key、无需 Python 运行时——LLM 能力由引擎层（CLI）提供，不嵌入二进制
- **MAGMA 四图架构**：Temporal、Entity、Causal、Semantic 四种边类型

## 1.2 技术栈

| 层级 | 技术选型 | 说明 |
|------|----------|------|
| 语言 | Go | 编译型单二进制，交叉编译方便 |
| 存储 | SQLite (WAL mode) | `modernc.org/sqlite`，纯 Go 实现无 CGO |
| CLI 框架 | Cobra (`spf13/cobra`) | 标准 Go CLI 框架 |
| ID 生成 | UUID v4 (`google/uuid`) | |
| 向量模型 | Ollama nomic-embed-text | 可选，本地推理 |
| LLM 集成 | CLI-in-the-loop | CLI（Claude Code）是 LLM 引擎，驱动实体补充、因果评估、语义判断 |

## 1.3 代码结构

```
mnemon/
├── main.go                          # 入口
├── cmd/                             # CLI 命令层
│   ├── root.go                      # 根命令 + openDB()
│   ├── remember.go                  # 写入：存储 insight
│   ├── recall.go                    # 读取：关键词/智能召回
│   ├── search.go                    # 读取：token 评分搜索
│   ├── diff.go                      # 读取：去重/冲突检测
│   ├── link.go                      # 写入：手动边创建
│   ├── related.go                   # 读取：BFS 图遍历
│   ├── forget.go                    # 写入：软删除
│   ├── gc.go                        # 生命周期：留存管理
│   ├── embed.go                     # 向量：嵌入管理
│   ├── status.go                    # 观测：统计信息
│   └── log.go                       # 观测：操作日志
│
├── internal/
│   ├── model/                       # 数据模型
│   │   ├── node.go                  # Insight 结构体 + Category 枚举
│   │   └── edge.go                  # Edge 结构体 + EdgeType 枚举
│   │
│   ├── store/                       # 存储层（SQLite CRUD）
│   │   ├── db.go                    # 连接管理 + Schema 迁移
│   │   ├── node.go                  # Insight CRUD + 生命周期计算
│   │   ├── edge.go                  # Edge CRUD + 图查询
│   │   └── oplog.go                 # 操作日志
│   │
│   ├── graph/                       # 图引擎（边自动生成）
│   │   ├── engine.go                # 编排：OnInsightCreated()
│   │   ├── temporal.go              # Temporal 边：backbone + proximity
│   │   ├── entity.go                # Entity 边：实体共现
│   │   ├── causal.go                # Causal 边：因果关键词检测
│   │   └── semantic.go              # Semantic 边：余弦相似度
│   │
│   ├── search/                      # 搜索与检索
│   │   ├── keyword.go               # Token 分词 + 评分
│   │   ├── intent.go                # 意图检测 + 边权重
│   │   └── recall.go                # 核心：多信号 RRF + beam search + reranking
│   │
│   └── embed/                       # 向量嵌入
│       ├── ollama.go                # Ollama HTTP 客户端
│       └── vector.go                # 余弦相似度 + 序列化
│
├── scripts/
│   └── e2e_test.sh                  # 端到端测试（125 个断言）
│
└── docs/                            # 文档
```

## 1.4 数据模型

### Insight（节点）

```
insights 表
├── id              TEXT PK          # UUID v4
├── content         TEXT NOT NULL     # 原始文本
├── category        TEXT              # preference|decision|fact|insight|context|general
├── importance      INTEGER           # 1-5 重要度
├── tags            TEXT              # JSON 数组
├── entities        TEXT              # JSON 数组（regex + LLM 提取）
├── source          TEXT              # 来源标识
├── access_count    INTEGER DEFAULT 0 # 访问计数
├── created_at      DATETIME          # 创建时间
├── updated_at      DATETIME          # 更新时间
├── deleted_at      DATETIME          # 软删除时间戳
├── last_accessed_at DATETIME         # 最后访问时间
├── embedding       BLOB              # float64 向量（little-endian）
└── effective_importance REAL         # 衰减后有效重要度
```

### Edge（边）

```
edges 表
├── source_id       TEXT              ─┐
├── target_id       TEXT               ├─ 复合主键
├── edge_type       TEXT              ─┘ CHECK IN (temporal, semantic, causal, entity)
├── weight          REAL              # 0.0 - 1.0
├── metadata        TEXT              # JSON（sub_type, created_by, cosine 等）
└── created_at      DATETIME
```

### OpLog（操作日志）

```
oplog 表
├── id              INTEGER PK AI    # 自增 ID
├── operation       TEXT              # 操作类型
├── insight_id      TEXT              # 关联 insight
├── detail          TEXT              # 详情
└── created_at      DATETIME
```

## 1.5 分层架构图

```
┌─────────────────────────────────────────────────────┐
│                    Claude (LLM)                      │
│  ┌──────────────────────────────────────────────┐   │
│  │  CLAUDE.md / memory.md skill 系统提示         │   │
│  │  · 何时调用 remember / recall / diff / link   │   │
│  │  · 如何评估 causal/semantic candidates        │   │
│  │  · 何时执行 gc / embed                        │   │
│  └──────────────────────────────────────────────┘   │
│         ↕ CLI 调用 (stdin/stdout JSON)               │
├─────────────────────────────────────────────────────┤
│              cmd/ — CLI 命令层                        │
│  · 参数解析 · 流程编排 · JSON 输出格式化              │
├─────────────────────────────────────────────────────┤
│  ┌────────────┐ ┌────────────┐ ┌─────────────────┐ │
│  │  graph/    │ │  search/   │ │    embed/       │ │
│  │  引擎层    │ │  搜索层    │ │    向量层       │ │
│  │            │ │            │ │                 │ │
│  │ temporal   │ │ keyword    │ │ ollama client   │ │
│  │ entity     │ │ intent     │ │ cosine sim      │ │
│  │ causal     │ │ recall     │ │ serialize       │ │
│  │ semantic   │ │            │ │                 │ │
│  └──────┬─────┘ └─────┬──────┘ └────────┬────────┘ │
│         │             │                  │          │
├─────────┴─────────────┴──────────────────┴──────────┤
│              store/ — 存储层 (SQLite)                 │
│  · db.go: 连接 + Schema 迁移                         │
│  · node.go: Insight CRUD + 生命周期                   │
│  · edge.go: Edge CRUD + 图查询                       │
│  · oplog.go: 操作审计                                │
├─────────────────────────────────────────────────────┤
│              model/ — 数据模型                        │
│  · Insight + Category                                │
│  · Edge + EdgeType                                   │
└─────────────────────────────────────────────────────┘
                        ↕
              ~/.mnemon/mnemon.db (SQLite WAL)
```

## 1.6 四图架构

### Temporal 图（时间图）

- **Backbone 链**：同 source 的相邻 insight 之间创建双向 PRECEDES/SUCCEEDS 边
- **Proximity 边**：24h 窗口内的 insight 之间创建边，权重 `w = 1/(1+hours_diff)`
- **最大边数**：每个新 insight 最多 10 条 proximity 边

### Entity 图（实体图）

- **实体提取**：双层提取
  - Regex 层：CamelCase、ALLCAPS、文件路径、URL、@mention、中文书名号
  - 字典层：140+ 技术术语（Go, React, Docker, Kubernetes, Redis 等）
  - LLM 层：Claude 通过 `--entities` flag 提供补充实体
- **共现边**：共享实体的 insight 之间创建双向边，metadata 标记 `entity: "实体名"`
- **最大边数**：每个实体最多链接 5 个已有 insight

### Causal 图（因果图）

- **信号检测**：regex 匹配因果关键词（because, therefore, due to, 因为, 所以 等）
- **Token 重叠**：计算新 insight 与最近 10 条同 source insight 的 token 重叠率
- **方向推断**：含因果关键词的一方为"果"，另一方为"因"
- **Sub-type 推荐**：causes / enables / prevents
- **候选输出**：2-hop BFS 邻域搜索，输出 causal_candidates 供 Claude 评估

### Semantic 图（语义图）

- **自动边**：当 embedding 可用且余弦相似度 >= 0.50 时自动创建，最多 3 条
- **候选边**：余弦相似度 >= 0.30 的输出为 semantic_candidates 供 Claude 评估
- **回退**：无 embedding 时使用 token 重叠 >= 0.10 作为候选

## 1.7 生命周期管理

### 有效重要度衰减公式

```
effective_importance = base_weight(importance)
                     × log(1 + access_count)
                     × 0.5^(days_since_access / 30)
                     × (1 + 0.1 × min(edge_count, 5))
```

其中 `base_weight` 映射：`imp 1→1.0, 2→1.5, 3→2.0, 4→3.0, 5→5.0`

### 免疫规则

insight 满足以下任一条件即**免疫**（不被自动清理）：
- `importance >= 4`
- `access_count >= 3`

### 自动清理

- 触发条件：insight 总数 > 1000
- 操作：软删除（设置 deleted_at），每批 10 条
- 选择顺序：effective_importance 最低的非免疫 insight

### GC 命令

- `gc --threshold 0.5`：列出 EI < 阈值的非免疫候选
- `gc --keep <id>`：提升留存（access_count += 3，刷新时间戳）

## 1.8 CLI 命令矩阵

| 命令 | 类型 | 说明 | 关键 Flag |
|------|------|------|-----------|
| `remember` | 写 | 存储新 insight | `--cat`, `--imp`, `--tags`, `--entities`, `--source` |
| `recall` | 读 | 关键词/智能召回 | `--smart`, `--intent`, `--cat`, `--limit`, `--source` |
| `search` | 读 | Token 评分搜索 | `--limit` |
| `diff` | 读 | 去重/冲突检测 | `--limit` |
| `link` | 写 | 手动创建边 | `--type`, `--weight`, `--meta` |
| `related` | 读 | BFS 图遍历 | `--edge`, `--depth` |
| `forget` | 写 | 软删除 | |
| `gc` | 生命周期 | 留存管理 | `--threshold`, `--limit`, `--keep` |
| `embed` | 向量 | 嵌入管理 | `--status`, `--all` |
| `status` | 观测 | 统计信息 | |
| `log` | 观测 | 操作审计 | `--limit` |

## 1.9 与 Claude 的交互协议

Mnemon 通过 JSON 输出与 Claude 交互，核心交互点：

### 写入时（remember 输出）

```json
{
  "id": "uuid",
  "edges_created": {"temporal": 2, "entity": 3, "causal": 1, "semantic": 0},
  "semantic_candidates": [
    {"id": "xxx", "content": "...", "token_similarity": 0.65}
  ],
  "causal_candidates": [
    {"id": "xxx", "content": "...", "hop": 1, "via_edge": "temporal",
     "causal_signal": "because", "suggested_sub_type": "causes"}
  ],
  "embedded": true,
  "effective_importance": 2.5,
  "auto_pruned": 0
}
```

Claude 根据 candidates 决定是否调用 `link` 命令建立关系。

### 读取时（recall --smart 输出）

```json
{
  "results": [
    {
      "insight": {"id": "xxx", "content": "...", ...},
      "score": 0.85,
      "intent": "WHY",
      "via": "causal",
      "signals": {"keyword": 0.8, "entity": 0.5, "similarity": 0.7, "graph": 0.9}
    }
  ],
  "meta": {
    "intent": "WHY",
    "intent_source": "auto",
    "anchor_count": 15,
    "traversed": 200,
    "hint": ""
  }
}
```

Claude 可通过 `--intent` 覆盖自动检测，通过 `signals` 字段复判排序。

[< 返回设计概览](../DESIGN.md)

---

# 3. 核心概念与架构

## 3. 核心概念

![Insight & Edge Data Model](../../diagrams/09-insight-edge-datamodel.jpg)

### 3.1 Insight（记忆节点）

Insight 是 Mnemon 中的基本记忆单元。每条 insight 代表一个独立的知识片段：

```
┌─────────────────────────────────────────────┐
│ Insight                                     │
├─────────────────────────────────────────────┤
│ id         : UUID                           │
│ content    : "选择 Qdrant 而非 Milvus..."    │
│ category   : decision                       │
│ importance : 5  (1-5)                       │
│ tags       : ["vector-db", "architecture"]  │
│ entities   : ["Qdrant", "Milvus"]           │
│ source     : "user"                         │
│ access_count        : 3                     │
│ effective_importance : 0.85                  │
│ created_at : 2026-02-18T10:00:00Z           │
└─────────────────────────────────────────────┘
```

**类别（Category）** 分为六种，帮助区分记忆的性质：

| 类别 | 含义 | 示例 |
|------|------|------|
| `preference` | 用户偏好 | "偏好使用中文交流" |
| `decision` | 架构/技术决策 | "选择 SQLite 而非 PostgreSQL" |
| `fact` | 客观事实 | "API 限流为 100 req/s" |
| `insight` | 推理结论 | "Beam search 比 full BFS 更适合…" |
| `context` | 项目上下文 | "Phase 3 已完成，118 个测试通过" |
| `general` | 通用 | 不属于以上分类的内容 |

**重要度（Importance）** 从 1 到 5，影响检索排序和生命周期：

- **5**：关键决策，永远不会被自动清理
- **4**：重要事实，免疫自动剪枝
- **3**：标准记忆
- **2**：低优先级
- **1**：临时信息，最先被清理

### 3.2 Edge（关系边）

Edge 连接两个 insight，代表它们之间的关系。每条边包含：

```
┌────────────────────────────────────────────┐
│ Edge                                       │
├────────────────────────────────────────────┤
│ source_id  : UUID  ──→  target_id : UUID   │
│ edge_type  : temporal | semantic |         │
│              causal   | entity             │
│ weight     : 0.0 ~ 1.0                    │
│ metadata   : {"sub_type": "backbone", ...} │
└────────────────────────────────────────────┘
```

四种边类型构成 MAGMA 四图模型的基础，详见[第 4 节：图模型与理论](04-graph-model.md)。

### 3.3 数据库模式

每个命名记忆体拥有独立的 SQLite 文件，位于 `~/.mnemon/data/<store>/mnemon.db`，使用 WAL 模式支持并发读取。默认记忆体为 `default`；可创建额外记忆体进行数据隔离（参见[记忆体管理](../USAGE.md#记忆体管理)）。

```sql
-- 记忆节点
insights (
  id, content, category, importance,
  tags, entities, source,
  embedding,                    -- 可选，768 维向量
  access_count, last_accessed_at,
  effective_importance,          -- 衰减后的有效重要度
  created_at, updated_at, deleted_at
)

-- 关系边（复合主键）
edges (
  source_id, target_id, edge_type,  -- PK
  weight, metadata, created_at
)

-- 操作日志（审计追踪）
oplog (
  id, operation, insight_id, detail, created_at
)
```

---

## 4. 系统架构

Mnemon 的架构分为五层：

```
┌─────────────────────────────────────────────────────────────┐
│  Integration Layer    Hook / Skill / Guide                   │
├─────────────────────────────────────────────────────────────┤
│  CLI Layer            remember, recall, diff, link, gc ...  │
├─────────────────────────────────────────────────────────────┤
│  Core Engine          search/ (recall, intent, keyword)     │
│                       graph/  (temporal, entity, causal,    │
│                                semantic)                    │
│                       embed/  (ollama, vector)              │
├─────────────────────────────────────────────────────────────┤
│  Storage Layer        store/  (db, node, edge, oplog)       │
├─────────────────────────────────────────────────────────────┤
│  External (Optional)  Ollama (localhost:11434)               │
└─────────────────────────────────────────────────────────────┘
```


**项目代码结构：**

```
mnemon/
├── cmd/                       # CLI 命令（Cobra）
│   ├── root.go                # 根命令，全局 flags，记忆体解析
│   ├── store.go               # 记忆体管理（list、create、set、remove）
│   ├── remember.go            # 存储 insight + 自动建边
│   ├── recall.go              # 检索（智能图增强，默认）
│   ├── diff.go                # 独立去重/冲突检查
│   ├── link.go                # 手动创建边
│   ├── related.go             # 从 insight 出发 BFS 遍历
│   ├── search.go              # 关键词搜索
│   ├── embed.go               # 管理 embedding
│   ├── forget.go              # 软删除 insight
│   ├── gc.go                  # 垃圾回收
│   ├── setup.go               # 部署集成（钩子、技能、引导）
│   ├── viz.go                 # 知识图谱可视化
│   ├── status.go              # 统计信息
│   └── log.go                 # 操作日志
├── internal/
│   ├── model/                 # 数据结构
│   │   ├── node.go            # Insight 定义
│   │   └── edge.go            # Edge 定义
│   ├── graph/                 # MAGMA 四图实现
│   │   ├── engine.go          # 自动建边编排器
│   │   ├── temporal.go        # 时序边
│   │   ├── entity.go          # 实体边
│   │   ├── causal.go          # 因果边
│   │   └── semantic.go        # 语义边
│   ├── search/                # 检索算法
│   │   ├── recall.go          # 意图感知多信号检索
│   │   ├── diff.go            # 内置去重检查
│   │   ├── intent.go          # 意图检测
│   │   └── keyword.go         # Token 级关键词评分
│   ├── store/                 # SQLite 持久化
│   │   ├── db.go              # 数据库初始化、事务、记忆体管理
│   │   ├── node.go            # Insight CRUD、生命周期
│   │   ├── edge.go            # Edge CRUD
│   │   └── oplog.go           # 操作日志
│   ├── embed/                 # 嵌入向量支持
│   │   ├── ollama.go          # Ollama HTTP 客户端
│   │   └── vector.go          # 向量序列化、余弦相似度
│   └── setup/                 # LLM CLI 集成部署
│       ├── claude.go          # Claude Code 部署逻辑
│       ├── openclaw.go        # OpenClaw 部署逻辑
│       ├── detect.go          # 环境检测
│       ├── prompt.go          # 提示文件部署（guide.md）
│       ├── settings.go        # 钩子注册到 settings.json
│       ├── markdown.go        # Markdown 注入/移除
│       └── assets/            # 嵌入模板（从源文件同步）
│           ├── claude/        # Claude Code 资产
│           │   ├── SKILL.md, guide.md
│           │   ├── prime.sh, user_prompt.sh
│           │   ├── stop.sh, compact.sh
│           └── openclaw/      # OpenClaw 资产
│               └── SKILL.md
├── scripts/
│   └── e2e_test.sh            # 端到端测试套件
├── main.go                    # 入口
├── CLAUDE.md                  # 项目级开发指南
└── Makefile                   # 构建、安装、测试
```

### 4.1 数据目录布局

```
~/.mnemon/
├── active                        # 当前默认记忆体名（纯文本）
├── prompt/                       # 所有记忆体共享
│   ├── guide.md                  # 行为引导（recall/remember 规则）
│   └── skill.md                  # 技能定义（命令参考）
└── data/                         # 每个记忆体拥有独立目录
    ├── default/
    │   └── mnemon.db             # SQLite 数据库（WAL 模式）
    ├── work/
    │   └── mnemon.db
    └── <name>/
        └── mnemon.db
```

**隔离边界**：每个记忆体包含独立的 `mnemon.db` — 洞察、边、操作日志完全隔离。Prompt 文件（`guide.md`、`skill.md`）共享 — 行为规则是通用的，记忆数据是私有的。

### 4.2 记忆体隔离

Mnemon 支持命名记忆体（store），为不同 agent、项目或场景提供轻量数据隔离。

**为什么用命名记忆体而非只靠 `--data-dir`？**

`--data-dir` 覆盖整个基础目录 — 需要调用者管理完整路径，语义不清晰。命名记忆体提供语义明确的标识（`MNEMON_STORE=work` 对比 `--data-dir ~/.mnemon-work`），并且天然适配环境变量 — 这是并发进程间隔离的标准机制。

**解析优先级**（从高到低）：

```
--store 标志  >  MNEMON_STORE 环境变量  >  ~/.mnemon/active 文件  >  "default"
```

分层设计服务于不同场景：

| 机制 | 场景 |
|------|------|
| `--store` 标志 | 一次性 CLI 覆盖、脚本 |
| `MNEMON_STORE` 环境变量 | 按进程隔离 — 不同 agent 使用不同记忆体 |
| `active` 文件 | 持久化用户偏好 — `mnemon store set work` |
| `"default"` | 零配置 — 开箱即用 |

**自动迁移**：当 `data/` 目录不存在但旧版 `~/.mnemon/mnemon.db` 存在时，mnemon 自动将其移动到 `data/default/mnemon.db`。老用户升级无感知。

**设计原则 — 轻量且有界**：记忆体隔离解决的是必要的数据分离需求，不会膨胀为多租户系统。没有访问控制、没有跨 store 查询、除名称外没有 store 元数据。保持功能有界 — Mnemon 是记忆守护进程，不是知识库平台。

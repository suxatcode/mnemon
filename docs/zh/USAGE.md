# Mnemon — 用法与参考

> 你不需要自己运行 mnemon 命令 — agent 会自动执行，由钩子驱动，受技能文件指引。本文档是理解 agent 能力、调试和高级手动操作的参考。

---

## CLI 命令

### 核心命令

```bash
# Remember — 存储新洞察（内置 diff：重复跳过，冲突自动替换）
mnemon remember "选择 Qdrant 而非 Milvus 做向量搜索" \
  --cat decision --imp 5 --entities "Qdrant,Milvus" --source agent

# Recall — 意图感知的图增强检索（默认）
mnemon recall "vector database" --limit 10

# Search — 基于 token 评分的关键词搜索
mnemon search "authentication" --limit 10

# Forget — 软删除洞察
mnemon forget <id>
```

### 图操作

```bash
# Link — 创建类型化边
mnemon link <source_id> <target_id> --type semantic --weight 0.85
mnemon link <source_id> <target_id> --type causal --weight 0.8 \
  --meta '{"sub_type":"causes","reason":"..."}'

# Related — 从某个洞察出发的 BFS 遍历
mnemon related <id> --edge causal --depth 2
```

### 生命周期管理

```bash
# GC — 查看低保留度候选
mnemon gc --threshold 0.5

# GC keep — 提升某个洞察的保留度
mnemon gc --keep <id>
```

### 记忆体管理

Mnemon 支持命名记忆体（store）进行数据隔离。每个记忆体拥有独立的数据库。

```bash
# 列出所有记忆体（* 标记当前活跃的）
mnemon store list

# 创建新记忆体
mnemon store create work

# 切换默认活跃记忆体
mnemon store set work

# 删除记忆体（不可删除当前活跃的）
mnemon store remove old-project
```

**记忆体解析优先级**（从高到低）：

1. `--store <name>` CLI 标志
2. `MNEMON_STORE` 环境变量
3. `~/.mnemon/active` 文件
4. 回退到 `"default"`

不同 agent 或进程可通过 `MNEMON_STORE` 环境变量使用不同记忆体 — 无全局状态竞争。旧版数据库（`~/.mnemon/mnemon.db`）在首次运行时自动迁移到 `~/.mnemon/data/default/`。

### 可观测性

```bash
mnemon status    # 记忆统计
mnemon log       # 操作日志
```

### 可视化

导出知识图谱进行可视化探索：

```bash
# DOT 格式 — 使用 Graphviz 渲染（brew install graphviz）
mnemon viz --format dot -o graph.dot
dot -Tpng graph.dot -o graph.png

# 交互式 HTML — 直接在浏览器中打开（vis.js，无需安装）
mnemon viz --format html -o graph.html
open graph.html
```

节点按分类着色（decision、fact、insight、preference、context），边按类型着色（temporal、semantic、causal、entity）。

---

## 嵌入向量支持（可选）

Mnemon 无需 Ollama 即可完整运行 — 所有核心功能（remember、recall、link、图遍历）开箱即用。添加 Ollama 可通过向量相似度增强召回精度，但从不是必需的。

### 有无嵌入的对比

| 能力 | 无 Ollama | 有 Ollama |
|---|---|---|
| **召回锚点** | 关键词 + 时间 | 关键词 + 向量 + 时间（RRF 混合） |
| **语义边** | Token 重叠（较粗） | 余弦相似度 ≥ 0.50（精确） |
| **遍历评分** | 纯结构分 | 结构 + 语义 |
| **重排序权重** | 关键词 45%、实体 25%、图 30% | 关键词 30%、实体 15%、相似度 35%、图 20% |

Ollama 不可用时，重排序系统自动将相似度权重重新分配给关键词和图信号 — 无需配置，无降级模式标志。系统在运行时以 2 秒超时检测 Ollama 可用性。

### 安装

```bash
brew install ollama              # 或参见 https://ollama.ai
ollama pull nomic-embed-text     # 下载嵌入模型
```

验证：

```bash
mnemon embed --status
```

```json
{
  "total_insights": 87,
  "embedded": 87,
  "coverage": "100%",
  "ollama_available": true,
  "model": "nomic-embed-text"
}
```

### 回填已有洞察

如果在使用 mnemon 之后才安装 Ollama，已有洞察不会有嵌入向量。一条命令即可回填：

```bash
mnemon embed --all
```

这会为所有未嵌入的洞察生成嵌入向量并自动创建语义边。可在前后使用 `mnemon embed --status` 检查覆盖率。

---

## 架构

```
┌──────────────────┐     CLI commands      ┌──────────────────┐
│   LLM Agent      │ ───────────────────── │     Mnemon       │
│ (Claude Code,    │  remember, recall,    │                  │
│  Cursor, etc.)   │  link, forget, gc     │  SQLite (WAL)    │
└──────────────────┘                       │  ┌────────────┐  │
                                           │  │ Insights   │  │
        The LLM decides WHAT               │  ├────────────┤  │
        to remember and link.              │  │ 4 Edge     │  │
                                           │  │ Types:     │  │
        Mnemon handles HOW                 │  │ temporal   │  │
        to store, index, and               │  │ entity     │  │
        retrieve.                          │  │ causal     │  │
                                           │  │ semantic   │  │
      ┌──────────────────┐                 │  ├────────────┤  │
      │  Ollama          │  (optional)     │  │ Embeddings │  │
      │  nomic-embed-text│ ◄───────────── │  └────────────┘  │
      └──────────────────┘                 └──────────────────┘
```

受 [MAGMA](https://arxiv.org/abs/2601.03236) 四图模型启发。详见[设计与架构](DESIGN.md)。

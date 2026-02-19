# Mnemon

**LLM 智能体的持久记忆系统** — LLM 监督式、技能集成、四图架构。

[![Go 1.24+](https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![CI](https://github.com/Grivn/mnemon/actions/workflows/ci.yml/badge.svg)](https://github.com/Grivn/mnemon/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](../../LICENSE)

---

LLM 智能体在会话之间会遗忘一切。上下文压缩丢失关键决策，跨会话知识消失，长对话将早期信息推出窗口。

Mnemon 为你的 LLM 提供持久的跨会话记忆 — 只需一个 Go 二进制文件和一个技能文件。

### 为什么选择 Mnemon？

记忆具有**复利效应** — 积累越久，价值越大。LLM 引擎不断迭代，技能文件几乎零成本编写，但记忆是随用户一起增长的私有资产。它是智能体生态中唯一值得深度投入的组件。

Mnemon 基于一个核心理念：**LLM 本身就是最好的编排器。** 不在管线内嵌入小型 LLM，而是让你的宿主 LLM — 对话中已有完整上下文的那个 — 充当监督者。二进制是器官（确定性存储、图索引、搜索、衰减）；LLM 是大脑（决定记什么、怎么关联、何时遗忘）。技能文件是教授协议的教科书。

这意味着：**记忆管理逻辑从提示词迁移到代码 — 确定性的、可测试的、可移植的。** 同一套二进制 + 技能文件可在 Claude Code、Cursor 或任何读取 markdown 的 LLM CLI 上运行。

| 模式 | LLM 角色 | 代表项目 |
|---|---|---|
| **LLM-Embedded** | 管线内部的执行者 | Mem0, MAGMA |
| **MCP Server** | 通过 MCP 协议提供工具 | MemCP |
| **LLM-Supervised** | 独立二进制的外部监督者 | Mnemon |

详见 [设计与架构](DESIGN.md)。

## 快速开始

```bash
git clone https://github.com/Grivn/mnemon.git && cd mnemon
make setup          # 构建 + 安装二进制 + 技能 + 钩子
make claude-inject  # 注入记忆引导到 ./CLAUDE.md
```

就这样。启动新的 Claude Code 会话 — 钩子自动召回相关记忆，技能文件教授命令语法，CLAUDE.md 引导何时记忆。

移除 CLAUDE.md 中的记忆引导：`make claude-eject`。

## 工作原理

Mnemon 分三层：

**二进制** — Go CLI + SQLite 存储。处理持久化、图索引、关键词搜索、嵌入向量、保留度衰减。内部无 LLM、无 API 密钥、无网络调用。

**三个集成层**教会 LLM 使用二进制：

| 层 | 职责 | 方式 |
|---|---|---|
| **[Hook (recall)](../../scripts/hooks/user_prompt.sh)** | 自动召回 | 每条用户消息运行 `mnemon recall`，将结果注入 LLM 上下文 |
| **[Hook (stop)](../../scripts/hooks/stop.sh)** | 记忆提醒 | 每次回复后，提醒 LLM 考虑是否需要记忆 |
| **[CLAUDE.md](../../CLAUDE.md)** | 行为引导 | 告诉 LLM *何时*使用记忆、*何时*存储新记忆 |
| **[Skill](../../skills/mnemon/SKILL.md)** | 命令参考 | 记录命令语法、分类、工作流 |

```
用户消息
    │
    ▼
  Hook ─── 自动召回 ──→ [过往记忆] 注入上下文
    │
    ▼
  CLAUDE.md ── "使用过往记忆；回复后评估是否记忆"
    │
    ▼
  Skill ── "方法：mnemon remember --cat ...（diff 内置）"
    │
    ▼
  Sub-agent ── 主 LLM 委派；sub-agent 读取 Skill 并执行命令
```

### 为什么这样设计？

- **Hook 可靠处理召回** — 无需 LLM 主动发起，记忆自动出现在每次对话中
- **CLAUDE.md 权限最高** — 项目级指令，LLM 遵循度高于工具文档
- **Skill 保持专注** — 纯命令参考，不混入行为逻辑
- **Sub-agent 隔离开销** — 记忆写入在轻量 sub-agent（~1000 tokens）中执行，而非主对话（~25000 tokens）

### 适配其他 LLM CLI

对于非 Claude Code 工具，将三层合并到系统提示词或规则文件中：将召回逻辑、行为引导和命令参考复制到 `.cursorrules`、`RULES.md` 或等效文件。

## 特性

- **LLM 监督式** — 宿主 LLM 主动决定记什么、更新什么、关联什么、遗忘什么；无内嵌 LLM，无额外 API 调用
- **技能集成** — 一个技能文件教会任何 LLM CLI 完整的命令协议；适用于 Claude Code、Cursor 或任何读取 markdown 的工具
- **四图架构** — 时序、实体、因果、语义四种边，不仅仅是向量相似度
- **意图感知召回** — 图遍历 + 可选向量搜索（RRF 融合），所有查询默认启用
- **内置去重** — `remember` 自动检测重复和冲突；跳过或自动替换
- **保留度生命周期** — 重要性衰减、访问计数提升、免疫规则、垃圾回收
- **可选嵌入向量** — 本地 Ollama 集成，支持混合向量+关键词搜索
- **图谱可视化** — 导出为 Graphviz DOT 或交互式 vis.js HTML

## 用法

### 核心命令

```bash
# Remember — 存储新洞察（内置 diff：重复跳过，冲突自动替换）
mnemon remember "选择 Qdrant 而非 Milvus 做向量搜索" \
  --cat decision --imp 5 --entities "Qdrant,Milvus" --source agent

# Recall — 意图感知的图增强检索（默认）
mnemon recall "vector database" --limit 10

# Search — 基于 token 评分的关键词搜索
mnemon search "authentication" --limit 10

# Diff — 独立的重复/冲突检查（可选；remember 已内置此功能）
mnemon diff "待检查的新事实"

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

### 嵌入向量（可选）

需要 [Ollama](https://ollama.ai) 和 `nomic-embed-text`：

```bash
ollama pull nomic-embed-text

mnemon embed --status    # 查看嵌入覆盖率
mnemon embed --all       # 批量生成所有洞察的嵌入
mnemon embed <id>        # 为单个洞察生成嵌入
```

嵌入向量可用时，`recall` 自动使用混合向量+关键词搜索（RRF 融合）。

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

## 配置

| 环境变量 | 默认值 | 说明 |
|---------|-------|------|
| `MNEMON_DATA_DIR` | `~/.mnemon` | 数据库目录 |
| `MNEMON_EMBED_ENDPOINT` | `http://localhost:11434` | Ollama API 端点 |
| `MNEMON_EMBED_MODEL` | `nomic-embed-text` | 嵌入模型名称 |

或在任何命令上使用 `--data-dir` 标志。

## 开发

```bash
make build          # 构建二进制
make install        # 构建 + 安装到 $GOBIN
make test           # 运行 E2E 测试套件
make setup          # 完整设置（二进制 + 技能 + 钩子）
make eject          # 移除技能
make eject-hooks    # 从 Claude Code 设置中移除钩子
make uninstall      # 移除所有
make help           # 显示所有目标
```

**依赖**：Go 1.24+、`modernc.org/sqlite`、`spf13/cobra`、`google/uuid`

**可选**：[Ollama](https://ollama.ai) + `nomic-embed-text` 嵌入支持

## 文档

- [设计与架构](DESIGN.md) — 核心概念、四图模型、LLM 监督式架构、算法、设计决策
- [架构图](../diagrams/) — 系统架构、记忆/召回流程、四图模型、生命周期管理（drawio + 导出图片）

## 许可证

[MIT](../../LICENSE)

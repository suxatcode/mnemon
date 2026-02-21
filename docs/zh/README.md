<p align="center">
  <img src="../logo/logo.svg" width="160" height="160" alt="Mnemon Logo" />
</p>

# Mnemon

**LLM 智能体的持久记忆系统** — LLM 监督式、钩子集成、四图架构。

[![Go 1.24+](https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![CI](https://github.com/mnemon-dev/mnemon/actions/workflows/ci.yml/badge.svg)](https://github.com/mnemon-dev/mnemon/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](../../LICENSE)

---

LLM 智能体在会话之间会遗忘一切。上下文压缩丢失关键决策，跨会话知识消失，长对话将早期信息推出窗口。

Mnemon 为你的 LLM 提供持久的跨会话记忆 — 只需一个 Go 二进制文件和一条 setup 命令。

<p align="center">
  <img src="../diagrams/10-knowledge-graph.jpg" width="720" alt="知识图谱 — 87 条洞察通过时序、实体、语义和因果边连接" />
  <br />
  <sub>Mnemon 构建的真实知识图谱 — 87 条洞察，2150 条边，横跨四种图类型。</sub>
</p>

### 为什么选择 Mnemon？

记忆具有**复利效应** — 积累越久，价值越大。LLM 引擎不断迭代，技能文件几乎零成本编写，但记忆是随用户一起增长的私有资产。它是智能体生态中唯一值得深度投入的组件。

Mnemon 基于一个核心理念：**LLM 本身就是最好的编排器。** 不在管线内嵌入小型 LLM，而是让你的宿主 LLM — 对话中已有完整上下文的那个 — 充当监督者。二进制是器官（确定性存储、图索引、搜索、衰减）；LLM 是大脑（决定记什么、怎么关联、何时遗忘）。技能文件是教授协议的教科书。

这意味着：**记忆管理逻辑从提示词迁移到代码 — 确定性的、可测试的、可移植的。** 同一套二进制 + 技能文件可在 Claude Code、Cursor 或任何读取 markdown 的 LLM CLI 上运行。

| 模式 | LLM 角色 | 代表项目 |
|---|---|---|
| **LLM-Embedded** | 管线内部的执行者 | Mem0, Letta |
| **MCP Server** | 通过 MCP 协议提供工具 | claude-mem |
| **LLM-Supervised** | 独立二进制的外部监督者 | Mnemon |

<p align="center">
  <img src="../diagrams/llm-supervised-concept.jpg" width="720" alt="LLM 监督式架构 — 三种模式对比，及 Mnemon 实现细节：钩子、大脑/器官分离、Sub-agent 委派" />
  <br />
  <sub>LLM 监督式模式：钩子驱动生命周期，宿主 LLM 做判断，二进制处理确定性计算。</sub>
</p>

详见 [设计与架构](DESIGN.md)。

## 快速开始

### Claude Code

```bash
go install github.com/mnemon-dev/mnemon@latest
mnemon setup
```

`mnemon setup` 自动检测 Claude Code，交互式部署技能文件、钩子和行为引导。启动新会话 — 记忆自动运作。

### OpenClaw

```bash
go install github.com/mnemon-dev/mnemon@latest
mnemon setup --target openclaw
```

部署技能文件和行为引导到 `~/.mnemon/prompt/guide.md`。由于 OpenClaw 的钩子集成暂未自动化，需要手动配置：

> 阅读 `~/.mnemon/prompt/guide.md` 并按照其 recall/remember 工作流配置自身。

### 从源码构建

```bash
git clone https://github.com/mnemon-dev/mnemon.git && cd mnemon
make install && mnemon setup
```

### 卸载

```bash
mnemon setup --eject
```

## 工作原理

设置完成后，记忆透明运作 — 你照常使用 LLM CLI。Mnemon 通过 Claude Code 的[钩子系统](https://docs.anthropic.com/en/docs/claude-code/hooks)集成，在关键生命周期节点注入记忆操作：

```
会话启动
    │
    ▼
  Prime（SessionStart）─── prime.sh ──→ 加载 guide.md（记忆执行手册）
    │
    ▼
  用户发送消息
    │
    ▼
  Remind（UserPromptSubmit）─── user_prompt.sh ──→ 提醒 agent 进行 recall 和 remember
    │
    ▼
  LLM 生成回复（遵循技能文件 + guide.md 规则）
    │
    ▼
  Nudge（Stop）─── stop.sh ──→ 提醒 agent 进行 remember
    │
    ▼
  （上下文压缩时）
  Compact（PreCompact）─── compact.sh ──→ 提取关键洞察进行 remember
```

四个钩子驱动记忆生命周期。**Prime** 加载行为引导 — 详细的 recall、remember、sub-agent 委派执行手册。**Remind** 在工作开始前提醒 agent 评估是否需要 recall 和 remember。**Nudge** 在工作结束后提醒 agent 考虑 remember。**Compact** 在上下文压缩前指示 agent 提取并保存关键洞察。**技能文件**教会 agent 命令语法。**行为引导**（`~/.mnemon/prompt/guide.md`）定义 recall、remember、委派的详细规则。

你不需要自己运行 mnemon 命令。agent 会自动执行 — 由钩子驱动，受技能文件和行为引导指引。

## 特性

- **零用户操作** — 安装一次，记忆通过钩子在后台运行
- **LLM 监督式** — 宿主 LLM 主动决定记什么、更新什么、遗忘什么；无内嵌 LLM，无 API 密钥
- **钩子集成** — 四个生命周期钩子：Prime（加载引导）、Remind（recall 和 remember）、Nudge（remember）、Compact（压缩前保存）
- **四图架构** — 时序、实体、因果、语义四种边，不仅仅是向量相似度
- **意图感知召回** — 图遍历 + 可选向量搜索（RRF 融合），所有查询默认启用
- **内置去重** — `remember` 自动检测重复和冲突；跳过或自动替换
- **保留度生命周期** — 重要性衰减、访问计数提升、免疫规则、垃圾回收
- **可选嵌入向量** — 本地 [Ollama](https://ollama.ai) 集成，支持混合向量+关键词搜索

## 愿景

所有本地 AI 智能体 — 跨会话、跨框架 — 共享一个活跃的记忆池。

```
  Claude Code ──┐
                │
  OpenClaw ─────┤
                ├──▶  ~/.mnemon  ◀── 共享记忆
  OpenCode ─────┤
                │
  Gemini CLI ───┘
```

基础已就绪：一个 `~/.mnemon` 数据库，任何 agent 都可以读写。Claude Code 的钩子集成是参考实现 — 同样的模式（生命周期钩子 + 技能文件 + 行为引导）可以复制到任何支持事件钩子或系统提示的 LLM CLI。

## 常见问题

**不同会话共享记忆吗？**
是的。所有会话使用同一个 `~/.mnemon` 数据库 — 一个会话中记住的决策在所有未来会话中可用。

**本地模式还是全局模式？**
`mnemon setup` 默认**本地**（项目级 `.claude/`），适合大多数用户。**全局**（`mnemon setup --global`，安装到 `~/.claude/`）在所有项目中激活 mnemon — 如果想让其他框架（如 OpenClaw）通过 Claude Code CLI 共享记忆很方便，但可能增加维护开销。

**如何自定义行为？**
编辑 `~/.mnemon/prompt/guide.md`。该文件控制 agent 何时召回记忆以及什么值得记住。技能文件（`SKILL.md`）由 setup 自动部署，通常无需手动编辑。

**什么是 Sub-agent 委派？**
记忆写入不在主对话中进行。宿主 LLM（如 Opus）决定*记什么*，然后委派实际的 `mnemon remember` 执行给轻量 sub-agent（如 Sonnet）。这节省 token 并保持记忆操作不污染主上下文。

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
mnemon setup        # 交互式设置（检测环境 + 部署钩子/技能/引导）
mnemon setup --eject  # 移除所有集成
make help           # 显示所有目标
```

**依赖**：Go 1.24+、`modernc.org/sqlite`、`spf13/cobra`、`google/uuid`

**可选**：[Ollama](https://ollama.ai) + `nomic-embed-text` 嵌入支持

## 文档

- [设计与架构](DESIGN.md) — 核心概念、四图模型、LLM 监督式架构、算法、集成设计
- [用法与参考](USAGE.md) — CLI 命令、嵌入向量支持、架构概览
- [架构图](../diagrams/) — 系统架构、记忆/召回流程、四图模型、生命周期管理

## 许可证

[MIT](../../LICENSE)

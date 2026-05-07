# ALMA 概览

## 命名说明

本次调研中存在两个相关但不同的 ALMA：

1. **ALMA meta-learning memory design**：论文/源码 `zksha/alma`，全称 Automated meta-Learning of Memory designs for Agentic systems，目标是让系统自动搜索更好的 memory structure。
2. **ALMA-memory library**：`RBKunnela/ALMA-memory` 风格的工程库，目标是给 agent 提供 persistent memory、heuristics、anti-pattern、multi-agent sharing、verified retrieval。

两者都纳入本文，但它们不是同一个系统。

## ALMA meta 架构

本地源码：`/tmp/mnemon-agent-research-sources/alma-meta`

关键文件：

| 位置 | 观察 |
|---|---|
| `core/meta_agent.py` | `MetaAgent` 驱动 analyze -> generate code -> examine -> evaluate |
| `core/meta_agent_prompt.py` | 构造 analysis prompt、generate code prompt、reflection prompt |
| `core/memo_manager.py` | 保存 LLM 生成的 `memo_structure_<sha>.py`，执行评估并管理 reward |
| `evals/agents/memo_structure.py` | 定义 `Sub_memo_layer` 与 `MemoStructure` 抽象 |
| `evals/workflows/agent_workflow.py` | 执行 retrieve/update 评估流程 |

ALMA meta 的核心不是「记忆内容演化」，而是「记忆结构代码演化」。

```text
current memo structure
  -> evaluate trajectory
  -> LLM analysis
  -> LLM generates new memory structure code
  -> execute in container
  -> repair if failed
  -> evaluate reward
  -> archive candidate
```

## ALMA-memory library 架构

本地源码：`/tmp/mnemon-agent-research-sources/alma-memory`

关键能力：

- retrieve before task；
- learn after task outcome；
- memory types：heuristic、outcome、user preference、domain knowledge、anti-pattern；
- similar outcomes 触发 heuristic；
- repeated failures 触发 anti-pattern；
- multi-agent sharing；
- trust/verification；
- `MemorySlice.to_prompt()` 注入 context；
- MCP / Python / TypeScript SDK。

它是库式 memory layer，而不是 agent runtime。

## 与 Mnemon 的关系

ALMA meta 对 Mnemon 是长期研究方向：如果未来 Mnemon 要自动搜索不同 memory graph/schema/retrieval policy，ALMA meta 是参考。但当前阶段它太重。

ALMA-memory 对 Mnemon 是功能对比：typed memories、retrieval feedback、verified retrieval、anti-pattern 都值得参考，但其库式集成比 Mnemon 目标更侵入。

## 参考来源

- 本地源码: `alma-meta/core/meta_agent.py`
- 本地源码: `alma-meta/core/meta_agent_prompt.py`
- 本地源码: `alma-meta/core/memo_manager.py`
- 本地源码: `alma-memory/README.md`
- 本地源码: `alma-memory/alma/core.py`
- 论文: [Learning to Continually Learn via Meta-learning Agentic Memory Designs](https://arxiv.org/abs/2602.07755)

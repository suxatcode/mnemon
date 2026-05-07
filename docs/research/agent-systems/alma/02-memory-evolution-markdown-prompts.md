# ALMA 的记忆、演化与 Prompt 用法

## ALMA meta 的记忆演化

ALMA meta 的演化对象是 memory design 本身：

- prompt 要求 LLM 分析当前 memory structure；
- 当前 structure 由多个 `Sub_memo_layer` 组成；
- 每层有 `Retrieve` 和 `Update`；
- `MemoStructure` 有 general retrieve/update orchestration；
- LLM 生成新的 Python code；
- `Memo_Manager` 保存并执行候选代码；
- 失败后通过 reflection prompt 修复；
- 评估 reward 后进入 archive。

这是一种「meta-evolution」：不是记住更多 facts，而是改进 memory 机制。

## ALMA-memory 的记忆处理

ALMA-memory 的典型循环是：

```text
task
  -> retrieve relevant memories
  -> agent executes task
  -> learn outcome / strategy / failure
  -> update heuristics or anti-patterns
  -> future retrieval improves
```

它强调：

- scoped learning；
- outcome-based memory；
- failure anti-pattern；
- trust scoring；
- feedback-aware reranking；
- verified retrieval；
- multi-agent sharing。

## Markdown 用法

ALMA meta 中 Markdown 主要是 prompt/文档载体，不是主要 runtime behavior artifact。LLM 输出会从 Markdown code fence 中提取 Python code，再保存为 `memo_structure_<sha>.py`。

ALMA-memory 文档站和 guide 使用 Markdown，但 runtime 主要是 Python/TypeScript SDK、MCP tools、structured memory objects，而不是 `SKILL.md` 风格。

## 特殊 prompt

`core/meta_agent_prompt.py` 中的 prompt 有几个模式：

- 把 LLM 设为 Senior Agent Construction Engineer；
- 给出任务类型和当前 memory structure 源码；
- 要求输出结构化 analysis schema；
- 生成新代码时给出 base `memo_structure.py` 模板和约束；
- reflection prompt 使用执行错误修复代码。

这类 prompt 强约束、多阶段、面向代码生成。它适合 memory architecture search，不适合 Mnemon 第一阶段的轻量 harness。

## 对 Mnemon 的设计判断

ALMA 提醒我们：memory-driven self-evolution 有两种层级：

1. **行为资产演化**：skills、guidelines、install notes、rules。适合 Mnemon 当前阶段。
2. **记忆机制演化**：schema、retrieval layer、update algorithm、reward loop。适合未来研究阶段。

Mnemon 当前不应直接做 ALMA meta 式代码自演化。更现实的是：

- 先让 agent 用 Mnemon recall/remember/link 积累 evidence；
- 将 repeated procedures 变成 Markdown candidate；
- review 后安装；
- 等行为层稳定后，再评估是否需要 meta-search memory engine。

## 参考来源

- 本地源码: `alma-meta/core/meta_agent_prompt.py`
- 本地源码: `alma-meta/core/meta_agent.py`
- 本地源码: `alma-meta/core/memo_manager.py`
- 本地源码: `alma-memory/alma/learning/protocols.py`
- 本地源码: `alma-memory/alma/retrieval/engine.py`
- 本地源码: `alma-memory/alma/types.py`
- 论文: [ALMA paper page](https://arxiv.org/abs/2602.07755)

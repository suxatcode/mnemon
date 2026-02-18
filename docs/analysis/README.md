# Mnemon 架构分析报告

> 学术成果到工业实现的转化分析
>
> 基于 MAGMA 论文 (arXiv:2601.03236)、MAMGA 官方实现、memcp 实现的深度对比

## 目录

1. [当前项目实现架构](./01-implementation-architecture.md)
2. [典型场景时序图](./02-sequence-diagrams.md)
3. [与 MAGMA 论文的对比分析](./03-magma-paper-comparison.md)
4. [与 MAMGA/memcp 实现的对比分析](./04-implementation-comparison.md)
5. [工业化简化的收益与风险评估](./05-tradeoff-assessment.md)

## 一句话总结

Mnemon 是 MAGMA 论文的 **工业化重构实现**：保留了四图架构的核心骨架（temporal / entity / causal / semantic），将 LLM 能力从管道内部提升到引擎层——CLI（Claude Code）就是 mnemon 的 LLM 引擎，通过 CLI-in-the-loop 机制实现实体补充、因果评估、语义判断等所有需要 LLM 的操作。二进制本身用 regex + 字典处理高频低成本的自动化边生成，将高价值判断委托给引擎层的 LLM（通常是 Opus/Sonnet 级别，能力远超 MAGMA 使用的 gpt-4o-mini）。这不是"去掉 LLM"，而是"把 LLM 放在更合适的位置"。

## 引用

- **MAGMA 论文**: Dongming Jiang et al., "MAGMA: A Multi-Graph based Agentic Memory Architecture for AI Agents", arXiv:2601.03236, Jan 2026
- **MAMGA 实现**: https://github.com/FredJiang0324/MAMGA (Python, NetworkX + FAISS)
- **memcp 实现**: https://github.com/maydali28/memcp (Python, FastMCP + SQLite)

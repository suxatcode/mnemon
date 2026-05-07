# Hermes memory lifecycle 细节

## 核心判断

Hermes 是最接近 Mnemon 当前思路的系统：bounded Markdown facts、skills as procedures、session search for ephemeral history、background curator for skill library。它没有把记忆系统做成厚重数据库 adapter，而是让 agent 通过 Markdown 和工具自己维护行为资产。

这与 Mnemon 的目标高度一致：`GUIDELINE.md` 负责初始行为原则，`INSTALL.md` 说明如何安装 hooks，`SKILL.md` 承载 workflow，memory 只保存 durable facts。

## 生命周期详表

| 维度 | 观察 |
|---|---|
| 主要记忆载体 | `~/.hermes/memories/MEMORY.md` 和 `~/.hermes/memories/USER.md`。 |
| 文件语义 | `MEMORY.md` 存环境、项目、事实、决策；`USER.md` 存用户偏好和画像。 |
| 长度限制 | `MEMORY.md` 默认 2,200 chars，约 800 tokens；`USER.md` 默认 1,375 chars，约 500 tokens。 |
| 条目格式 | 条目用 `§` 分隔；文件 header 显示 usage percent 和 char count。 |
| 加载时机 | session start 注入为 frozen prompt snapshot；session 中 memory 变化持久化，但不会改变当前已缓存 system prefix。 |
| 写路径 | agent 使用 `memory` tool 的 add/replace/remove；没有独立 read action，因为读取来自 session start snapshot。 |
| 超出处理 | add 超限会返回错误、当前 entries 和 usage；agent 应 consolidate、replace 或 remove 后再添加。 |
| 整理建议 | 文档建议超过 80% capacity 时 consolidation；流程和过程不放 memory，转入 skills。 |
| 重复处理 | exact duplicate 会被拒绝。 |
| 安全处理 | memory tool 有 prompt injection、exfiltration、invisible unicode 等扫描。 |
| 历史召回 | `session_search` 使用 SQLite FTS5 与 LLM summarization，面向过去 session，不等同 durable memory。 |
| skill 存储 | `~/.hermes/skills/<skill>/SKILL.md`，可带 references/templates/scripts/assets。 |
| skill 限制 | self-evolution repo 中 skills 目标 <=15KB；tool descriptions <=500 chars；parameter descriptions <=200 chars；优化有增长惩罚。 |
| 定时任务 | v0.12.0 引入 Autonomous Curator，gateway cron ticker 驱动，默认 7-day cycle，负责评估、合并、修剪 skill library。 |

## 写入规则

Hermes prompt 明确区分三类信息：

- durable facts：写 `MEMORY.md` 或 `USER.md`。
- procedures/workflows：写 skill。
- temporary progress/session outcomes/TODO：不要写 durable memory，需要时用 session search。

这正是 Mnemon 需要的分层。尤其是「用户纠正」「工具坑点」「稳定偏好」「环境事实」可以进 memory；「如何执行某类任务」必须进 skill；「本轮做到哪里」只作为短期状态或 session artifact。

## 溢出与 consolidation

Hermes 的溢出处理很直接：

1. 尝试 add memory。
2. 如果超过字符上限，tool 返回错误和当前 memory 状态。
3. agent 选择 replace/remove/consolidate。
4. 再次 add 更短、更稳定的表述。

这比后台自动改写更容易审计。Mnemon 可以采用同类策略：memory store 给出 hard cap 或 soft cap；超过阈值时不自动塞入，而是要求 agent 输出 consolidation patch。

## Skills 与渐进披露

Hermes skills 是 procedural memory：

```text
~/.hermes/skills/<skill>/
  SKILL.md
  references/
  templates/
  scripts/
  assets/
```

它采用 progressive disclosure：

- Level 0：`skills_list()` 只给 skill 列表，约 3k tokens。
- Level 1：`skill_view(name)` 读取完整 `SKILL.md`。
- Level 2：`skill_view(name, path)` 读取引用文件。

这对 Mnemon 很重要：`GUIDELINE.md` 不应包含所有细节；INSTALL 只说明如何安装；具体 workflow 放 skill 并按需打开。

## 定时 curator

Hermes v0.12.0 的 Autonomous Curator 是 self-evolution 的工程化版本：

- gateway cron ticker 触发；
- 默认 7 天周期；
- 后台 agent 检查 skill library；
- 合并相近 skills、修剪无效 skills、输出 `logs/curator/run.json` 与 `REPORT.md`；
- 运行时 self-improvement loop 在每轮后判断是否保存/更新 memory 或 skill。

这个机制适合长期运行的 Hermes，但 Mnemon 第一阶段不需要默认开启。更合理的是在 INSTALL 中把它定义为可选维护任务：例如每周让 agent 运行一次 `mnemon review`，生成可审查 diff。

## 对 Mnemon 的启发

Hermes 给 Mnemon 的直接模板：

```text
bounded fact memory
  + skill procedures
  + session search for old transcripts
  + reviewed markdown edits
  + optional scheduled curator
```

具体建议：

- `GUIDELINE.md` 写「什么该记、什么不该记、如何提议修改」。
- `INSTALL.md` 写「四个 hook 阶段怎么安装、每个 hook 做什么」。
- hook 产出候选，不直接无限追加 memory。
- 超过 80% 进入整理模式。
- workflow 一律沉淀成 skill，不写 fact memory。

## 参考来源

- 公开站点: [Hermes Agent](https://hermes-ai.net/)
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent/website/docs/user-guide/features/memory.md`
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent/website/docs/user-guide/features/skills.md`
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent/agent/prompt_builder.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent/tools/memory_tool.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent/RELEASE_v0.12.0.md`
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent-self-evolution/README.md`
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent-self-evolution/PLAN.md`

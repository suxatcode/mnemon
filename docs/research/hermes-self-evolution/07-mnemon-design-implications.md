# 对 Mnemon 的设计启示

## 结论

基于 Hermes 为主、OpenClaw 和 Claude Code 为辅的调研，Mnemon 当前最合理的方向是：

```text
Markdown-first
Everything is skill
Hook-installed
Hot/cold memory split
Proposal-first evolution
Filesystem as reviewable control plane
Index/model as cold-memory capacity layer
```

这与用户提出的方向一致：harness framework 本身不需要一开始做复杂 adapter，大多数能力通过 skill、`INSTALL.md`、`GUIDELINE.md` 和 hooks 表达即可。

## 一句话架构

Mnemon 应该被设计成一个“可由 agent 安装的自进化行为层”，而不是一个“需要所有 agent 接入的记忆数据库”。

更具体地说：

```text
INSTALL.md 告诉 agent 如何安装 Mnemon
GUIDELINE.md 告诉 agent 什么该记、怎么演化、什么不能动
skills/ 表达具体能力
hooks/ 在关键阶段 nudge/remind/flush/review
memory/hot/ 给模型直接读
memory/warm/ 保存整理后的 topic/session capsules
memory/cold/ 保存长期 evidence 和索引
reports/ 保存所有维护动作
```

## 文档与目录建议

建议 Mnemon 设计文档最终收敛成一个主设计文档，但实现 artifact 可以保持分层。

```text
mnemon/
  INSTALL.md
  GUIDELINE.md
  skills/
    recall/
    reflect/
    curate/
    install-hooks/
  memory/
    hot/
    warm/
    cold/
  hooks/
    recall.md
    observe.md
    reflect.md
    curate.md
  reports/
    review/
    curator/
```

如果要保持极简，也可以先只定义：

```text
INSTALL.md
GUIDELINE.md
skills/
reports/
```

并把 memory 目录作为可选进阶安装项。

## INSTALL.md 应写什么

`INSTALL.md` 的目标不是“解释 Mnemon 理论”，而是让目标 agent 能把自己接入 Mnemon。

建议包含：

1. 平台识别：Hermes、Claude Code、Codex、OpenClaw 或 generic agent。
2. 四类 hook：recall、observe、reflect、curate。
3. 每类 hook 的输入、输出、预算和权限。
4. 哪些文件要加载为 guideline。
5. 哪些 skill 要安装。
6. 哪些任务需要 scheduled/idle trigger。
7. 如何运行 dry-run。
8. 如何查看 reports。
9. 如何禁用和回滚。

最小安装可以只做：

```text
1. 把 GUIDELINE.md 加入 agent 的项目指令。
2. 把 skills/ 注册为可发现 skill。
3. 安装 session-start recall hook。
4. 安装 session-end reflect hook。
5. 维护动作默认只写 reports/，不直接改 hot memory。
```

## GUIDELINE.md 应写什么

`GUIDELINE.md` 是初始行为准则，不应写成长篇论文。它应该告诉 agent：

| 主题 | 规则 |
|---|---|
| 记什么 | 稳定事实、用户偏好、项目约定、重复工具坑点 |
| 不记什么 | 一次性任务进度、临时 TODO、未确认推断、secrets |
| memory vs skill | facts/preferences 进 memory，procedures/workflows 进 skill |
| 当前指令优先 | 旧记忆不能覆盖当前用户请求 |
| proposal-first | 持久修改先写 proposal/report |
| evidence | 重要记忆要关联来源 |
| size budget | hot memory 超预算时先整理再写入 |
| curation | 合并窄 skill，archive 不 delete，pinned 不动 |

这份 guideline 应被安装到目标 agent 最容易稳定读取的位置，例如 Claude Code 的 `CLAUDE.md`/rules、Hermes 的 context/guidance、OpenClaw bootstrap files、Codex 的 `AGENTS.md` 或 skill。

## Skill 体系建议

Mnemon 的核心 skill 不应太多。建议第一批：

| Skill | 作用 |
|---|---|
| `mnemon-install` | 根据 `INSTALL.md` 为当前 agent 安装 hook/guideline |
| `mnemon-recall` | 根据当前任务召回 hot/warm/cold 相关内容 |
| `mnemon-reflect` | 在任务结束时提出 memory/skill 更新 |
| `mnemon-curate` | 合并、demote、archive 记忆和 skill |
| `mnemon-research` | 做外部系统调研时保存 evidence 与 source map |

每个 skill 应保持 class-level，不要为每个项目或每次错误创建独立 skill。项目特定内容放 `references/` 或 project capsule。

## Hook 四阶段设计

Mnemon 可把 hook 安装抽象为四阶段，而不要求所有平台事件名一致。

| Mnemon 阶段 | Hermes | Claude Code | OpenClaw |
|---|---|---|---|
| recall | `on_session_start`, `pre_llm_call` | `SessionStart`, `UserPromptSubmit` | bootstrap, message preprocess |
| observe | `pre_tool_call`, `post_tool_call` | `PreToolUse`, `PostToolUse` | command/session/message hooks |
| reflect | `post_llm_call`, `on_session_end` | `Stop`, `SessionEnd` | command reset/new, session hooks |
| curate | gateway ticker, cron, manual | scheduled tasks, manual command | cron/dreaming, compaction hooks |

`INSTALL.md` 可以为每个平台写映射。generic agent 则只需说明：在对应生命周期事件上运行同等功能即可。

## 热冷记忆策略

建议 Mnemon 明确两种接口：

### Model-facing hot memory

模型直接读：

- 当前项目 capsule。
- 用户稳定偏好。
- 当前 guideline。
- 当前任务相关 recall。
- active skill 摘要。

要求：

- 短。
- 可解释。
- 低冲突。
- 可审查。

### Engineering cold memory

工程层保存：

- raw evidence。
- session summaries。
- historical transcripts。
- reports。
- archived skills。
- indexes。
- usage metadata。

要求：

- 大容量。
- 有 provenance。
- 可搜索。
- 可 promotion/demotion。
- 不直接进入 prompt。

这样能避免“md 无限增长”，也避免“复杂数据库直接成为行为层”。

## Curation 策略

第一版 Mnemon curator 建议只做 proposal：

```yaml
run:
  mode: dry-run
  scope:
    - skills
    - memory/hot
    - memory/warm
proposals:
  consolidations: []
  demotions: []
  promotions: []
  archives: []
  patches: []
```

写入规则：

- 默认不 delete，只 archive。
- 高风险文件只 proposal。
- 用户确认后才 patch `GUIDELINE.md` 和 `INSTALL.md`。
- agent-created artifacts 可低风险自动 patch，但仍写 report。
- bundled/package/imported artifacts 默认不自动改。
- pinned artifacts 不 archive。

这基本复制 Hermes curator 的安全姿态，但扩展到 memory hot/warm。

## Dreaming 策略

Dreaming 不应是一开始就必须安装的功能。它适合作为冷记忆规模变大后的进阶模式。

建议三阶段：

1. **Light review**：从最近 session/evidence 中抽候选。
2. **Theme consolidation**：把候选按主题聚合到 warm capsules。
3. **Promotion review**：只有满足复用、确认、相关、近期等条件时才进入 hot memory 或 skill。

OpenClaw 的 insight 是正确的：旧上下文会在 compaction 后丢失细节，所以 pre-compact flush 和 dreaming 能补上长期连续性。但 Mnemon 第一阶段应保持可解释和可审查，不直接自动提升大量记忆。

## 风险与约束

| 风险 | 约束 |
|---|---|
| 自进化污染当前任务 | 当前用户指令优先，recall 只作辅助 |
| hot memory 膨胀 | 固定预算，超出先 curate |
| skill 爆炸 | class-first，curator 合并窄 skill |
| 旧规则变成强指令 | memory 写 declarative facts，不写 imperative commands |
| 后台任务误改 | dry-run-first，report-first，archive 不 delete |
| 跨 agent 安装复杂 | `INSTALL.md` + platform mappings，不写厚 adapter |
| 冷记忆召回噪音 | threshold + `NONE` gate + evidence |
| secret 泄漏 | write scanner + redaction + deny list |
| 无法验证演化效果 | eval cases、测试、LLM judge、human review |

## 推荐实施顺序

1. 写清 `GUIDELINE.md`：memory vs skill、proposal-first、热冷分层。
2. 写清 `INSTALL.md`：四阶段 hook 和平台映射。
3. 定义 3 到 5 个核心 Mnemon skill。
4. 实现 report 格式，不急着自动改文件。
5. 实现 hot memory budget 和 demotion proposal。
6. 实现 skill curator proposal。
7. 再接 cold memory index/search。
8. 最后做 dreaming 和 eval-driven self-evolution。

这个顺序能保持 Hermes 的轻量优势，同时为 OpenClaw 式高容量记忆留下演进路径。

## 最终判断

用户提出的方向是合理的：Mnemon 不应该一开始就构建复杂 adapter 层。更好的设计是让 `INSTALL.md` 和 `GUIDELINE.md` 成为 agent 可读的安装与行为契约，让 skill 成为主要能力表达，让 hooks 成为触发底座，让 filesystem 承载可审查的冷热记忆，再用传统 memory/index 模型解决长期容量。

这不是“只有 Markdown”，而是“Markdown 作为自进化控制面，工程层作为长期记忆底座”。

## 参考来源

- Hermes curator: <https://hermes-agent.nousresearch.com/docs/user-guide/features/curator>
- Hermes hooks: <https://hermes-agent.nousresearch.com/docs/user-guide/features/hooks>
- Hermes Self-Evolution: <https://github.com/NousResearch/hermes-agent-self-evolution>
- OpenClaw Dreaming: <https://docs.openclaw.ai/concepts/dreaming>
- OpenClaw Hooks: <https://docs.openclaw.ai/automation/hooks>
- Claude Code Memory: <https://code.claude.com/docs/en/memory>
- Claude Code Hooks: <https://code.claude.com/docs/en/hooks>

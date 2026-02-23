[< 返回设计概览](../DESIGN.md)

# 7. LLM CLI 集成

![集成架构](../../diagrams/08-three-layer-integration.jpg)

Mnemon 通过生命周期钩子、技能文件和行为引导与 LLM CLI 集成。Claude Code 的[钩子系统](https://docs.anthropic.com/en/docs/claude-code/hooks)是参考实现 — 所有组件通过 `mnemon setup` 自动部署。

## 7.1 集成架构

四个钩子驱动记忆生命周期：

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
  Skill（SKILL.md）── 命令语法参考（自动发现）
    │
    ▼
  LLM 生成回复（遵循 guide.md 行为规则）
    │
    ▼
  Nudge（Stop）─── stop.sh ──→ 提醒 agent 进行 remember
    │
    ▼
  （上下文压缩时）
  Compact（PreCompact）─── compact.sh ──→ 提取关键洞察进行 remember
```

三层协同工作：

| 层 | 内容 | 位置 | 职责 |
|---|------|------|------|
| **钩子** | Claude Code 生命周期事件触发的 Shell 脚本 | `.claude/hooks/mnemon/` | Prime（引导）、Remind（recall 和 remember）、Nudge（remember）、Compact（关键保存） |
| **技能** | `SKILL.md` — Claude Code 技能格式的命令参考 | `.claude/skills/mnemon/` | 教 LLM *怎么*使用 mnemon 命令 |
| **引导** | `guide.md` — recall、remember、委派的详细执行手册 | `~/.mnemon/prompt/` | 教 LLM *何时*召回、*什么*值得记住、*如何*委派 |

## 7.2 钩子详情

Claude Code 在特定生命周期事件触发钩子。Mnemon 注册最多四个，各自承担记忆生命周期中的不同角色：

**Prime（SessionStart）— `prime.sh`**

会话启动时运行一次。加载行为引导 — 详细的 recall、remember、sub-agent 委派执行手册：

```bash
STATS=$(mnemon status 2>/dev/null)
if [ -n "$STATS" ]; then
  # 从 JSON 中提取计数并显示在状态行中
  echo "[mnemon] Memory active (<insights> insights, <edges> edges)."
else
  echo "[mnemon] Memory active."
fi
[ -f ~/.mnemon/prompt/guide.md ] && cat ~/.mnemon/prompt/guide.md
```

引导内容出现在 LLM 的系统上下文中，为整个会话建立 recall/remember/委派行为。

**Remind（UserPromptSubmit）— `user_prompt.sh`**

每条用户消息时运行。轻量级 prompt 提醒，提醒 agent 在工作开始前评估是否需要 recall 和 remember：

```bash
echo "[mnemon] Evaluate: recall needed? After responding, evaluate: remember needed?"
```

agent 根据 guide.md 的规则决定是否响应此提醒 — 这是建议，不是强制执行。

**Nudge（Stop）— `stop.sh`**

每次 LLM 回复后运行。提醒 agent 考虑是否需要 remember。如果已处理过记忆操作则保持静默：

```bash
MSG=$(echo "$INPUT" | jq -r '.last_assistant_message // ""' 2>/dev/null)
if echo "$MSG" | grep -qi "mnemon remember\|sub-agent.*remember\|Stored.*imp="; then
  exit 0  # 已处理
fi
echo "[mnemon] Consider: does this exchange warrant a remember sub-agent?"
```

**Compact（PreCompact）— `compact.sh`（可选）**

上下文窗口压缩前触发。指示 agent 提取最关键的洞察并 remember，防止上下文丢失：

```bash
echo "[mnemon] Context compaction starting. Review this session and remember the most valuable insights (up to 5) before context is compressed. Delegate to Task sub-agents now."
```

## 7.3 自动化 Setup

`mnemon setup` 自动处理所有部署：

```
$ mnemon setup

Detecting LLM CLI environments...
  ✓ Claude Code (v1.x)    .claude/

Select environment: Claude Code
Install scope: Local — this project only (.claude/)

[1/3] Skill
  ✓ Skill     .claude/skills/mnemon/SKILL.md

[2/3] Prompts
  ✓ Prompts   ~/.mnemon/prompt/ (guide.md, skill.md)

[3/3] Optional hooks
  Select hooks to enable:
    [x] Remind  — 提醒 agent 进行 recall 和 remember（推荐）
    [x] Nudge   — 工作结束后提醒 agent 进行 remember
    [ ] Compact — 压缩前提取关键洞察

Setup complete!
  Hooks   prime, remind, nudge
  Prompts ~/.mnemon/prompt/ (guide.md, skill.md)

Start a new Claude Code session to activate.
Edit ~/.mnemon/prompt/guide.md to customize behavior.
Run 'mnemon setup --eject' to remove.
```

关键 setup 选项：

| 标志 | 效果 |
|------|------|
| `--global` | 安装到 `~/.claude/`（所有项目）而非 `.claude/`（项目级） |
| `--target claude-code` | 非交互式，仅 Claude Code |
| `--eject` | 移除所有 mnemon 集成 |
| `--yes` | 自动确认所有提示（CI 友好） |

Prime 钩子始终安装。Remind、Nudge、Compact 钩子可选（Remind 和 Nudge 默认启用）。

## 7.4 Sub-Agent 委派

记忆写入不在主对话中进行。宿主 LLM 将其委派给轻量 sub-agent：

```
主 Agent（Opus）                       Sub-Agent（Sonnet）
┌──────────────────────┐              ┌──────────────────────┐
│ 完整对话上下文          │  委派        │ ~1000 tokens 上下文    │
│（~25k tokens）         │ ──────────→ │ 读取 SKILL.md         │
│                       │              │ 执行命令              │
│ 决定记什么              │  结果        │ 基于判断评估候选        │
│                       │ ←────────── │                      │
└──────────────────────┘              └──────────────────────┘
```

**为什么用 Sub-Agent？**

| 维度 | 主对话 | Sub-Agent |
|------|-------|-----------|
| 上下文大小 | ~25,000 tokens | ~1,000 tokens |
| 模型 | Opus（昂贵） | Sonnet（更便宜） |
| 范围 | 完整对话 | 仅记忆任务 |
| 执行 | 同步，阻塞用户 | 后台，非阻塞 |

主 agent 只提供记什么——内容、分类、重要性、实体。Sub-agent 读取 SKILL.md，执行正确的 `mnemon remember` 命令，并基于判断而非机械规则评估 `remember` 返回的 Link 候选。

这种分离意味着：

- **Token 经济性**：每次记忆写入约 ~7,000 tokens，而非主对话中的 ~25,000
- **上下文隔离**：记忆处理不会污染主对话上下文
- **模型效率**：Sonnet 处理常规执行，Opus 专注高层决策

## 7.5 适配其他 LLM CLI

对于支持钩子的 CLI，复制 Claude Code 模式：注册调用 mnemon 命令的生命周期钩子，部署技能文件，提供行为引导。

对于不支持钩子的 CLI，将 recall/remember 引导合并到对应的系统提示文件中：

- Cursor → `.cursorrules`
- Windsurf → `RULES.md`
- OpenClaw → `mnemon setup --target openclaw` 部署技能 + 引导，但钩子需手动配置插件
- 其他 → 系统提示 / 规则文件

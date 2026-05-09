# Mnemon Harness 安装指南

> 从 [HARNESS.md](HARNESS.md) 派生的可安装资产。把本文交给目标 agent，要求它把 Mnemon 安装到自己的 runtime 中。

## 目标

以轻量 memory harness 的方式安装 Mnemon：

```text
SKILL.md 教命令。
GUIDELINE.md 教判断。
Hook 在生命周期边界提醒。
mnemon 执行确定性记忆操作。
```

除非 runtime 确实需要自动化，否则不要先构建 custom adapter。一个 capable agent 应能把这些说明映射到自己的原生机制。

## 前置条件

确认 `mnemon` binary 可用：

```bash
mnemon --version
```

如果缺失，使用项目支持的安装方式，例如：

```bash
brew install mnemon-dev/tap/mnemon
```

或：

```bash
go install github.com/mnemon-dev/mnemon@latest
```

## 安装步骤

1. 将 `SKILL.md` 安装到 runtime 的 skill、rule、command 或 instruction 机制。
2. 将 `GUIDELINE.md` 安装到 runtime 在 session 开始和记忆敏感决策前能读取的位置。
3. 默认配置 project-scoped Mnemon store，除非用户明确要求 global store。
4. 当 runtime 支持 hooks 时，添加四个 hook phase。
5. 如果 hooks 不可用，用持久规则编码同样的 phase 检查。
6. 执行下面的验证 checklist。

## Hook Phase

每个 hook 可以只输出一条短的自然语言提醒。Hook 脚本不应强制执行记忆操作。

| Phase | Runtime 时机 | 必须提醒 |
|---|---|---|
| Prime | Session start / bootstrap | 加载 Mnemon skill、guideline 和当前 store 信息 |
| Remind | User prompt submit / before planning | 判断 recall 是否可能改变当前任务 |
| Nudge | Stop / after response | 判断 durable writeback 是否有正当性 |
| Compact | Before context compaction | 只保存关键连续性 |

如果 runtime 只支持部分 hook 时机，就安装可用部分，并把缺失检查保留在持久指令中。

## Runtime 映射示例

使用最接近的原生等价机制：

| Runtime | 安装目标 |
|---|---|
| Codex | `AGENTS.md`、skill、本地指令，以及启用后的 hooks |
| Claude Code | `CLAUDE.md`、skill、slash command、settings hooks、project/user memory |
| OpenClaw | Plugin hooks 和 skill |
| Skill-first agents | Skill、memory guidance 和轻量提醒 |
| Minimal CLI | 引用 skill 和 guideline 的 rule 文件或 system instruction |

这些映射只是例子。即使路径或文件名不同，也要保留行为契约。

## 验证

当 agent 能做到以下事情时，安装可接受：

1. 解释 Mnemon recall 何时有用、何时应跳过。
2. 对相关任务运行 `mnemon recall "<focused query>" --limit 5`。
3. 写入一条带 provenance 的 durable memory。
4. 对 trivial task 跳过 memory。
5. 如果 runtime 暴露压缩事件，则在压缩前只保存关键连续性。

如果 memory 被用于每个 prompt、普通聊天被保存为 memory，或者陈旧 memory 覆盖当前用户指令和仓库事实，则安装不可接受。

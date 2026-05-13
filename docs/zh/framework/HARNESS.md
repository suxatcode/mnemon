# Mnemon Memory Harness

> 草案。本文是 Mnemon memory harness 设计的中文单一入口。它同时面向人类和 agent：一个具备文件读写与命令执行能力的 agent 应该可以阅读本文，并把 Mnemon 安装进自己的运行时环境。

## 目标

Mnemon 不是 agent runtime。它是围绕 agent runtime 的外部记忆 harness。

宿主 runtime 仍然负责与用户交互、规划任务、编辑文件、运行命令和做语义判断。Mnemon 负责提供持久记忆、稳定记忆协议，以及在关键生命周期阶段提醒 runtime 使用跨会话记忆。

```text
Runtime 负责做事。
Mnemon 负责保存经验、召回经验，并约束记忆协议。
```

这个 harness 应保持简单：

- **Skill first**：agent 通过 Markdown 指令和命令示例学习 Mnemon。
- **Guideline driven**：agent 获得一份记忆策略，用来判断何时 recall、remember、link、forget，或者什么都不做。
- **Hook assisted**：四个生命周期提醒在关键时刻重新激活 guideline。
- **Protocol constrained**：agent 做语义判断；Mnemon 提供确定性命令、结构化输出、provenance、去重和生命周期操作。
- **Markdown evolved**：稳定经验可以沉淀成经过 review 的 Markdown 资产：skill、guideline、install note、rule、contract 或 eval case。

## 非目标

Mnemon 不应成为：

- 完整 agent runtime
- 工作流引擎
- 大型 adapter framework
- 自动 prompt 注入系统
- 只追加不治理的记忆仓库
- 向量数据库 wrapper
- 无审查的自修改 agent

不同 runtime 不需要先拥有专门的 Mnemon adapter 才能使用这个 harness。只要一个 runtime 能读取指令、运行命令，并且可以选择性挂接 hook 或规则，它就可以按照本文安装 Mnemon。

## Harness 形态

Harness 由四类概念资产组成。

| 资产 | 作用 |
|---|---|
| **Mnemon binary** | 通过 `remember`、`recall`、`link` 和生命周期命令执行确定性记忆操作 |
| **Skill** | 教 agent 有哪些命令，以及如何调用 |
| **Guideline** | 教 agent 什么时候记忆有用、什么值得写入，以及如何避免噪音 |
| **Hooks** | 在 session 开始、任务开始、任务结束和上下文压缩前提醒 agent 应用 guideline |

这些资产可以安装为 skill 文件、规则文件、系统指令、插件文档、hook 脚本，或者任何 runtime 支持的等价形式。具体安装格式不重要，重要的是保留行为语义。

## Markdown 契约

持久 harness 层应主要由 Markdown 表达。runtime-specific adapter 是可选便利，不是核心设计。

标准安装包应能表达为三份可读文件：

| 文件 | 主要读者 | 职责 |
|---|---|---|
| `SKILL.md` | Agent | 命令语法、示例、可用操作、输出解释和硬性 guardrail |
| [`INSTALL.md`](INSTALL.md) | Agent 或人类安装者 | 如何在目标 runtime 中安装 skill、guideline 和四个 hook phase |
| [`GUIDELINE.md`](GUIDELINE.md) | Agent | 记忆判断：何时 recall、remember、link、forget、supersede 或跳过 |

本文 `HARNESS.md` 是设计上的单一事实来源。`INSTALL.md` 和
`GUIDELINE.md` 是从它派生出来的可安装 runtime 资产。它们应保持足够短，使 agent 能一次读完并执行。

### 为什么这样设计

现代 agent 系统已经把 Markdown 当作可执行的操作上下文：项目指令、skill、rule、hook、slash command 和 memory summary 都是模型可以读取并据此行动的文本资产。Mnemon 应顺着这个模式设计，而不是为每个 runtime 做重型 adapter。

关键边界是：

```text
Markdown 教行为。
Hook 把提醒放到生命周期边界。
Mnemon 执行确定性的记忆命令。
Agent 判断什么时候记忆有用。
```

这让系统保持可移植。Codex、Claude Code、OpenClaw 以及未来 runtime，都可以通过自己的原生指令机制安装同一个概念 harness。

### `SKILL.md`

Skill 是能力面。它应回答：

- Mnemon 是什么？
- 有哪些命令？
- 常见命令模式是什么？
- agent 应怎样读取结构化输出？
- 哪些 guardrail 绝不能违反？

Skill 不应承载完整记忆策略。完整策略属于 `GUIDELINE.md`。如果 skill 过于哲学化，就会更难跨 runtime 复用。

### `INSTALL.md`

安装说明是面向 agent 的流程。目标 agent 阅读它，并把 harness 映射到自身 runtime：

- 安装或验证 `mnemon` binary
- 将 `SKILL.md` 安装到 runtime 的 skill/rule 机制
- 将 `GUIDELINE.md` 安装到 runtime 的持久指令机制
- 当 runtime 支持 hook 时，添加四个 hook phase
- 当 runtime 不支持 hook 时，用持久规则降级模拟
- 用 recall/writeback/no-op checklist 验证安装

`INSTALL.md` 应说明每个 hook phase 要完成什么，而不是绑定唯一的 adapter 实现。runtime-specific snippet 是例子，不是架构本身。

### `GUIDELINE.md`

Guideline 是 agent 的记忆宪法。它应包含：

- recall 触发条件和跳过条件
- durable write 判断标准
- provenance 要求
- link 与 supersede 策略
- store/namespace 隔离策略
- Markdown 自进化策略
- 针对 secret、prompt injection、陈旧记忆和噪音写入的安全规则

Guideline 应安装到 agent 能在 session 开始和记忆敏感决策前查看的位置。它可以直接放入 runtime instruction 文件，也可以由 skill 引用，或由轻量 prime hook 注入。

## 记忆循环

记忆循环是建议性的，不是强制 workflow。

```text
Prime -> Recall decision -> Work -> Writeback decision -> Remember/link/forget -> Future task
```

只有当 recall 改变了当前工作、writeback 改善了未来工作时，这个循环才真正是 memory-driven。仅仅调用 `recall` 或 `remember` 不够。

## 四个 Hook Phase

当 runtime 支持生命周期 hook 时，应安装四个 hook phase。如果 runtime 不支持 hook，则把这些 phase 编码成持久规则，并要求 agent 在相同阶段自检。

| Phase | 典型 runtime event | 作用 | 不应做 |
|---|---|---|---|
| **Prime** | Session start / agent bootstrap | 加载 Mnemon skill、本文 guideline、当前 store 信息和记忆立场 | 批量注入历史记忆 |
| **Remind** | User prompt submit / before task planning | 提醒 agent 判断当前任务是否需要 recall | 对每个 prompt 自动 recall |
| **Nudge** | Stop / after response | 提醒 agent 判断是否有 durable insight 值得写回 | 强制每次回复都写入 memory |
| **Compact** | Before context compaction | 在上下文丢失前保留关键连续性 | 机械保存完整对话 |

Hook 输出应短、自然、可解释，并且在记忆无关时可以被 agent 忽略。Hook 是认知提醒，不是控制器。

### Prime

Prime 建立记忆方位。

它应告诉 agent：

- Mnemon 可用。
- agent 应使用 Mnemon skill 查看命令语法。
- 本 harness guideline 定义何时使用记忆。
- 必须尊重当前 store 或 namespace。
- 历史记忆只应在与当前任务相关时召回。

### Remind

Remind 发生在 agent 开始任务之前。

它应要求 agent 在任务可能依赖以下内容时考虑 recall：

- 先前用户偏好
- 先前项目决策
- 架构约定
- 重复失败或修复经验
- 部署或环境事实
- 之前未完成的工作

对于简单、本地、上下文已经充分的任务，agent 可以跳过 recall。

### Nudge

Nudge 发生在 agent 完成任务之后。

它应要求 agent 判断本次 session 是否产生了未来值得复用的 durable knowledge。只有当 insight 未来可能再次有用时，agent 才应写入 memory。

### Compact

Compact 发生在上下文压缩之前。

它只应保留关键连续性：

- 尚未关闭的决策
- 影响工作的用户偏好
- 未解决的 blocker
- 重要实现事实
- 未来 agent 必须重复或避免的命令和 workflow

## 记忆 Guideline

Guideline 是每个 agent 都应遵守的记忆行为策略。

### Recall

当过往经验可能改变当前任务时，执行 recall。

适合 recall 的触发条件：

- 用户提到之前的工作、先前决策或既有偏好。
- 任务涉及架构、发布、部署、集成或长期项目约定。
- agent 正在长时间间隔或上下文压缩后恢复任务。
- 任务可能重复已知失败模式。
- 用户要求与先前风格、策略或 policy 保持一致。

较弱的 recall 触发条件：

- 简单的一次性命令。
- 当前上下文已经清楚的纯局部代码修改。
- 可完全由当前 prompt 或可见仓库回答的问题。

Recall 结果是证据，不是权威。当前用户指令、当前仓库状态和已验证来源优先于陈旧记忆。

### Remember

只记 durable insight。

适合写入 memory 的内容：

- 稳定用户偏好
- 项目约定
- 架构或产品决策
- 重复失败模式和修复方式
- 非显而易见的 setup 或部署事实
- 未来 agent 应遵守的约束
- supersede 旧决策的新决策

不适合写入 memory 的内容：

- secret、credential、token 或私密数据
- 临时进度流水账
- 原始对话日志
- 未验证假设
- 源码中已经显而易见的事实
- 未来大概率不会再用到的噪音实现细节

每条 durable write 都应包含足够 provenance，让未来 agent 能判断这条记忆是否仍然适用。

推荐 provenance：

- `source`：user、agent、system、repo、docs、command output
- `source_ref`：文件路径、命令、issue、PR、conversation 或 hook phase
- `reason`：为什么值得记住
- `confidence`：这个 insight 的可靠程度
- `evidence`：可用时给出具体证据
- `scope`：project、user、runtime 或 global

### Link

当关系对未来 recall 有用时，建立 link。

有用的 link：

- 一个决策 supersede 另一个决策
- 一个失败由特定 setup 或依赖导致
- 一个偏好适用于某个项目或 runtime
- 一个 workflow 依赖某个工具、文件或环境
- 两条记忆未来应一起被召回

不要仅仅因为两条记忆语义上有点相似就创建 link。

### Forget 与 Supersede

Memory 必须演化。

当一条 memory 过期时，优先 supersede 或软删除，而不是继续追加冲突记忆。未来 agent 应能判断哪个决策是当前有效的。

以下场景应使用生命周期操作：

- 已存决策现在是错的
- 用户偏好发生变化
- 实现细节不再符合当前仓库
- 某条 memory 噪音太大或范围太宽
- 更强 memory 替代了较弱 memory

### Scope 与隔离

默认使用 project-scoped memory。只有稳定用户偏好或明确安全的跨项目实践才应进入 global memory。

不要让一个项目的架构假设静默影响另一个项目。如果 runtime 支持 namespace 或 store，安装 Mnemon 时应明确 store strategy。

## 安装

安装是一个 agent task。把本文交给目标 agent，要求它用最接近自身 runtime 的机制，把 Mnemon 安装进自己的环境。

推荐的用户流程是：

```text
1. 把 INSTALL.md 交给目标 agent。
2. INSTALL.md 告诉 agent SKILL.md 和 GUIDELINE.md 在哪里。
3. agent 将这些文件安装到自身原生指令系统。
4. 如果 runtime 支持 hook，agent 添加四个 hook phase。
5. agent 用小型 recall/writeback/no-op 检查验证行为。
```

这意味着，一个 runtime 不需要先拥有专用 adapter 才能使用 Mnemon。
Adapter 或 `mnemon setup --target <runtime>` 命令可以在之后自动化同样步骤，但架构本身应保持仅靠 Markdown 就可理解、可安装。

### 前置条件

目标机器应能访问 `mnemon` binary：

```bash
mnemon --version
```

如果缺失，使用项目支持的安装方式之一：

```bash
brew install mnemon-dev/tap/mnemon
```

或：

```bash
go install github.com/mnemon-dev/mnemon@latest
```

### 安装 Skill

安装一个 skill、rule 或 instruction 文件，教会 agent：

- Mnemon 是外部记忆工具。
- 核心协议是 `remember`、`recall`、`link` 和生命周期命令。
- agent 应读取结构化命令输出，而不是猜测结果。
- agent 应遵守本文 harness guideline 做记忆决策。

Skill 应专注于命令语法和能力说明。本文中的 guideline 负责判断策略。

### 安装 Guideline

将本文，或其中的“记忆 Guideline”部分，安装到 runtime 的持久指令机制中。

有效形式包括：

- skill 引用
- rules 文件
- project instruction 文件
- plugin guide
- system prompt section
- runtime 启动时会读取的仓库文档

Guideline 应足够可见，使 agent 不需要用户每个 session 重复记忆规则也能应用它。

### 安装 Hooks

如果 runtime 支持 hook，安装四个轻量 hook：

| Hook | 必须行为 |
|---|---|
| Prime | 告诉 agent 加载 Mnemon skill/guideline，并尊重当前 store |
| Remind | 任务开始前询问 recall 是否有用 |
| Nudge | 任务结束后询问 writeback 是否有用 |
| Compact | 压缩前只保存关键连续性 |

Hook 脚本可以只打印自然语言提醒。它们不需要自己执行重型 memory 操作。

不同 runtime 的 hook 脚本也不需要完全相同。真正需要保持的是 phase 行为契约，而不是脚本正文。例如：

- Codex 可以使用 hooks 加 `AGENTS.md`、skill 或本地指令。
- Claude Code 可以使用 `CLAUDE.md`、skill、slash command、settings hooks 或 project/user memory 文件。
- OpenClaw 可以使用 plugin hooks 和 skill，但 Mnemon 不应要求一个 OpenClaw-specific memory engine。
- Skill-first runtime 可以把绝大多数行为直接表达为 skill、memory guidance 和轻量提醒。

如果 runtime 没有 hook，用 rules 或持久指令模拟同样检查：

```text
任务开始时，判断 Mnemon recall 是否有用。
任务结束时，判断 durable memory writeback 是否有用。
上下文压缩前，保存关键连续性。
```

### 验证安装

当 agent 能做到以下行为时，安装可接受：

1. 解释何时应 recall、何时应跳过 recall。
2. 针对相关任务运行 `mnemon recall`。
3. 写入带 provenance 的 durable memory。
4. 面对 trivial task 时避免写入 memory。
5. 如果 runtime 暴露压缩事件，则能在压缩前保存关键状态。

## 评估

Harness 工作正常的表现：

- recall 改善任务连续性或决策质量
- writeback 产生未来价值
- memory 体量受到控制
- stale memory 可以被 supersede
- project store 不互相污染
- agent 能解释为什么 recall 或 remember

Harness 失败的表现：

- hook 强制每个任务都使用 memory
- agent 把普通聊天保存成 memory
- 旧 memory 覆盖当前仓库事实
- memory 增长速度高于 recall 质量增长
- global memory 泄漏项目特定假设

## 轻量自进化

自进化应先从轻量 Markdown loop 开始，而不是先做重型 framework。

正式 modular self-evolution harness 文档见 [Mnemon Harness](../../harness/README.md)。历史 v0.2 架构保留在 [Self-Evolution Harness Archive](../../design/self-evolution-harness/SELF_EVOLUTION_HARNESS.md)。

Mnemon 不应自动改写 runtime 行为。它应帮助 agent 发现重复经验、保存证据，并提出 Markdown 变更候选；这些候选必须由人类或仓库 review 接受后才生效。

```text
experience
  -> Mnemon memory
  -> LLM reflection
  -> markdown candidate
  -> diff / PR / human review
  -> installed skill, guideline, rule, contract, or eval
```

这条路径现实可行，因为 LLM agent 已经很擅长读取 Markdown 指令。Skill、rule、install guide 和 harness guideline 都容易编写、检查、diff、review 和回滚。

### 演化什么

第一阶段应优先演化文本资产：

| Asset | 何时演化 | 示例 |
|---|---|---|
| **Skill** | 某个流程在多个任务中反复有效 | 发布 workflow、迁移 workflow、review workflow |
| **Guideline** | 记忆策略需要更精确的判断 | “除非用户说明稳定，否则不要记一次性部署 IP” |
| **Install Note** | 某个 runtime 集成方式已经可靠 | 如何在某个 CLI 中安装四个 hook phase |
| **Rule / Contract** | 稳定项目约束必须始终遵守 | “不要提交 `.env`；只更新 `.env.example`” |
| **Eval Case** | 重复失败应变成可测试样例 | 一个验证 recall 是否阻止同类错误的复现任务 |

不要一开始就演化代码、数据库 schema 或 runtime 内核。等 Markdown loop 被证明有用后，再考虑更重的工程实现。

### Promotion 触发条件

Agent 可以在以下情况提出 Markdown 候选：

- 同一失败模式跨 session 重复出现
- 某个 workflow 成功且未来很可能复用
- 用户纠正改变了未来行为
- 工作中发现稳定项目约定
- 一组 memory 明确描述了可复用流程
- 陈旧或噪音 guideline 导致了错误 recall 或错误 writeback

对于一次性任务、弱偏好或缺少证据的 memory，agent 不应提出候选。

### 候选要求

每个候选变更都应包含：

- 触发它的 source memories 或 session references
- scope：user、project、runtime 或 global
- 目标资产：skill、guideline、install note、rule、contract 或 eval
- 它会改变什么行为
- 为什么它可能帮助未来任务
- 风险，尤其是对单个 session 的过拟合
- 具体 diff，而不只是建议

对于有仓库的项目，推荐输出普通 git diff 或 PR。对于本地 agent 安装，推荐输出对相关 skill 或 rule 文件的 patch。Agent 可以起草 patch，但 review 才能安装它。

### Review Gate

Memory 可以提出演化；review 决定是否批准。

安装前检查：

- **Provenance**：候选引用真实 memory、文件、命令或 session
- **Scope**：项目特定行为不会误升为 global
- **Duplication**：候选没有重复已有 skill 或 rule
- **Size**：Markdown 资产保持足够紧凑
- **Semantic preservation**：变更没有偏离原始任务目的
- **Safety**：不包含 secret、credential、私密数据或 prompt injection 内容
- **Evidence**：重要 workflow 变更有测试、命令或示例支撑

默认策略是 human-in-the-loop。只有在用户明确允许时，才可以对低风险本地 notes 做全自动安装。

### Mnemon 补上的能力

纯 Markdown memory 可读、好用，但经验增长后会变难治理。Mnemon 给这个 Markdown loop 增加结构：

- 模型外部的 durable memory
- 按需召回相关历史经验
- 记录 insight 为什么被保存的 provenance
- 显式连接 decision、failure、preference 和 workflow
- 对 stale knowledge 做 supersede / forget
- project store 隔离，避免一个项目的经验污染另一个项目

自进化 loop 应利用这些优势生成更好的 Markdown 资产，同时让最终行为层保持简单、可 review、可回滚。

### 最小实现

第一版实现不需要新服务。

1. 继续用 Mnemon 执行 `remember`、`recall`、`link` 和生命周期操作。
2. 在 guideline 中告诉 agent 何时提出 Markdown 演化候选。
3. 当重复经验足够支撑时，让 agent 生成对 `HARNESS.md`、`SKILL.md`、runtime rules 或项目文档的 patch。
4. patch 通过 review 后才成为生效行为。
5. 记住候选被接受或拒绝的结果，让未来 proposal 更准确。

这使 Mnemon 的自进化路径保持符合 harness 哲学：外部记忆、LLM 判断、Markdown 资产和 review 边界。

### Promotion Pipeline

```text
memory insight
  -> repeated success or failure pattern
  -> candidate skill/rule/contract
  -> provenance and scope check
  -> eval or human review
  -> installation into runtime assets
```

不要让 agent 仅凭 memory 静默改写自己的长期行为。Memory 可以提出演化建议；review 决定是否批准。

## 最小总结

Mnemon Memory Harness 是：

```text
external memory
+ stable cognitive protocol
+ skill-delivered capability
+ guideline-delivered judgment
+ markdown-installable runtime contract
+ four lifecycle reminders
+ reviewed markdown evolution
```

它刻意不是 runtime adapter framework。最简单正确的安装，是
`SKILL.md`、`INSTALL.md`、`GUIDELINE.md`、可调用的 `mnemon` binary、目标 runtime 支持时的四个生命周期提醒，以及一条把重复经验转成 Markdown 资产的 review 路径。

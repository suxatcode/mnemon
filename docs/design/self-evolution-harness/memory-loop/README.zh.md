# Memory Loop MVP 设计

本文档描述 memory loop 的第一版实现切片。目标是让 harness 保持足够小：围绕已有 HostAgent 安装少量 hook prompt 和 Markdown 能力，同时使用 Mnemon 作为长期记忆后端。

交互式展示：[site/index.html](./site/index.html)

英文版本：[README.md](./README.md)

参考实现：[harness/memory-loop](../../../../harness/memory-loop)

## 核心模型

MVP 只有三个核心主体：

| 主体 | 作用 | 边界 |
| --- | --- | --- |
| HostAgent | 宿主 agent runtime。它运行任务、接收 hook 注入，并决定是否加载记忆 skill 或启动 dreaming subagent。 | 不拥有记忆存储协议。 |
| `MEMORY.md` | 工作记忆文件。它很小、直接面向 prompt，并在 Prime 阶段进入 system prompt。 | 由 `memory_set.md` 和 dreaming subagent 维护。 |
| Mnemon | 长期记忆存储和 binary。它独立安装，例如通过 `brew install`。 | 通过 `memory_get.md` 和 dreaming subagent 协议访问。 |

其他内容都是围绕这三个主体的支撑资产。

## 维护资产

第一版维护以下资产：

| 资产 | 类型 | 用途 |
| --- | --- | --- |
| `env.sh` | 配置 | 定义 `MNEMON_MEMORY_LOOP_ENV`、`MNEMON_MEMORY_LOOP_DIR` 和记忆长度阈值。 |
| `GUIDE.md` | 手册 | 描述何时读记忆、何时写记忆、什么信息值得保留。 |
| Claude Code setup scripts | 安装 | 第一条具体安装路径。安装 hooks、skills、subagent 和 memory 文件。 |
| Prime hook | Hook | 将 `MEMORY.md` 和 `GUIDE.md` 注入 system prompt。 |
| Remind hook | Hook | 提醒 HostAgent 判断是否需要读取记忆。 |
| Nudge hook | Hook | 提醒 HostAgent 判断是否需要积累工作记忆。 |
| Compact hook | Hook | 在上下文压缩前提醒 HostAgent 保存重要信息。 |
| `memory_get.md` | Skill | 定义如何从 Mnemon 召回长期记忆。 |
| `memory_set.md` | Skill | 定义如何编辑 `MEMORY.md`。 |
| dreaming subagent spec | Subagent | 定义如何将 `MEMORY.md` 巩固到 Mnemon，并压缩或移除工作记忆条目。 |

## 策略与实现分离

`GUIDE.md` 刻意保持抽象。它描述记忆行为，而不是存储机制。

它回答这类问题：

- 当前是否应该读记忆？
- 当前是否应该写记忆？
- 这条信息是否足够稳定，值得保留？
- 这是长期偏好、项目约定，还是可复用事实？

它不要求 HostAgent 判断目标是 `MEMORY.md` 还是 Mnemon。这个决定下沉到能力层。可复用能力通过 `MNEMON_MEMORY_LOOP_DIR` 定位运行目录。

- `memory_get.md` 将“读记忆”映射为 Mnemon recall。
- `memory_set.md` 将“写记忆”映射为 `$MNEMON_MEMORY_LOOP_DIR/MEMORY.md` 编辑。
- dreaming subagent 将“巩固”映射为 Mnemon write 加 `$MNEMON_MEMORY_LOOP_DIR/MEMORY.md` 压缩。

这个拆分让 guide 可以跨不同 HostAgent 复用。

## 运行流程

### Prime

Prime 是唯一的直接加载路径。

输入：

- `MEMORY.md`
- `GUIDE.md`

动作：

- 将二者注入 HostAgent 的 system prompt。

边界：

- Prime 不调用 `memory_get.md`。
- Prime 不召回 Mnemon。
- Prime 不写长期记忆。

### Remind / Recall

Remind 创造读记忆机会。

流程：

1. Remind 根据 `GUIDE.md` 提醒 HostAgent 判断是否应该读记忆。
2. 如果需要，HostAgent 加载 `memory_get.md`。
3. `memory_get.md` 说明如何调用 Mnemon recall。
4. Mnemon 返回有界的 recall context 给 HostAgent。

边界：

- 长期记忆不会被全量注入。
- recall 结果不会自动写回 `MEMORY.md`。
- `GUIDE.md` 不需要知道 Mnemon 协议细节。

### Nudge / Accumulate

Nudge 创造写工作记忆机会。

流程：

1. Nudge 根据 `GUIDE.md` 提醒 HostAgent 判断是否应该积累记忆。
2. 如果需要，HostAgent 加载 `memory_set.md`。
3. `memory_set.md` 说明如何新增、替换或删除 `MEMORY.md` 条目。

边界：

- 在线记忆积累只写 `MEMORY.md`。
- 它不直接写 Mnemon。
- 它应避免记录流水账、一次性进度和低置信度观察。

### Compact

Compact 是上下文边界时的 Nudge。

流程：

1. 在上下文压缩前，Compact 提醒 HostAgent 判断是否有重要信息会丢失。
2. 如果需要，HostAgent 加载 `memory_set.md`。
3. `memory_set.md` 将必要的最后补丁写入 `MEMORY.md`。

边界：

- Compact 不是 dreaming。
- Compact 不做全量工作记忆清理。
- Compact 不直接写长期记忆。

### Dreaming

Dreaming 是维护流程，不是普通在线 hook。

流程：

1. HostAgent 启动专用 dreaming subagent。
2. subagent 读取完整 `MEMORY.md`。
3. subagent 按 Mnemon 协议将当前工作记忆写入 Mnemon。
4. subagent 压缩、整理或移除 `MEMORY.md` 条目。

触发时机：

- `MEMORY.md` 超过 quota。
- 上下文压缩前。
- 用户或 HostAgent 主动要求。

边界：

- Dreaming 负责巩固与清理。
- 它不替代 Remind、Nudge 或 Compact。
- 它需要保留 prompt-facing 有用性，同时把耐久信息移动到长期记忆。

## 第一版范围

MVP 包含：

- 最小 `GUIDE.md`。
- Claude Code setup scripts，将 Prime、Remind、Nudge、Compact 挂载进 `.claude/settings.json`。
- `MEMORY.md` 模板。
- 用于 Mnemon recall 的 `memory_get.md` skill。
- 用于 `MEMORY.md` 编辑的 `memory_set.md` skill。
- dreaming subagent spec。
- 明确假设 Mnemon 作为 binary 和长期存储独立安装。

MVP 不包含：

- 自定义 agent runtime。
- 复杂 adapter framework。
- 第二种 working-memory 格式。
- 普通在线 hook 直接写长期记忆的路径。

## 设计原则

Harness 应保持 agent-agnostic。它向 HostAgent 提供安装记忆行为所需的材料：

- 规则手册和安装脚本；
- 用于时机控制的 hooks；
- 用于在线记忆操作的 skills；
- 用于离线巩固的 subagent；
- 用于长期存储的 Mnemon。

这让第一版足够可实现，同时保留目标记忆循环：`MEMORY.md` 提供 prompt-facing working memory，Mnemon 提供 durable long-term memory，dreaming 在二者之间移动信息。

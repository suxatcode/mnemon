# Letta 的记忆、Markdown 与 Prompt 用法

## 记忆处理方案

Letta 的 prompt 告诉 agent：

- recall memory 是过去交互数据库；
- 可用 `conversation_search` 搜索；
- core memory 在 context 中，可编辑；
- archival memory 在 context 外，需要显式 search；
- 新的重要信息应立即写入 core 或 archival memory。

这是一种 self-editing memory agent：模型不仅读 memory，还负责选择工具修改 memory。

## Markdown 用法

Letta 的 Markdown 主要出现在：

- docs；
- memory repo 的 block markdown/git 表示；
- examples；
- prompt/content formatting。

它不是 Claude/Codex/Hermes 那种以 `SKILL.md`、`AGENTS.md`、`CLAUDE.md` 为主的行为安装层。Letta 的行为更多由 code、schema、server API、tool descriptions 和 system prompts 控制。

## 特殊 prompt

`memgpt_chat.py` 的关键 prompt 模式：

- 把 memory hierarchy 直接解释给 agent；
- 明确 core memory 的编辑工具；
- 明确 archival memory 必须 search；
- 告诉 agent 它会看到 archival memory statistics；
- 要求遇到重要新信息时更新 memory。

`prompt_generator.py` 则动态加入 metadata：

- previous message count；
- archival memory size；
- archival tags。

这是一种「meta-information first」设计：先告诉 agent 有多少外部 memory，再让它决定是否 search。

## 智能体演化方案

Letta 的演化主要是：

- core memory blocks 被 agent 修改；
- archival memory 被 agent 扩展；
- recall memory 随 conversation history 增长；
- server/API 层支持 attach/detach/update memory blocks；
- sleeptime/voice agent 等变体可在后台或专用 agent 中处理 memory。

它不是「skills 自我演化」路线，而是「agent state 自我编辑」路线。

## 对 Mnemon 的设计判断

Letta 适合提醒 Mnemon：

- memory tool 必须能精确 append/replace；
- external memory 应按需 retrieval；
- in-context memory 应严格预算；
- memory metadata 有助于 agent 判断是否 search。

但 Mnemon 当前应避免：

- 深度耦合 agent state；
- 直接复制 core/archival schema；
- 把自进化限定为 memory block 编辑。

Mnemon 更适合把 Letta 的 hierarchy 思想翻译成轻量版：

```text
GUIDELINE.md = stable behavior policy
SKILL.md = command/procedure capability
Mnemon store = external durable memory
reviewed markdown patch = behavior evolution
```

## 参考来源

- 本地源码: `letta/prompts/system_prompts/memgpt_chat.py`
- 本地源码: `letta/prompts/prompt_generator.py`
- 本地源码: `letta/functions/function_sets/base.py`
- 本地源码: `letta/server/rest_api/proxy_helpers.py`
- 本地源码: `letta/services/memory_repo/`
- 官方文档: [Letta stateful agents](https://docs.letta.com/guides/core-concepts/stateful-agents)
- 官方文档: [Letta memory blocks](https://docs.letta.com/guides/core-concepts/memory/memory-blocks)
- 官方文档: [Letta archival memory](https://docs.letta.com/guides/core-concepts/memory/archival-memory)
- 论文: [MemGPT: Towards LLMs as Operating Systems](https://arxiv.org/abs/2310.08560)

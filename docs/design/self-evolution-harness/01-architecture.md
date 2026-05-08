# 01. 总体架构

## 核心边界

Self-Evolution Harness 不实现 agent。它安装到 host agent 上，复用 host agent 的 runtime。

| 责任 | Host agent | Harness |
|---|---|---|
| LLM 调用 | 拥有 | 不接管 |
| prompt assembly | 拥有 | 提供 guideline、recall output、prompt templates |
| tool routing | 拥有 | 提供 write allowlist 和 validation scripts |
| hook bus | 拥有 | 提供 semantic hook templates |
| scheduler | 拥有 | 提供 scheduled job descriptor；可选提供 maintenance runner |
| memory files | 可读写 | 拥有 `.mnemon` canonical layout、schemas、budgets、scanner |
| skills | 可注册/调用 | 提供 core skill pack |
| reports | 可写 | 定义 report schema 和 templates |
| evaluation | CI/host 执行 | 提供 constraints、datasets、PR template |
| host native files | 拥有 | 感知能力，只写 managed pointer / hook binding |

设计底线：

```text
Harness core 不要求常驻进程。
Harness 不持有 agent state。
Harness 不拦截 LLM 调用。
Harness 不拥有 tool router、hook bus、scheduler。
Harness 不要求 host link runtime library。
Harness 可提供可选 maintenance runner，但 runner 只执行维护 jobs，不拥有 host agent loop。
Harness 拥有 `.mnemon` canonical filesystem，但不拥有 host 原生模板的非托管内容。
```

更精确地说，harness 区分三层：

| Layer | 必需性 | 形态 | 作用 |
|---|---:|---|---|
| Core package | 必需 | Markdown、schemas、skills、hooks、reports | 定义行为资产和安装契约 |
| Filesystem | 必需 | `.mnemon` canonical root | 保存 memory、skills、state、reports、binding metadata |
| Host binding | 按 host 能力 | instruction pointer、skill surface、semantic hook binding | 把 recall/observe/reflect/curate 映射到 host |
| Maintenance runner | 可选 | cron tick / CLI / resident wrapper | 执行 curator、dreaming、index、eval 等维护 jobs |

Runner 的存在不改变 host-owned runtime 原则。它只能处理 maintenance artifacts，不能处理 live user conversation。

## 能力等级

不同 host agent 能力不同，harness 必须可降级安装。

| Level | Host 能力 | 安装 artifacts | 自进化能力 |
|---|---|---|---|
| L0 skill-only | 只能读 Markdown 或手动调用 skills | `GUIDELINE.md`、`skills/recall`、`skills/reflect`、`skills/curate` | 手动 recall/reflect/curate |
| L1 instruction + skill | 支持 project instruction 和 skill discovery | L0 + instruction snippet + skill registry mapping | 稳定遵循 memory/skill 边界，主动提出 proposal |
| L2 lifecycle hooks | 支持 pre/post prompt/tool/session hooks | L1 + `hooks/recall`、`hooks/observe`、`hooks/reflect` | 自动 recall/observe/reflect |
| L3 scheduled/idle | 支持 scheduled task、cron、idle hook，或安装 optional runner | L2 + `hooks/curate`、scheduled descriptor、backup policy、runner job spec | 自动 curator/dreaming |
| L4 eval/CI | 支持 tests、benchmarks、PR flow | L3 + `eval/constraints.yaml`、dataset schema、PR template | 离线 self-evolution |

安装流程首先是 agent-readable 的 hook mounting contract。Host agent 读 `INSTALL.md` 后探测自己的能力，再选择最高可安全安装等级。不能因为 host 缺少 hook 就模拟一个常驻 adapter。

## Harness 数据流

```text
Install time:
  host agent reads INSTALL.md
    -> inventory instruction / skill / hook / scheduler surfaces
    -> choose capability level
    -> create/update `.mnemon` canonical files
    -> write managed instruction pointer
    -> expose core skills
    -> bind semantic hooks if available
    -> write state/install.json
    -> write install report

Task time:
  session_start / pre_llm_call
    -> recall hook or recall skill
    -> short context injected by host

Tool time:
  pre_tool / post_tool
    -> observe hook
    -> evidence appended to long-term episodic memory
    -> usage sidecar updated if host supports it

Post-turn:
  turn_delivered / stop / session_end
    -> reflection prompt
    -> memory/skill proposals
    -> optional allowlisted patch
    -> reflection report

Maintenance:
  idle / scheduled / manual / optional runner
    -> curator dry-run
    -> consolidation / demotion / archive proposals
    -> backup before apply
    -> curator report

Offline:
  eval / CI
    -> candidate generation
    -> constraints
    -> tests / judge
    -> PR proposal
```

## Semantic Events

Harness 定义语义事件，host binding 负责映射到具体平台。

| Event | Purpose | Required? | Fallback |
|---|---|---:|---|
| `session_start` | 加载 guideline、Prompt Memory、skill index | L2 | instruction checklist |
| `pre_llm_call` | 注入 recall/reminder | L2 | manual `recall` skill |
| `pre_tool_call` | safety gate、target allowlist | L2 | host permission + guideline |
| `post_tool_call` | observe evidence、usage signal | L2 | session-end summary |
| `turn_delivered` | post-turn reflection | L2 | `reflect` skill / manual command |
| `pre_compact` | flush continuity | L2/L3 | manual flush before compact |
| `session_end` | summary、reflection proposal | L2 | end checklist |
| `idle_tick` | curator/dreaming | L3 | manual `curate` |
| `scheduled_tick` | periodic maintenance/eval | L3/L4 | external cron / CI |
| `runner_tick` | optional maintenance runner job loop | L3/L4 | host scheduler/manual run |
| `manual_review` | dry-run/apply | L0 | must exist |

## Core Artifacts

Harness 的核心不是对象方法，而是 artifacts：

| Artifact | Role |
|---|---|
| `harness.yaml` | 机器可读 manifest |
| `INSTALL.md` | host agent 可执行安装说明 |
| `GUIDELINE.md` | 行为与记忆准则 |
| `fs.yaml` | canonical filesystem 与 hook mounting policy |
| `bindings/` | active host bindings、hook mapping、projection metadata、drift reports |
| `skills/*/SKILL.md` | core skills |
| `hooks/*` | hook templates |
| `prompts/*.md` | host 调用的 scoped prompts |
| `schemas/*.json` | IO、state、report、proposal、allowlist contracts |
| `scripts/*` | host 可选调用的薄脚本 |
| `memory/` | Prompt Memory、Long-Term Memory 与 consolidation artifacts |
| `state/` | install、usage/provenance sidecar、curator state |
| `reports/` | install、reflection、curator、eval reports |
| `runner/` | optional job descriptors、locks、budgets |
| `eval/` | constraints、datasets、PR templates |

## Filesystem Strategy

Harness 虽然没有 mandatory runtime，但需要自己的文件系统。推荐默认安装到 repo-local `.mnemon/`，并通过 host 原生表面挂载四类语义 hook：

```text
.mnemon canonical state
  -> managed pointer in host instruction surface
  -> core skills exposed through native skill surface or manual reading
  -> recall / observe / reflect / curate bound to host lifecycle hooks
```

原则：

1. `.mnemon` 是 source of truth。
2. Host 原生能力要先感知再绑定。
3. 只修改 managed markers 内的 instruction pointer。
4. Native skill projection 可以 symlink/copy，但只是暴露 `.mnemon` skill，不成为 canonical。
5. Host-owned native content 默认只读；导入时标记为 `user + native_import` 并保护。
6. Curator/dreaming 操作 canonical files，再刷新 bindings/projections。

详细设计见 [10-filesystem-and-host-projection.md](10-filesystem-and-host-projection.md)。

## Safety Model

默认原则：

1. 当前用户请求优先于所有 memory/guideline。
2. 旧 memory 只作参考，不是 system command。
3. facts/preferences 进 memory，procedures/workflows 进 skill。
4. raw evidence 进 long-term episodic memory，不直接进 prompt。
5. 自动写入只允许 allowlist targets。
6. host 不能强制 target allowlist 时，只能 proposal-only。
7. curator 默认 dry-run。
8. archive over delete。
9. pinned/package/imported/user-created artifacts 默认不自动改。
10. 所有 mutation 写 report；高风险 mutation 需要 human approval。

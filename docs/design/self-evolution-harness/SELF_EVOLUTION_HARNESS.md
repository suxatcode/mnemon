# Self-Evolution Harness 设计

本文档是 Mnemon self-evolution harness 的上层架构背景。当前 MVP 的具体设计已拆分为 memory loop 与 skill loop 两个更窄的设计入口。

当前正式 harness 文档入口见 [docs/harness](../../harness/README.md)，其中包含 modular agent harness 设计（[EN](../../harness/modular-agent/DESIGN.md) / [中文](../../harness/modular-agent/DESIGN.zh.md)）、memory loop 设计（[EN](../../harness/memory-loop/DESIGN.md) / [中文](../../harness/memory-loop/DESIGN.zh.md)）与 skill loop 设计（[EN](../../harness/skill-loop/DESIGN.md) / [中文](../../harness/skill-loop/DESIGN.zh.md)）。Issue 入口见 [#10](https://github.com/mnemon-dev/mnemon/issues/10)，初始设计 PR 见 [#9](https://github.com/mnemon-dev/mnemon/pull/9)。

## 1. 背景与决策

Mnemon 当前是一个 LLM-supervised persistent memory binary：宿主 LLM 负责判断，Mnemon binary 负责确定性存储、索引、召回和图结构维护。下一阶段不是把 Mnemon 做成一个新的 agent runtime，而是把它扩展成一个 **agent-agnostic self-evolution harness**。

Harness 的目标是：任何 host agent 只要能读取 Markdown、暴露指令/skill/hook 中的一部分能力，就可以安装 Mnemon 的记忆与自进化行为层。

核心决策：

| 决策 | 结论 |
|---|---|
| 产品形态 | harness，不是 agent framework |
| Runtime 所属 | host agent 拥有 LLM loop、prompt assembly、tool routing、hook bus、scheduler、UI 和权限 |
| Canonical state | `.mnemon` 是 memory、skills、state、reports、bindings 的 source of truth |
| 安装方式 | agent-readable `INSTALL.md` 优先；脚本只是后续便利 |
| 行为资产 | skill-first；workflow/procedure 进入 skills，facts/preferences 进入 memory |
| 记忆结构 | Working Memory + Long-Term Memory + Consolidation |
| 自演化写入 | proposal-first；低风险且可强制 allowlist 时才自动 apply |
| 后台能力 | optional maintenance runner，只运行维护 jobs，不成为第二个 agent |

## 2. 目标与非目标

目标：

- 让 Mnemon 能通过 `INSTALL.md`、`GUIDELINE.md`、skills、hooks、schemas、state 和 reports 安装到不同 host agent。
- 用 `.mnemon` 统一承载 canonical filesystem，避免状态散落到各 host 原生模板。
- 用 recall、observe、reflect、curate 四类语义 hook 描述自进化生命周期。
- 用 Working Memory / Long-Term Memory / Consolidation 描述冷热记忆循环。
- 用 skill index/manage 和 curator 治理程序性记忆。
- 用 risk ladder、static scan、approval、checkpoint/report 控制自演化风险。

非目标：

- 不实现新的 agent runtime。
- 不接管 host 的 prompt assembly 或 tool router。
- 不默认要求 daemon。
- 不为每个 host 写厚 adapter 作为第一阶段架构。
- 不把 long-term recall 当成自动 prompt injection。
- 不允许后台任务静默修改 `GUIDELINE.md`、`INSTALL.md`、hooks、eval constraints 或 host config 非托管区域。

## 3. 核心边界

| 责任 | Host agent | Harness |
|---|---|---|
| LLM 调用 | 拥有 | 不接管 |
| Prompt assembly | 拥有 | 提供 guideline、recall output、scoped prompts |
| Tool routing | 拥有 | 提供 write allowlist、schema、validation scripts |
| Hook bus | 拥有 | 提供 semantic hook templates |
| Scheduler | 拥有 | 提供 scheduled job descriptor；可选 runner tick |
| Permission model | 拥有 | 声明 protected targets 和 risk policy |
| Memory files | 可读写 | 拥有 `.mnemon` canonical layout、budgets、reports |
| Skills | 可注册/调用 | 提供 core skills、skill index/manage contract |
| Reports | 可写 | 定义 report schema 和 templates |
| Host-native files | 拥有 | 只写 managed pointer / hook binding / generated projection |

红线测试：

```text
Can a generic agent still install this by reading INSTALL.md and GUIDELINE.md?
Can the feature degrade to proposal-only Markdown artifacts?
Can the host remain the owner of LLM loop, prompt assembly, tools, hooks, scheduler, UI, and permissions?
```

任一答案为 no，通常说明该能力不属于 harness core。

## 4. 能力等级

不同 host agent 能力不同，harness 必须可降级安装。

| Level | Host 能力 | 安装 artifacts | 自进化能力 |
|---|---|---|---|
| L0 Manual | 只能读 Markdown 或手动调用 skills | `GUIDELINE.md`、core skills | 手动 recall/reflect/curate |
| L1 Instruction | 支持 project instruction 和 skill discovery | L0 + managed instruction pointer + skill registry mapping | 稳定遵循 memory/skill 边界，主动提出 proposal |
| L2 Hooks | 支持 pre/post prompt/tool/session hooks | L1 + `hooks/recall`、`hooks/observe`、`hooks/reflect` | 自动 recall/observe/reflect |
| L3 Maintenance | 支持 scheduled task、cron、idle hook，或可安装 optional runner | L2 + `hooks/curate`、scheduled descriptors、backup policy | curator/dreaming |
| L4 Eval/CI | 支持 tests、benchmarks、PR flow | L3 + `eval/constraints.yaml`、proposal templates | 离线约束和风险评估 |

Installer 选择最高可安全安装等级。缺少 hook 时，不能用常驻 adapter 伪造 host 能力；应降级为 manual skill 或 proposal-only。

## 5. 总体数据流

```text
Install time:
  host agent reads INSTALL.md
    -> inventory instruction / skill / hook / scheduler surfaces
    -> choose capability level
    -> create or update .mnemon canonical files
    -> write managed instruction pointer
    -> expose core skills
    -> bind semantic hooks if available
    -> write bindings/active.json
    -> write install report

Task time:
  session_start / pre_llm_call
    -> recall hook or recall skill
    -> short context returned to host

Tool time:
  pre_tool / post_tool
    -> observe hook
    -> evidence appended to long-term episodic memory
    -> usage sidecar updated if allowed

Post-turn:
  turn_delivered / stop / session_end
    -> reflection prompt
    -> memory/skill proposals
    -> optional allowlisted patch
    -> reflection report

Maintenance:
  idle / scheduled / manual / optional runner
    -> curator and dreaming jobs
    -> consolidation / demotion / archive proposals
    -> backup before apply
    -> curator or dreaming report

Offline:
  eval / CI
    -> constraints
    -> scanner / tests / judge
    -> PR-style proposal
```

## 6. Canonical Filesystem 文件系统

Harness 没有 mandatory runtime，但必须有 durable filesystem。推荐 repo-local `.mnemon/` 作为 canonical root：

```text
.mnemon/
  harness.yaml
  INSTALL.md
  GUIDELINE.md
  fs.yaml
  inventory.json
  bindings/
    active.json
    hosts/
    projections/
  skills/
    core/
      install/SKILL.md
      recall/SKILL.md
      observe/SKILL.md
      reflect/SKILL.md
      curate/SKILL.md
      research/SKILL.md
    project/
    generated/
    archive/
  memory/
    prompt/
      MEMORY.md
      USER.md
      project.md
    longterm/
      episodic/
        evidence/
        transcripts/
        events/
        decisions/
        failures/
      semantic/
        facts/
        preferences/
        summaries/
        topics/
        index/
      imports/
      archive/
        prompt/
    consolidation/
      candidates/
      summaries/
      promotions/
      demotions/
      decisions/
  hooks/
    recall.md
    observe.md
    reflect.md
    curate.md
  prompts/
  schemas/
  scripts/
  state/
    install.json
    usage.json
    curator_state.json
    host_activity.json
    jobs/
    locks/
  reports/
    install/
    reflection/
    curator/
    dreaming/
    projection/
    eval/
  backups/
  runner/
    jobs/
    budgets/
  eval/
    constraints.yaml
    templates/
```

Filesystem tiers：

| Tier | Authority | Examples |
|---|---|---|
| Canonical harness state | `.mnemon` | memory, skills, usage/provenance sidecar, reports, runner jobs |
| Managed bindings | generated from `.mnemon` | instruction pointers, skill projections, hook config |
| Host-owned native content | host/user | existing instructions, user rules, native skills outside markers |

只有 `.mnemon` 是 source of truth。Managed bindings 可重建；host-owned native content 只能感知和尊重，不能静默覆盖。

`fs.yaml` 表达这套规则：

```yaml
schema_version: 1
root: .mnemon
authority: canonical
protected:
  - GUIDELINE.md
  - INSTALL.md
  - harness.yaml
  - schemas/**
  - hooks/**
canonical:
  memory_prompt: memory/prompt
  memory_longterm: memory/longterm
  memory_consolidation: memory/consolidation
  skills_active:
    - skills/core
    - skills/project
    - skills/generated
  skills_archive: skills/archive
  reports: reports
projection:
  managed_marker: mnemon
  default_mode: pointer
  hook_binding_mode: host_native_or_manual
  refresh_events:
    - install
    - upgrade
    - curate_apply
    - skill_promote
drift:
  action: report
  report_dir: reports/projection
```

## 7. 安装与挂载

Installation is not an adapter and not a host-specific runtime. Installation means:

```text
host agent reads INSTALL.md
  -> understands semantic hook contract
  -> maps host lifecycle events to recall / observe / reflect / curate
  -> exposes core skills
  -> points host instructions at .mnemon
  -> records binding
```

Host surface sensing reads capabilities, not product identity:

| Surface | Question |
|---|---|
| Instruction surface | Where can the host read persistent project instructions? |
| Skill surface | Can the host discover `SKILL.md` directories or equivalent commands? |
| Hook surface | Can the host call something on session, model, tool, or stop events? |
| Scheduler surface | Can the host run idle/scheduled maintenance? |
| Permission surface | Can the host restrict write targets? |
| Report surface | Where can the host write human-readable reports? |

Managed instruction block 应保持短，只指向 canonical files：

```markdown
<!-- mnemon:start -->
Mnemon self-evolution harness is installed for this workspace.

Read `.mnemon/GUIDELINE.md` for behavior rules.
Use `.mnemon/skills/core/recall/SKILL.md` before context injection when relevant.
Use `.mnemon/skills/core/observe/SKILL.md` around tool/evidence events when available.
Use `.mnemon/skills/core/reflect/SKILL.md` after completed work.
Use `.mnemon/skills/core/curate/SKILL.md` for maintenance.

Do not copy long memory into this file. `.mnemon` is canonical.
<!-- mnemon:end -->
```

Host owns everything outside the marker.

Binding record：

```yaml
binding:
  schema_version: 1
  host_label: detected-by-agent
  capability_level: L2
  canonical_root: .mnemon
  instruction_surface:
    path: AGENTS.md
    mode: managed_pointer
    marker: mnemon
  skill_surface:
    mode: native|pointer|manual
    targets: []
  hooks:
    recall:
      trigger: user_prompt
      mode: host_hook
      target: .mnemon/hooks/recall.md
    observe:
      trigger: post_tool_call
      mode: host_hook
      target: .mnemon/hooks/observe.md
    reflect:
      trigger: session_end
      mode: host_hook
      target: .mnemon/hooks/reflect.md
    curate:
      trigger: manual
      mode: manual_skill
      target: .mnemon/skills/core/curate/SKILL.md
  write_policy:
    enforced_by_host: true
    default_mode: proposal
```

Projection modes：

| Mode | Use case | Behavior |
|---|---|---|
| `pointer` | host can read referenced files | native file points to `.mnemon/GUIDELINE.md`, Prompt Memory, skill index |
| `managed_block` | instruction file supports Markdown | insert a small marked block; keep user content untouched |
| `hook_binding` | host supports lifecycle or tool hooks | bind host event to `.mnemon/hooks/<name>.md` or core skill |
| `symlink` | host skill loader follows symlinks | symlink active `.mnemon` skill dirs into native skill dir |
| `copy` | host requires physical files | copy generated projections with checksum and source pointer |
| `json_patch` | host has structured config | apply reversible managed patch |
| `native_import` | user has existing native assets | import as user/foreground with protected provenance |

Uninstall removes managed blocks and generated projections but keeps `.mnemon` memory/state/reports/backups unless the user explicitly requests deletion.

## 8. Semantic Hooks 与 Core Skills

Harness defines semantic events; host binding maps them to concrete platform events.

| Event | Purpose | Fallback |
|---|---|---|
| `session_start` | load guideline, Prompt Memory, skill index | instruction checklist |
| `pre_llm_call` | inject recall/reminder | manual `recall` skill |
| `pre_tool_call` | safety gate, target allowlist | host permission + guideline |
| `post_tool_call` | observe evidence, usage signal | session-end summary |
| `turn_delivered` | post-turn reflection | manual `reflect` skill |
| `pre_compact` | flush continuity | manual flush before compact |
| `session_end` | summary, reflection proposal | end checklist |
| `idle_tick` | curator/dreaming | manual `curate` |
| `scheduled_tick` | periodic maintenance/eval | external cron / CI |
| `runner_tick` | optional maintenance runner job loop | host scheduler/manual run |
| `manual_review` | dry-run/apply | must exist |

Hook IO：

```yaml
hook_event:
  hook: recall|observe|reflect|curate
  event_id: string
  host: string
  cwd: string
  trigger: string
  timestamp: string
  payload: object
  budgets:
    latency_ms: 0
    output_chars: 0
  permissions:
    writable_targets: []
    protected_targets: []
```

```yaml
hook_result:
  hook: recall|observe|reflect|curate
  event_id: string
  status: ok|none|proposal|blocked|error
  prompt_addition: string
  writes:
    - target: string
      action: create|patch|append|report
      status: applied|proposed|blocked
  report: string
  warnings: []
```

Core skills：

| Skill | Purpose | Boundary |
|---|---|---|
| `install` | map semantic hooks into current host | ask before host-owned edits; preserve user memory/state |
| `recall` | return short context or `NONE` | never inject raw transcript; no persistent writes |
| `observe` | collect evidence around tools/errors/corrections | evidence only; no semantic long-term conclusion by default |
| `reflect` | post-turn self-improvement review | facts/preferences -> memory; workflows -> skill; proposal-only if no allowlist |
| `curate` | long-term maintenance | dry-run default; archive over delete; skip protected/pinned/user/package/imported |
| `research` | preserve external/source-level research evidence | source links and inference labels required |

Fallbacks are first-class:

| Host capability missing | Behavior |
|---|---|
| No skill system | Use Markdown files and instruction snippets |
| No hooks | Manual `recall`/`reflect`/`curate` skills |
| No write allowlist | Reports only, no direct patch |
| No scheduler | Manual curator or external cron |
| No CI | Eval proposals only |

## 9. 记忆循环 Memory Loop

Architecture names use cognitive terms; implementation paths use engineering terms:

```text
Cognitive model:
Working Memory  <->  Memory Consolidation  <->  Long-Term Memory

Engineering model:
Prompt Memory   <->  Dreaming Jobs         <->  Mnemon Store + Skills
```

| Cognitive role | Engineering implementation | Filesystem owner | Purpose |
|---|---|---|---|
| Working Memory | Prompt Memory / Markdown Memory | `memory/prompt/` | small, high-confidence memory injected into host prompt |
| Episodic Memory | Evidence / Event Log | `memory/longterm/episodic/` | events, transcripts, tool outputs, decisions, failures |
| Semantic Memory | Mnemon Store | `memory/longterm/semantic/` | facts, preferences, summaries, project knowledge, indexes |
| Procedural Memory | Skills | `skills/` | reusable workflows, tactics, procedures, habits |
| Memory Consolidation | Dreaming Jobs | `memory/consolidation/`, `reports/dreaming/` | compact, archive, extract, promote, and propose skills |

### Working Memory

Working Memory is bounded Markdown directly loaded into the host prompt snapshot:

```text
memory/prompt/
  MEMORY.md
  USER.md
  project.md
```

It should contain stable user preferences, durable project facts, environment facts repeatedly needed by the agent, short high-confidence constraints, and compact lessons not better represented as skills.

It should not contain raw transcripts, long logs, one-off task progress, temporary TODOs, low-confidence inference, or procedural workflows.

Recommended budgets:

| File | Target |
|---|---:|
| `MEMORY.md` | 2k-4k chars |
| `USER.md` | 1k-2k chars |
| `project.md` | 2k-6k chars |

Overflow creates consolidation/demotion proposals, not silent truncation.

### Long-Term Memory

Long-Term Memory is not one storage mechanism:

```text
Long-Term Memory
  episodic   -> Mnemon evidence/event storage
  semantic   -> Mnemon facts/summaries/preferences/indexes
  procedural -> skills
```

Properties:

- large capacity and long retention;
- searchable and rankable;
- not fully loaded into prompt;
- can store raw evidence and long histories;
- can use Mnemon, RAG, SQLite/FTS, vector search, graph storage, or another backend;
- lower immediate reliability than Prompt Memory because recall is selective;
- source of candidates for Prompt Memory promotion and skill creation.

Long-Term Memory is not "bad memory". Prompt Memory is small and high-performance; Long-Term Memory is larger, longer-lived, and retrieved only when relevant.

### Daily Write Path

Foreground agents should not perform complex semantic long-term writes by default:

```text
interaction
  -> append low-cost evidence/event log
  -> maintain Prompt Memory when explicitly asked or when the host memory tool permits it
  -> defer semantic extraction and skill generation to Dreaming Jobs
```

Evidence event:

```yaml
type: evidence_event
timestamp: 2026-05-09T00:00:00Z
source: post_tool_call|user_correction|turn_summary|failure|manual_import
scope:
  user: optional
  project: optional
  branch: optional
summary: "The build failed because pnpm was missing from PATH."
refs:
  transcript: memory/longterm/episodic/transcripts/session-abc.md
  tool_call: optional
sensitivity: public|internal|secret-redacted
candidate_for:
  - semantic
  - skill
```

### Consolidation

Dreaming Jobs implement consolidation. Dreaming is not a free-form background agent; it is scoped jobs with schemas, budgets, reports, and write allowlists.

| Job | Reads | Writes | Purpose |
|---|---|---|---|
| `compact` | `memory/prompt/**` | prompt patch proposal | keep Working Memory under quota |
| `archive` | prompt entries, evidence events | `memory/longterm/archive/prompt/**` | preserve demoted prompt memory |
| `extract` | evidence, transcripts, summaries | semantic memory proposal | turn evidence into facts/preferences/summaries |
| `promote` | semantic memory, recall hits, user confirmations | prompt patch proposal | reactivate durable facts into Working Memory |
| `skill-review-signal` | repeated workflows, failures, tool traces | reflection/curator report or `skills/generated/**` via skill_manage | feed procedures into skill path |

Movement protocol:

| Gate | Direction | Trigger | Writes |
|---|---|---|---|
| G1 Capture | interaction -> episodic | observe/reflect/pre-compact/import | evidence events, transcripts, summaries |
| G2 Compact | prompt -> prompt proposal | quota pressure/staleness/conflict | compact patch proposal |
| G3 Extract | episodic -> semantic | stable fact detected | semantic proposal |
| G4 Promote | semantic -> prompt | high confidence/frequency/scope match | prompt patch proposal |
| G5 Proceduralize | repeated experience -> skill | repeated workflow or tool tactic | skill_manage patch/create/write_file proposal |

Promotion to Prompt Memory requires strong evidence:

```text
importance >= threshold
AND confidence >= threshold
AND recurrence >= threshold OR user_confirmed
AND risk <= allowed_risk
AND prompt_budget_available OR replacement_plan_exists
AND not better_as_skill
AND evidence_links_present
```

Demotion triggers include budget pressure, staleness, supersession, too much detail, low usage, conflict, or a better representation as skill. Default behavior is archive over delete.

### Recall

Long-Term recall is retrieval, not memory loading.

Rules:

- raw transcript is never injected;
- recall is summarized and evidence-linked;
- current user request outranks recall;
- irrelevant long-term memory returns `NONE`;
- repeated useful recall can create a consolidation candidate;
- recall context is not automatically promoted to Prompt Memory.

Ranking fields include relevance, recency, frequency, confidence, scope match, importance, risk, and budget cost.

## 10. 技能演进 Skill Evolution

Procedural memory lives in skills. The compact loop is:

```text
skills_list / skill_view
  -> skill_manage
  -> usage sidecar
  -> background review
  -> curator
```

Skill artifact:

```text
skills/<namespace>/<name>/
  SKILL.md
  references/
  templates/
  scripts/
  assets/
```

`SKILL.md` frontmatter stays small:

```yaml
---
name: debug-build-failures
description: Diagnose recurring build failures by checking environment, dependency, cache, and test signals.
---
```

Rules:

- `name` is stable, lowercase, filesystem-safe, and class-level.
- `description` tells the model when to load the skill.
- Operational state lives in `state/usage.json`, not frontmatter.
- Long session detail moves to `references/`.
- Reusable starter files move to `templates/`.
- Deterministic checks move to `scripts/`.
- Binary or media assets move to `assets/`.

Skill manage surface:

| Action | Meaning | Default policy |
|---|---|---|
| `create` | create a new `SKILL.md` | foreground-confirmed or background review |
| `patch` | replace unique string in `SKILL.md` or support file | preferred update path |
| `edit` | rewrite full `SKILL.md` | major overhaul only |
| `write_file` | add/update support file | preferred for long details |
| `remove_file` | remove support file | report required |
| `delete` | remove from active library | maps to archive for recoverability |

Usage sidecar:

```json
{
  "schema_version": 1,
  "skills": {
    "debug-build-failures": {
      "created_by": "agent",
      "provenance": "background_review",
      "state": "active",
      "pinned": false,
      "use_count": 3,
      "view_count": 7,
      "patch_count": 1,
      "created_at": "2026-05-09T00:00:00Z",
      "last_used_at": "2026-05-09T00:00:00Z",
      "last_viewed_at": "2026-05-09T00:00:00Z",
      "last_patched_at": "2026-05-09T00:00:00Z",
      "archived_at": null,
      "absorbed_into": null
    }
  }
}
```

Lifecycle is deliberately small:

```text
active -> stale -> archived
```

`pinned` is orthogonal. Pinned skills are skipped by curator but can still be patched when explicitly requested.

Auto-curation eligibility:

```text
created_by == "agent"
AND provenance in {"background_review", "curator"}
AND pinned != true
AND state in {"active", "stale"}
AND target not protected
```

### Three Production Entrances

| Entrance | Trigger | Policy |
|---|---|---|
| User-declared | user explicitly asks to save/update a procedure | protected by default; curator does not silently change |
| Agent-offered | foreground agent notices reusable procedure and asks user | no confirmation, no durable write |
| Background review | post-turn `reflect` hook/job | may create self-authored skills; curator-eligible by default |

Review preference order:

1. Update a currently loaded skill.
2. Update an existing umbrella skill.
3. Add a support file under an existing umbrella.
4. Create a new class-level umbrella skill.
5. Say "nothing to save" when no real signal exists.

Curator is not a fourth per-turn production entrance. It maintains library shape across time: mark stale, archive, merge narrow skills into umbrella skills, move useful detail into support files, skip protected/pinned/user/package/imported assets, snapshot before apply, and write reports.

Memory/skill boundary:

| Signal | Destination |
|---|---|
| user preference or durable fact | Working Memory / Long-Term Memory |
| reusable workflow or tool tactic | Skill |
| raw logs, traces, failures | episodic Long-Term Memory |
| repeated procedural pattern found during maintenance | skill patch/create through review or curator |

## 11. 可选 Maintenance Runner

Harness core does not need a daemon. A daemon is justified only for maintenance work that is periodic, low-priority, evidence-heavy, and unsafe to run inside an active user turn. The correct abstraction is a maintenance runner:

```text
cron / host scheduler / manual CLI
  -> runner tick
  -> lease
  -> budget
  -> scoped job
  -> report / proposal / allowlisted apply
  -> ledger
```

The runner is optional. L0/L1 installs should not include it. L2 can usually rely on host lifecycle hooks. L3/L4 may install it when the host lacks a scheduler or when dreaming/index/eval jobs need durable execution.

Runner boundaries:

- does not handle user messages;
- does not assemble the main prompt;
- does not inject memory into live turns;
- does not intercept host LLM calls;
- does not hold a separate model API key by default;
- does not route arbitrary tools;
- does not approve dangerous actions;
- does not watch the whole filesystem and mutate opportunistically.

Job taxonomy:

| Type | Uses LLM | Default write mode | Output |
|---|---:|---|---|
| `reflect.deferred` | yes | proposal | `reports/reflection/*`, optional proposal patch |
| `curator.transitions` | no | apply to state only | usage state transitions, stale markers |
| `curator.review` | yes | dry-run/proposal | consolidation/archive proposal |
| `dreaming.light` | no/optional | consolidation candidate write | candidate extraction from recent evidence |
| `dreaming.rem` | yes | report-only | theme report |
| `dreaming.deep` | yes | proposal | promotion/demotion proposals |
| `longterm.index.incremental` | no | apply to index only | FTS/vector metadata |
| `longterm.index.rebuild` | no | apply to index only | rebuilt index |
| `eval.batch` | yes/optional | proposal | eval report / PR text |
| `snapshot.rotate` | no | apply | backup manifest cleanup |

LLM jobs call a declared host command and validate output schema before any apply step:

```yaml
host_llm:
  command: ["claude", "-p"]
  stdin: prompt
  timeout_seconds: 600
  output_schema: schemas/proposal.schema.json
  allowed_tools: []
```

Stronger rule:

```text
one job step -> one scoped prompt -> one bounded LLM response -> schema validation
```

The runner cannot run open-ended observe/think/act loops.

## 12. Eval 与风险控制

Day-to-day self-evolution should use layered risk control, not a heavy always-on benchmark system.

```text
candidate change
  -> classify target and risk
  -> validate schema / path / size / budget
  -> scan for injection / exfiltration / destructive / persistence patterns
  -> apply trust policy
  -> choose allow / proposal / approval / block
  -> optional checkpoint
  -> apply or write report
```

Risk ladder:

| Level | Targets | Default outcome |
|---|---|---|
| R0 telemetry | `reports/**`, `state/usage.json`, non-mutating dry-run output | auto write |
| R1 self-authored skill patch | generated skill patch/support file with valid schema and clean scan | allow if host enforces target; otherwise proposal |
| R2 memory movement | Prompt Memory promotion/demotion, semantic extraction, recall ranking changes | proposal unless explicit low-risk policy allows |
| R3 harness behavior | `GUIDELINE.md`, `INSTALL.md`, hook prompts, hook mounting policy, eval constraints | human approval only |
| R4 hardline | secret exfiltration, destructive filesystem ops, hidden instructions, safety weakening, host config outside marker | block |

R4 is not "needs approval"; it is blocked from self-evolution. A human may still edit the file outside the harness.

Trust policy:

| Source | Safe | Caution | Dangerous |
|---|---|---|---|
| package/builtin | allow | allow | block unless package upgrade is explicitly reviewed |
| user-declared | allow | ask/report | ask/report |
| agent-created foreground | allow | proposal | block or ask |
| background review / curator | allow inside allowlist | proposal | block |
| imported/community | allow after scan | proposal | block |

Scanner checks:

- prompt injection and hidden instruction patterns;
- credential exfiltration and secret references;
- destructive commands and filesystem wipe patterns;
- persistence mechanisms such as cron, shell rc, service files, startup hooks;
- network exposure and tunneling;
- obfuscation, encoded execution, invisible Unicode;
- structural limits: file count, total size, single-file size, symlink escape, suspicious binary files.

Background rules:

- no interactive approval is assumed;
- `reflect`, `curate`, and `dreaming` default to report/proposal;
- low-risk R0 writes may apply;
- R1 applies only when target allowlist, scanner, schema, and provenance gates pass;
- R2/R3 become proposals;
- R4 blocks.

Every durable mutation beyond R0 should create a rollback point when the host can support it. If no checkpoint exists, the mutation should remain proposal-only or include enough diff context for manual rollback.

## 13. Reports 审计面

Reports are the audit surface. Every durable change must answer:

1. What changed or would change?
2. Was it prompt promotion, demotion, long-term recall, semantic extraction, evidence capture, or skill proposal?
3. Why?
4. Which evidence supports it?
5. What scores and thresholds were used?
6. Was it applied or only proposed?
7. How can it be rolled back?

Report metadata:

```yaml
report:
  id: string
  type: install|reflection|curator|dreaming|eval|migration|skill-production
  host: string
  capability_level: string
  started_at: string
  finished_at: string
  mode: dry-run|proposal|apply
  summary: string
  actions: []
  warnings: []
  errors: []
  evidence: []
```

Durable changes without reports are architecture violations.

## 14. 关键 Schemas 附录

Schemas 是契约，不要求所有 host 使用同一种实现。Host 可以用 JSON Schema、YAML 校验、脚本校验或人工 review，但字段语义应一致。

### 14.1 Write Target Allowlist

`schemas/write-target-allowlist.schema.json` 表达 install-time 写入策略。它连接 risk ladder 与 host 权限执行。

```json
{
  "allow": [
    "memory/**",
    "skills/**",
    "state/**",
    "reports/**",
    "archive/**"
  ],
  "protect": [
    "INSTALL.md",
    "GUIDELINE.md",
    "harness.yaml",
    "hooks/**",
    "eval/**",
    "schemas/**"
  ],
  "approval_required": [
    "GUIDELINE.md",
    "INSTALL.md",
    "harness.yaml",
    "hooks/**",
    "eval/**"
  ],
  "hardline_block": [
    "host_config_outside_marker",
    "secret_exfiltration",
    "destructive_filesystem_operation",
    "safety_policy_weakening"
  ]
}
```

If host cannot enforce this allowlist, reflection, curator, and dreaming jobs run proposal-only.

Risk result:

```yaml
risk:
  level: R0|R1|R2|R3|R4
  source: user|agent|background_review|curator|imported|package
  verdict: safe|caution|dangerous
  decision: allow|proposal|approval_required|block
  reasons: []
  required_gates:
    - target-allowlist
    - schema-validation
    - static-scan
    - budget-check
    - report-written
```

### 14.2 Inventory

`inventory.json` records what the installing agent detected. It is evidence for the install plan, not a host adapter.

```json
{
  "schema_version": 1,
  "host_label": "detected-by-agent",
  "detected_at": "2026-05-10T00:00:00Z",
  "surfaces": {
    "instruction": [
      {
        "path": "AGENTS.md",
        "mode": "markdown",
        "managed_marker_supported": true
      }
    ],
    "skills": [
      {
        "path": ".claude/skills",
        "mode": "directory",
        "supports_symlink": true
      }
    ],
    "hooks": [
      {
        "event": "post_tool_call",
        "mode": "host_config",
        "write_target_enforcement": true
      }
    ],
    "scheduler": [],
    "permissions": {
      "can_restrict_write_targets": true,
      "requires_human_approval_for_host_config": true
    }
  },
  "warnings": []
}
```

### 14.3 Bindings And Projections

`bindings/active.json` records current host bindings and generated projections. Projection state is regenerable; canonical state is not.

```json
{
  "schema_version": 1,
  "host": "detected-by-agent",
  "canonical_root": ".mnemon",
  "capability_level": "L2",
  "instruction_surface": {
    "path": "AGENTS.md",
    "mode": "managed_block",
    "marker": "mnemon",
    "checksum": "sha256:..."
  },
  "semantic_hooks": {
    "recall": {
      "trigger": "pre_llm_call",
      "mode": "host_hook",
      "target": ".mnemon/hooks/recall.md"
    },
    "observe": {
      "trigger": "post_tool_call",
      "mode": "host_hook",
      "target": ".mnemon/hooks/observe.md"
    },
    "reflect": {
      "trigger": "session_end",
      "mode": "host_hook",
      "target": ".mnemon/hooks/reflect.md"
    },
    "curate": {
      "trigger": "manual",
      "mode": "manual_skill",
      "target": ".mnemon/skills/core/curate/SKILL.md"
    }
  },
  "projections": [
    {
      "id": "native-skill-dev-server",
      "source": ".mnemon/skills/generated/dev-server/SKILL.md",
      "target": ".claude/skills/dev-server/SKILL.md",
      "mode": "symlink|copy|pointer",
      "checksum": "sha256:...",
      "generated_at": "2026-05-10T00:00:00Z"
    }
  ],
  "write_policy": {
    "enforced_by_host": true,
    "default_mode": "proposal"
  }
}
```

### 14.4 Runner Job Descriptor

Runner jobs are optional. Defaults should be disabled until installation explicitly enables them.

```yaml
job:
  id: dreaming-nightly
  type: dreaming.deep
  enabled: false
  trigger:
    kind: schedule
    interval_hours: 24
    min_idle_minutes: 30
  mode: dry-run
  inputs:
    - memory/longterm/episodic/evidence/**
    - memory/longterm/semantic/summaries/**
    - memory/consolidation/**
    - state/usage.json
  outputs:
    - reports/dreaming/**
    - memory/consolidation/candidates/**
  write_allowlist:
    - reports/dreaming/**
    - memory/consolidation/**
    - state/jobs/**
  budgets:
    max_runtime_seconds: 1800
    max_llm_calls: 8
    max_input_chars: 200000
    max_output_chars: 30000
    max_files_touched: 50
  locking:
    resources:
      - memory
      - usage
    stale_after_seconds: 7200
  kill_switch:
    file: state/runner.disabled
```

Apply is allowed only when all gates pass:

```text
job.enabled == true
AND mode == apply
AND lease acquired
AND backup succeeded
AND output schema valid
AND target in job write_allowlist
AND target in global allowlist
AND target not protected
AND target not pinned
AND provenance allows automated mutation
```

### 14.5 Job Ledger

Every runner attempt writes a ledger entry.

```json
{
  "schema_version": 1,
  "job_id": "dreaming-nightly",
  "job_type": "dreaming.deep",
  "status": "proposal_written",
  "mode": "dry-run",
  "started_at": "2026-05-10T00:00:00Z",
  "finished_at": "2026-05-10T00:12:00Z",
  "inputs": [
    "memory/longterm/semantic/summaries/**",
    "memory/longterm/episodic/evidence/**",
    "memory/consolidation/**"
  ],
  "outputs": [
    "reports/dreaming/2026-05-10.md"
  ],
  "budgets": {
    "llm_calls": 3,
    "input_chars": 84500,
    "output_chars": 9400
  },
  "mutations": [],
  "warnings": []
}
```

### 14.6 Backup Manifest

Backup before mutating:

- `skills/**`
- `memory/prompt/**`
- `memory/consolidation/**`
- `state/usage.json`

Backup manifest:

```yaml
backup:
  id: string
  reason: pre-curator-apply
  created_at: "2026-05-10T00:00:00Z"
  files:
    - source: skills/generated/dev-server/SKILL.md
      backup: backups/2026-05-10/dev-server/SKILL.md
      checksum: sha256:...
  report: reports/curator/2026-05-10.md
```

If a host cannot create backup or rollback context, apply mode should downgrade to proposal-only.

## 15. 实施路线 Roadmap

| Phase | Goal | Key deliverables | Acceptance |
|---|---|---|---|
| Phase 0: Spec Package | create `.mnemon` skeleton with no host automation | `harness.yaml`, `INSTALL.md`, `GUIDELINE.md`, `fs.yaml`, schemas, core skills, report templates | generic agent can install L0 manually |
| Phase 1: L1 Installable Harness | bind instruction, skill, and semantic hook surfaces | install skill, managed pointer, inventory, `bindings/active.json`, install state/report | reinstall is idempotent; uninstall preserves memory/state/reports |
| Phase 2: L2 Hooks | add recall/observe/reflect hook templates | hook IO schema, allowlist schema, scan/validate scripts | recall returns `NONE`; observe writes evidence; reflect proposal-only without allowlist |
| Phase 3a: L3 Curator Skill | maintenance governance without owning host runtime | `curate`, curator prompt/hook, snapshot/rollback, curator state/report | dry-run report; apply requires backup; protected artifacts skipped |
| Phase 3b: Optional Runner | cron/lease/ledger execution for async maintenance | job schemas, queue/done state, runner tick, kill switch | disabling runner does not disable manual skills |
| Phase 4: Memory Consolidation | connect Prompt Memory with Mnemon-backed episodic/semantic memory and skills | consolidation schema, promotion prompt, recall ranking, `NONE` gate | raw transcripts never inject directly; promotions link evidence |
| Phase 5: Eval-Driven Evolution | add lightweight risk gates | constraints, scanner, risk classifier, approval reports, rollback pointers | R2/R3 proposal by default; R4 blocked |

First implementation should start with:

```text
.mnemon/
  fs.yaml
  inventory.json
  bindings/active.json
  harness.yaml
  INSTALL.md
  GUIDELINE.md
  skills/core/{recall,reflect,curate}/SKILL.md
  schemas/{skill,usage,proposal,report,write-target-allowlist}.schema.json
  reports/templates/{reflection,curator}.md
  state/{install,usage}.json
```

Do not start by writing a daemon, server, SDK, database adapter, or universal agent wrapper.

## 16. Anti-Patterns 反模式

The harness fails if it becomes a hidden agent framework or makes self-evolution unreviewable.

| Anti-pattern | Correct shape |
|---|---|
| Harness assembles full prompt | Host assembles prompt; harness provides guideline, recall output, prompt templates |
| Harness routes tools | Host owns tool routing; harness provides allowlists, validation, reports |
| Hidden LLM client | LLM jobs call declared host command; missing command means proposal/manual |
| Opportunistic file watcher | Writes happen through semantic events, queued jobs, manual commands, or scheduled ticks |
| Database replaces Markdown control plane | Markdown remains behavior control plane; DB/index is implementation detail |
| Unlimited skill creation | Patch umbrella skills first; one-off detail remains evidence/session summary |
| Auto-mutating user/package assets | Provenance gates; user/package/imported/pinned protected by default |
| Policy changes through self-evolution | `GUIDELINE.md`, `INSTALL.md`, hooks, schemas, eval policy require human approval |
| Prompt Memory as transcript cache | Prompt Memory stays short and declarative; evidence goes long-term |
| Maintenance marketed as intelligence | Runner is cron + lease + ledger, not a brain |
| Host-native state as source of truth | `.mnemon` is canonical; host-native files are pointers/projections/bindings |

Architecture checklist:

1. Expressible as Markdown, schema, thin script, hook template, report, or optional job descriptor.
2. Runs without owning host agent loop.
3. Can be disabled without losing manual skill operation.
4. Has explicit input/output contracts.
5. Writes reports for durable changes.
6. Respects provenance and protected targets.
7. Can degrade to proposal-only.

## 17. 研究摘要 Research Synthesis

Research was used to identify common patterns and boundaries; it is not architecture naming. The design borrows only portable mechanisms.

| System | Useful reference | What Mnemon adopts | What Mnemon avoids |
|---|---|---|---|
| Claude Code | Markdown memory, project instructions, hooks, skills/commands | Markdown as behavior surface; lifecycle hooks; user/project memory separation | tying architecture to one product template |
| Codex | `AGENTS.md`, hooks, skills, generated memories | agent-readable instructions; local skill packages; hookable lifecycle | assuming one fixed host path |
| OpenClaw | active memory, dreaming, plugin hooks | consolidation as scheduled/idle maintenance; memory wiki as long-term pattern | making heavy runtime mandatory |
| Hermes | bounded Markdown memory, skills, curator, usage sidecar, background review | small Prompt Memory, procedural skills, curator governance, report-first maintenance | copying product shape or host-specific home directory |
| Letta | structured long-term memory, archival/recall/core memory distinction | separation between prompt-facing and archival memory | requiring a full stateful agent runtime |
| ALMA | memory-structure experimentation and meta-learning | future eval/research signal for memory evolution | generating runtime code as first-stage self-evolution |
| Agno | application-framework memory manager and explicit optimization | explicit memory optimization and summaries | turning Mnemon into an app framework |

Cross-system conclusions:

1. Markdown remains the most portable agent behavior control plane.
2. Skills are the natural carrier for procedural memory.
3. Prompt-facing memory must stay small and reviewable.
4. Large memory needs retrieval, evidence links, and consolidation rather than full prompt loading.
5. Background maintenance needs provenance, reports, backups, and hard write boundaries.
6. Host-specific adapters should be convenience scripts, not the core architecture.

Source provenance is kept in [Agent Systems Research](research/agent-systems/README.md). Detailed per-system notes were intentionally folded into this synthesis to keep the architecture maintainable.

## 18. 成功标准 Success Criteria

The first usable harness is successful when:

1. It can be installed manually in a generic agent using only Markdown.
2. It can be installed in at least one hook-capable host at L2.
3. It produces reflection proposals after a task.
4. It never patches outside write allowlist.
5. It preserves memory/state/reports across reinstall and upgrade.
6. It can run curator dry-run and produce a useful report.
7. Users can inspect every durable change as a Markdown diff.
8. The architecture is explainable from this single document plus the interactive HTML map.

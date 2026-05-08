# Self-Evolution Harness 详细设计

本目录把 `docs/research/hermes-self-evolution.md` 的研究结论转成可实现架构。目标不是实现一个新的 agent framework，而是实现一个 **agent-agnostic harness package**：通过 `INSTALL.md`、`GUIDELINE.md`、skills、hooks、schemas、state 和 reports 安装到任意 host agent 上，让 host agent 获得自进化能力。

## 设计目标

Self-Evolution Harness 应满足：

1. **Host-owned runtime**：LLM loop、tool router、hook bus、scheduler、UI、permission model 都归 host agent。
2. **Harness-owned filesystem**：harness 拥有 `.mnemon` canonical filesystem；host 原生文件只是 projection/binding。
3. **Installable everywhere**：Claude Code、Codex、Cursor、Continue、Hermes、OpenClaw、generic agent 都可按能力等级安装。
4. **Everything is skill**：流程、工具经验、操作方法主要沉淀为 skill；memory 只保存 facts/preferences。
5. **Hot/warm/cold memory**：模型直接消费 hot；warm 承载整理 capsule；cold 承载 evidence、history、index。
6. **Proposal-first evolution**：默认先写 reports/proposals；只有低风险、allowlist 内、host 可强制权限时才自动 patch。
7. **No mandatory agent runtime**：harness core 不要求常驻进程，不持有 agent state，不接管任何 host execution surface；可选 maintenance runner 只执行维护 jobs。

## 总体形态

```text
.mnemon/
  harness.yaml
  INSTALL.md
  GUIDELINE.md
  fs.yaml
  inventory.json
  install/
    hosts/
      claude-code.yaml
      codex.yaml
      cursor.yaml
      continue.yaml
      hermes.yaml
      generic.yaml
  bindings/
    active.json
    projections/
  skills/
    core/
      install/
      recall/
      observe/
      reflect/
      curate/
      research/
    project/
    generated/
      active/
      quarantine/
      candidates/
    archive/
  hooks/
    recall/
    observe/
    reflect/
    curate/
  prompts/
    recall.md
    reflection.md
    curator.md
    promotion.md
  schemas/
    harness.schema.json
    install-map.schema.json
    skill.schema.json
    hot-memory.schema.json
    usage.schema.json
    hook-io.schema.json
    proposal.schema.json
    report.schema.json
    write-target-allowlist.schema.json
  scripts/
    scan-memory-write
    validate-skill
    check-target-allowlist
    snapshot
    rollback
  memory/
    hot/
    warm/
    cold/
  state/
    install.json
    usage.json
    curator_state.json
    pins.json
    lineage.json
  reports/
    install/
    reflection/
    curator/
    dreaming/
    eval/
  runner/
    jobs/
    locks/
    budgets/
  eval/
    constraints.yaml
    templates/
      pr.md
```

## 文档地图

| 文档 | 内容 |
|---|---|
| [01-architecture.md](01-architecture.md) | 总体架构、边界、能力等级、数据流 |
| [02-installation-contract.md](02-installation-contract.md) | `harness.yaml`、`INSTALL.md`、host binding、升级/卸载 |
| [03-artifacts-and-schemas.md](03-artifacts-and-schemas.md) | 主要 artifacts 和 schemas 的详细字段 |
| [04-skills-and-hooks.md](04-skills-and-hooks.md) | core skills、四阶段 hooks、fallback 规则 |
| [05-memory-curation-eval.md](05-memory-curation-eval.md) | hot/warm/cold、curator、dreaming、eval gate |
| [06-implementation-roadmap.md](06-implementation-roadmap.md) | MVP、阶段计划、验收标准 |
| [07-maintenance-runner.md](07-maintenance-runner.md) | 可选 daemon/runner 的边界、jobs、状态、锁、预算 |
| [08-skill-production-paths.md](08-skill-production-paths.md) | foreground、post-turn review、maintenance synthesis 三条 skill 生产路径 |
| [09-anti-patterns.md](09-anti-patterns.md) | 防止 harness 滑成 agent framework 的反模式清单 |
| [10-filesystem-and-host-projection.md](10-filesystem-and-host-projection.md) | `.mnemon` canonical filesystem、host template sensing、projection/mount 策略 |
| [architecture-site.html](architecture-site.html) | 交互式 HTML 架构地图、管道流、host projection explorer |

## 架构一句话

Self-Evolution Harness 是一套可安装的行为资产、文件系统与维护契约。它把 canonical state 放在 `.mnemon`，把 host 原生模板当作 projection/binding，并让 host agent 在自己的生命周期事件上执行 recall、observe、reflect、curate 四类语义动作。

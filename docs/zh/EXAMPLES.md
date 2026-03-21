# Mnemon — Remember + Link 用例集

> `remember` 存储记忆节点并自动创建边；`link` 手动补充自动机制无法确认的边（通常来自 `semantic_candidates` 和 `causal_candidates`）。

---

## 目录

1. [架构决策 + 原因（因果边）](#1-架构决策--原因因果边)
2. [用户偏好 + 上下文（语义边）](#2-用户偏好--上下文语义边)
3. [故障排查链（时序 + 因果边）](#3-故障排查链时序--因果边)
4. [实体知识网络（实体边）](#4-实体知识网络实体边)
5. [完整工作流：从 remember 到 link](#5-完整工作流从-remember-到-link)
6. [参数速查](#6-参数速查)

---

## 1. 架构决策 + 原因（因果边）

**场景**：记录技术选型决策及其背后的原因，用因果边把「原因」指向「决策」。

```bash
# 存储决策
mnemon remember "选择 SQLite 而非 PostgreSQL 作为 mnemon 的存储引擎" \
  --cat decision --imp 5 \
  --tags "数据库,架构" \
  --entities "SQLite,PostgreSQL,mnemon"
# → 返回 id: aaa-111

# 存储原因
mnemon remember "选择 SQLite 是因为它支持零依赖的 Go 二进制分发，无需额外数据库进程" \
  --cat insight --imp 4 \
  --tags "数据库,架构" \
  --entities "SQLite,Go"
# → 返回 id: bbb-222

# 手动建立因果边：原因 → 决策
mnemon link bbb-222 aaa-111 --type causal --weight 0.9
```

**要点**：因果边的方向是「原因 → 结果」。当用 `recall "为什么选择 SQLite"` 时，WHY intent 会优先沿因果边遍历，并用 Kahn 拓扑排序将原因排在结果前面。

---

## 2. 用户偏好 + 上下文（语义边）

**场景**：记录用户的沟通偏好和触发该偏好的具体反馈，用语义边关联。

```bash
# 存储偏好
mnemon remember "用户偏好简洁回复，不要在每条回复末尾做总结" \
  --cat preference --imp 4 \
  --tags "沟通,风格" \
  --source agent
# → 返回 id: ccc-333

# 存储触发上下文
mnemon remember "用户纠正：不要在回复末尾总结你刚做了什么，我能看到 diff" \
  --cat context --imp 3 \
  --tags "沟通,反馈" \
  --source agent
# → 返回 id: ddd-444

# 语义关联
mnemon link ccc-333 ddd-444 --type semantic --weight 0.8
```

**要点**：`remember` 会自动创建余弦相似度 > 0.80 的语义边。0.40–0.80 之间的候选项会在 `semantic_candidates` 中返回，由 agent/用户通过 `link` 确认。

---

## 3. 故障排查链（时序 + 因果边）

**场景**：一次线上故障的三个阶段——现象、根因、修复，用时序边串联时间线，因果边表达推理关系。

```bash
# 现象
mnemon remember "2026-03-10 API 延迟飙升至 5 秒，用户报告超时错误" \
  --cat fact --imp 4 \
  --tags "事故,延迟,API" \
  --entities "API,延迟"
# → 返回 id: eee-555

# 根因
mnemon remember "根因：重试逻辑中数据库连接泄漏导致连接池耗尽" \
  --cat insight --imp 5 \
  --tags "事故,数据库,连接池" \
  --entities "连接池,重试逻辑"
# → 返回 id: fff-666

# 修复
mnemon remember "修复方案：在 retryHandler 中添加 defer conn.Close()，并将连接池最大空闲超时设为 30 秒" \
  --cat decision --imp 4 \
  --tags "事故,修复" \
  --entities "retryHandler,连接池"
# → 返回 id: ggg-777

# 因果边：根因 → 现象
mnemon link fff-666 eee-555 --type causal --weight 0.95

# 时序边：现象 → 修复（时间线顺序）
mnemon link eee-555 ggg-777 --type temporal --weight 0.8

# 因果边：根因 → 修复
mnemon link fff-666 ggg-777 --type causal --weight 0.85
```

**要点**：三个节点 + 三条边构成一个小型事故知识子图。后续 `recall "连接池耗尽"` 能沿图一次性检索出完整的排查链。

---

## 4. 实体知识网络（实体边）

**场景**：围绕同一实体（React）的多条知识点，用实体边聚合。

```bash
# 知识点 1
mnemon remember "React 19 引入了 use() hook，允许在渲染时读取资源" \
  --cat fact --imp 3 \
  --tags "前端,react" \
  --entities "React,use-hook"
# → 返回 id: hhh-888

# 知识点 2
mnemon remember "React 19 Server Components 可将客户端 bundle 减少 30-50%" \
  --cat fact --imp 3 \
  --tags "前端,react,性能" \
  --entities "React,Server-Components"
# → 返回 id: iii-999

# 知识点 3
mnemon remember "团队于 2026-02-20 完成从 React 18 到 React 19 的迁移" \
  --cat fact --imp 4 \
  --tags "前端,迁移" \
  --entities "React"
# → 返回 id: jjj-000

# 实体边：共享 React 实体
mnemon link hhh-888 iii-999 --type entity --weight 0.7
mnemon link hhh-888 jjj-000 --type entity --weight 0.6
mnemon link iii-999 jjj-000 --type entity --weight 0.6
```

**要点**：`remember` 会自动检测实体共现并创建实体边。但如果两条记忆创建时间间隔较远，自动机制可能遗漏——此时需要手动 `link`。

---

## 5. 完整工作流：从 remember 到 link

实际使用中，`remember` 的 JSON 输出包含 `id`、自动创建的边统计、以及需要人工确认的候选项：

```bash
$ mnemon remember "MAGMA 论文采用四图架构进行记忆检索" \
    --cat fact --imp 4 --entities "MAGMA"
```

输出（简化）：

```json
{
  "id": "a1b2c3d4-...",
  "action": "added",
  "edges_created": {
    "temporal": 1,
    "entity": 2,
    "causal": 0,
    "semantic": 1
  },
  "semantic_candidates": [
    {
      "id": "e5f6g7h8-...",
      "similarity": 0.72,
      "snippet": "RLM 范式与 MAGMA 方法论的关系..."
    }
  ],
  "causal_candidates": [
    {
      "id": "k9l0m1n2-...",
      "overlap": 0.25,
      "snippet": "选择 MAGMA 是因为..."
    }
  ]
}
```

根据候选项确认边：

```bash
# 确认语义候选（相似度 0.72，在 0.40-0.80 区间，需人工判断）
mnemon link a1b2c3d4 e5f6g7h8 --type semantic --weight 0.72

# 确认因果候选
mnemon link k9l0m1n2 a1b2c3d4 --type causal --weight 0.8
```

---

## 6. 参数速查

### remember 参数

| 参数 | 默认值 | 说明 |
|---|---|---|
| `--cat` | `general` | 分类：`preference` / `decision` / `fact` / `insight` / `context` / `general` |
| `--imp` | `3` | 重要性：1–5（4-5 免疫自动清理） |
| `--tags` | | 逗号分隔的标签（最多 20 个） |
| `--entities` | | 逗号分隔的实体（与自动提取合并，最多 50 个） |
| `--source` | `user` | 来源：`user` / `agent` / `external` |
| `--no-diff` | `false` | 跳过重复/冲突检测 |

### link 参数

| 参数 | 默认值 | 说明 |
|---|---|---|
| `--type` | `semantic` | 边类型：`temporal` / `semantic` / `causal` / `entity` |
| `--weight` | `0.5` | 边权重：0.0–1.0 |
| `--meta` | | JSON 格式的元数据，例如 `'{"reason":"主题相近"}'` |

### 四种边类型

| 类型 | 语义 | 自动创建条件 | 典型手动场景 |
|---|---|---|---|
| `temporal` | 时间先后 | 同 source 的骨干链 + 24h 窗口内近邻 | 跨 source 的时间线关联 |
| `semantic` | 主题相似 | 余弦相似度 > 0.80 | 0.40–0.80 区间的候选确认 |
| `causal` | 因果关系 | 关键词检测 + token overlap > 0.15 | agent 推理出的隐含因果 |
| `entity` | 实体共现 | 自动实体提取后共现 | 跨时间窗的实体聚合 |

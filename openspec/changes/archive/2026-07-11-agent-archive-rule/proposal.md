# AGENTS.md：推送前自动归档规则

## 动机

之前代码变更推送后，AI 未自动更新 `openspec/changes/archive/`。这违反了项目约定 — 每次变更都应有归档记录。需要在 `AGENTS.md` 中明确写入，让 AI 每次推送前自动执行。

## 方案

在 `AGENTS.md` 第 4 步新增一条规则：

> **Archive every change before pushing**: After making code changes and committing, create a dated archive entry under `openspec/changes/archive/YYYY-MM-DD-<slug>/` with `.openspec.yaml`, `proposal.md`, and `tasks.md`. Follow the same format as existing entries. Commit the archive separately before `git push`. Do NOT skip this step even if the user doesn't ask.

## 影响范围

- 仅 `AGENTS.md` 文本变更，无代码影响
- 后续所有 AI 辅助变更都会自动生成归档

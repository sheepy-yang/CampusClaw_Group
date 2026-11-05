# Proposal: 2026-11-05-learning-state-review-path

## Why

当前仓库已经具备个性化学习模块的基础结构，但复习路径尚未形成可从页面到服务端的完整主路径：

- `api/internal/store/memory.go` 已有 `LearningEvent`、`Mistake`、`ContentRecord`，并提供按学生和班级查询事件、错题的内存方法；现有演示数据也包含近两周学习记录。
- `api/internal/teaching/modules/tutoring/module.go` 已有 `Services.GenerateReviewPath`，但目前只统计错题数量，没有按近两周过滤，也没有利用学习事件或任务完成情况。
- `api/internal/httpserver/teaching_routes.go` 中 `POST /api/v1/skills/review-path` 仍调用通用骨架 Pipeline，返回 501；现有 `StudentHome` 页面已经调用该地址。
- 当前 `GET /api/v1/contents/{id}` 对学生只检查班级和发布状态，没有检查复习路径的 `studentId`，同班学生可能访问他人的个性化路径。
- 学习事件目前没有明确的结果类型；复习任务完成后也没有统一写回学习事件的流程。

本变更先建立可复用的 Learning State Service，并打通个性化复习路径这一条端到端主路径，为后续追问辅导和自适应练习提供统一的学习证据读取与事件写入契约。

## What Changes

- 新增共享学习状态应用能力：按当前学生、当前班级和最近 14 天汇总 `LearningEvent` 与 `Mistake`，形成可解释的知识点状态，不生成没有证据支持的精确掌握度分数。
- 扩展学习事件结果语义，区分正确、错误、已理解、已完成和未知；旧事件缺少结果字段时按 `unknown` 兼容处理。
- 为学习状态与复习路径建立 Repository Port，并提供基于现有 `store.Memory` 的适配器；V1 不新增 MySQL 迁移。
- 完善 `GenerateReviewPath`：无有效学习证据时不调用模型；有证据时生成 3～5 步计划，并校验步骤类型、知识点归属和字段长度。
- 实现学生查询本人学习状态、生成/查询本人复习路径、完成复习步骤的 HTTP 接口。
- 复习步骤完成后幂等地追加 `review_task/completed` 学习事件；自测仍不计入正式成绩，也不因“完成”直接提升掌握等级。
- 修复复习路径的资源可见性：学生只能查看自己的路径；本班教师可按学生 ID 查看；跨班或其他学生访问统一返回 404。
- 更新学生复习页面、OpenAPI、单元/集成测试和固定 AI 评测样本。
- 将本 Change 纳入 `make verify` 的 OpenSpec 严格校验范围。

## Out of Scope

- 追问式辅导 Agent、追问策略和答案释放条件。
- 自适应练习状态机、题目判分、难度调整和前置知识回退。
- 课堂回答聚类、Embedding 聚类和教师课堂互动页面。
- `mcp-teaching-tools` 的工具发现、调用和审计实现。
- 修改杨瑾文的评价业务逻辑；本 Change 只提供可供其他模块调用的学习事件写入契约，跨模块事件接入另行协商。
- MySQL 持久化迁移、Qdrant 知识图谱或复杂掌握度算法。
- 复杂知识图谱推理、强化学习推荐和实时推送。

## Prerequisite

仓库当前 `AGENTS.md` 与 `openspec/config.yaml` 仍声明处于第 7 课骨架阶段，禁止提前实现教学业务。本 Change 可以先编写和严格校验规约，但开始任何业务代码任务前，项目提交人必须完成或正式记录骨架阶段人工交付项的例外，并将项目阶段约束更新为“仅实现已通过严格校验的 OpenSpec Change”。不得绕过该阶段门禁直接开发。

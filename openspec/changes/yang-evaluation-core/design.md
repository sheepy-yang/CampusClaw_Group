# Design: yang-evaluation-core

## Approach

- 领域逻辑集中在 `api/internal/teaching/modules/evaluation`
- 异步任务经 `JobSink` 投递，由 `queue.Worker` 调用 `Process*` 回写
- 班级讲评使用 `security.AnonymizeForModel`
- 班级讲评模型输出先归一化再校验；不合规时降级为证据频次统计讲评（`generationMode=evidence_fallback`），彻底失败则写入 `generationError` 避免永久 `generating`
- 主观题确认后按转写置信度与依据充分性分支 `needs_teacher_review` / `graded_suggested`，并追加学习事件供诊断
- 原创性审查 `SimilarityReport.Verdict` 恒为 `null`

## Data

- `store.Submission`：转写、步骤、建议分、改分历史、快照、引用声明、AI 使用说明
- `store.LearningEvent`：诊断时序证据
- `store.ContentRecord`（skill=`class_review` / `diagnosis`）：待确认 → 已发布

## Risks

- 演示环境为内存存储，进程重启丢失；Compose 已预留 MySQL/Redis 替换点

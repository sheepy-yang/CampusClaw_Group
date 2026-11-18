# Tasks: yang-evaluation-core

- [x] 多模态批改服务与客观题规则判分
- [x] Worker：批改 / OCR / 讲评 / 诊断 / 相似性
- [x] 班级讲评人数门槛与匿名化
- [x] 学习诊断样本门槛、证据事件、教师放行
- [x] 作答快照策略与相似性证据（无自动结论）
- [x] 单元测试（evaluation + security anonymize）
- [x] 教师端批改 / 讲评 / 诊断 / 回放页面
- [x] OpenSpec change 文档
- [x] HTTP 端到端失败路径：权限隔离、样本门槛、keydown 拒绝、verdict=null
- [x] 演示种子：已发布主观题作业 + 诊断学习事件 Outcome
- [x] 教师回放页改为只读复核（禁止代替学生写快照）
- [x] 讲评模型 JSON 归一化与证据降级；失败写入 generationError
- [x] 主观题确认按转写置信度/依据分支 needs_teacher_review，并追加学习事件
- [x] 教师端讲评失败可观测；诊断 mastery level 与后端枚举对齐

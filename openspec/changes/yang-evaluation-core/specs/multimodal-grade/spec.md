# Spec: multimodal-grade

## 正常路径

- **Given** 学生提交主观题文本或手写图片摘要  
- **When** 调用 `POST /api/v1/skills/multimodal-grade`  
- **Then** 先落库再入队；任务完成后状态为待确认转写，含步骤划分、首错定位与建议分

- **Given** 客观题答案与标准答案  
- **When** 调用 `POST /api/v1/skills/multimodal-grade/objective`  
- **Then** 使用确定性规则判分，`usedLLM=false`

- **Given** 学生确认转写  
- **When** 调用 confirm-transcription  
- **Then** 低置信度进入 `needs_teacher_review`，否则采用建议分

- **Given** 教师改分  
- **When** 调用 override  
- **Then** 最终分以教师为准，并写入 `scoreHistory`

## 越权

- **Given** 学生 Token  
- **When** 访问其他学生的 submission  
- **Then** 返回 404

## 依赖故障

- **Given** 模型失败（simulateFail）  
- **When** 处理批改任务  
- **Then** 提交记录仍保留，状态为 `failed_submission_kept`

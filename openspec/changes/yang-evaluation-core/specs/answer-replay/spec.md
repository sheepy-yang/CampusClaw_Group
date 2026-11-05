# Spec: answer-replay

## 快照策略

- **Given** trigger=`manual` 或 `interval`  
- **When** 保存快照  
- **Then** 写入版本时间线

- **Given** trigger=`keydown`  
- **When** 保存快照  
- **Then** 拒绝（不采集逐键）

## 原创性审查

- **Given** 已有作答文本与讲义/历史版本  
- **When** 拉取 similarity  
- **Then** 返回相似片段、来源、定位；`verdict` 恒为 null

## 可见性

- **Given** 学生 Token  
- **When** 查看他人 attempt 时间线  
- **Then** 404

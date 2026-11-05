# Spec: diagnosis

## 样本不足

- **Given** 学生学习事件 &lt;3  
- **When** 生成诊断  
- **Then** 仅返回「样本不足」，`intercepted=true`，`studentVisible=false`

## 正常路径

- **Given** 事件 ≥3  
- **When** 生成诊断并完成 Worker  
- **Then** 每项掌握结论带 `evidence` 事件 ID；默认对学生不可见

- **Given** 教师确认放行  
- **When** `POST .../diagnosis/{id}/release`  
- **Then** `studentVisible=true` 且内容状态为 published

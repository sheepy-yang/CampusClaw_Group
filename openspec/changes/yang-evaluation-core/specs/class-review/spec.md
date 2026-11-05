# Spec: class-review

## 正常路径

- **Given** 有效反馈 ≥5 条  
- **When** 教师生成讲评  
- **Then** 送模前匿名化，异步生成提纲/例题/补救题，标记 `teacherOnly`

## 人数门槛

- **Given** 有效提交 &lt;5  
- **When** 生成讲评  
- **Then** `skipped=true`，不调用模型

## 隐私

- **Given** 含「李同学」等姓名的输入  
- **When** 匿名化  
- **Then** 模型输入不再包含「X同学」称谓

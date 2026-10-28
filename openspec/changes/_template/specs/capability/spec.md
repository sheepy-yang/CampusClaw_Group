# Spec: \<capability\>

## Scenario: happy path

- **Given** TODO
- **When** TODO
- **Then** TODO

## Scenario: unauthorized

- **Given** 学生身份
- **When** 调用教师专属接口
- **Then** 返回 403

## Scenario: cross-class

- **Given** 跨班资源 ID
- **When** 访问
- **Then** 返回 404（与不存在同形）

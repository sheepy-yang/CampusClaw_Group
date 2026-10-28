# OpenSpec

每项功能先建立 change，以 Given / When / Then 描述：

- 正常路径
- 越权
- 输入非法
- 依赖故障

建议目录：

```text
openspec/changes/<change-id>/
  proposal.md
  design.md
  specs/<capability>/spec.md
  tasks.md
```

示例占位见 `changes/_template/`。

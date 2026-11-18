# 杨瑾文模块 · 测试条例执行结果

- 条例：`docs/yang-evaluation/llm-test-cases.jsonl`
- 结果：`docs/yang-evaluation/llm-case-results.jsonl`
- 网关：`http-gateway`
- 用例：78（调 Chat：31）
- 通过：78
- 失败：0
- 生成时间：2026-11-18T01:59:06Z

| ID | 技能 | 标题 | 调模型 | 结果 | 耗时ms | 说明 |
| --- | --- | --- | --- | --- | --- | --- |
| `cr-001` | class_review | 五人双高频错因 | 是 | 通过 | 9751 | ok |
| `cr-002` | class_review | 五人单一错因主导 | 是 | 通过 | 6603 | ok |
| `cr-003` | class_review | 五人互异错因 | 是 | 通过 | 13441 | ok |
| `cr-004` | class_review | 六人重复秩混淆 | 是 | 通过 | 10560 | ok |
| `cr-005` | class_review | 五人含长描述错因 | 是 | 通过 | 8028 | ok |
| `cr-006` | class_review | 五人短标签 | 是 | 通过 | 8911 | ok |
| `cr-007` | class_review | 七人混合 | 是 | 通过 | 10318 | ok |
| `cr-008` | class_review | 五人同质几乎相同 | 是 | 通过 | 5906 | ok |
| `cr-skip-001` | class_review | 不足5人不调模型 | 否 | 通过 | 0 | ok |
| `cr-skip-002` | class_review | 刚好4人边界 | 否 | 通过 | 0 | ok |
| `cr-skip-003` | class_review | 0人提交 | 否 | 通过 | 0 | ok |
| `cr-var-001` | class_review | 变体错因集:主元为0未换行 | 是 | 通过 | 11039 | ok |
| `cr-var-002` | class_review | 变体错因集:特解构造错误 | 是 | 通过 | 11939 | ok |
| `cr-var-003` | class_review | 变体错因集:通解参数化混乱 | 是 | 通过 | 11859 | ok |
| `cr-var-004` | class_review | 变体错因集:矩阵乘法当消元 | 是 | 通过 | 8598 | ok |
| `cr-var-005` | class_review | 变体错因集:把齐次当非齐次 | 是 | 通过 | 10231 | ok |
| `dx-001` | diagnosis | 全错需支持 | 否 | 通过 | 0 | ok |
| `dx-002` | diagnosis | 混合掌握 | 否 | 通过 | 0 | ok |
| `dx-003` | diagnosis | 全对示范 | 否 | 通过 | 0 | ok |
| `dx-004` | diagnosis | 样本刚好3 | 否 | 通过 | 0 | ok |
| `dx-005` | diagnosis | 多样本跨知识点 | 否 | 通过 | 0 | ok |
| `dx-006` | diagnosis | 未知结果为主 | 否 | 通过 | 0 | ok |
| `dx-chat-001` | diagnosis | 单证据诊断对照 | 是 | 通过 | 2513 | ok |
| `dx-chat-002` | diagnosis | 双知识点对照 | 是 | 通过 | 3409 | ok |
| `dx-chat-003` | diagnosis | 进步趋势对照 | 是 | 通过 | 3550 | ok |
| `dx-chat-004` | diagnosis | 短证据 | 是 | 通过 | 5601 | ok |
| `dx-chat-005` | diagnosis | 长证据 | 是 | 通过 | 6496 | ok |
| `dx-chat-006` | diagnosis | 空趋势压力 | 是 | 通过 | 3855 | ok |
| `dx-insuf-001` | diagnosis | 样本不足拦截 | 否 | 通过 | 0 | ok |
| `obj-001` | multimodal_grade | 选择题正确 | 否 | 通过 | 0 | ok |
| `obj-002` | multimodal_grade | 选择题错误 | 否 | 通过 | 0 | ok |
| `obj-003` | multimodal_grade | 选择题空白归一化 | 否 | 通过 | 0 | ok |
| `obj-004` | multimodal_grade | 选择题空答 | 否 | 通过 | 0 | ok |
| `obj-005` | multimodal_grade | 填空多标准答案命中其一 | 否 | 通过 | 0 | ok |
| `obj-006` | multimodal_grade | 填空命中第二标准 | 否 | 通过 | 0 | ok |
| `obj-007` | multimodal_grade | 填空错误 | 否 | 通过 | 0 | ok |
| `obj-008` | multimodal_grade | blank 正确 | 否 | 通过 | 0 | ok |
| `obj-009` | multimodal_grade | blank 空白归一化 | 否 | 通过 | 0 | ok |
| `obj-010` | multimodal_grade | 近似但不完全相等 | 否 | 通过 | 0 | ok |
| `obj-extra-001` | multimodal_grade | 客观题边角#1 | 否 | 通过 | 0 | ok |
| `obj-extra-002` | multimodal_grade | 客观题边角#2 | 否 | 通过 | 0 | ok |
| `obj-extra-003` | multimodal_grade | 客观题边角#3 | 否 | 通过 | 0 | ok |
| `obj-extra-004` | multimodal_grade | 客观题边角#4 | 否 | 通过 | 0 | ok |
| `sim-001` | answer_replay | 高度相似同伴作答 | 否 | 通过 | 1 | ok |
| `sim-002` | answer_replay | 低相似同伴 | 否 | 通过 | 0 | ok |
| `sim-003` | answer_replay | 仅历史版本相似 | 否 | 通过 | 0 | ok |
| `sim-004` | answer_replay | 空历史仍可比同伴 | 否 | 通过 | 0 | ok |
| `sim-005` | answer_replay | 几乎复述同伴 | 否 | 通过 | 0 | ok |
| `sim-006` | answer_replay | 短作答 | 否 | 通过 | 0 | ok |
| `snap-001` | answer_replay | 允许 manual 快照 | 否 | 通过 | 0 | ok |
| `snap-002` | answer_replay | 允许 interval 快照 | 否 | 通过 | 0 | ok |
| `snap-003` | answer_replay | 拒绝 keydown 键采 | 否 | 通过 | 0 | ok |
| `snap-004` | answer_replay | 拒绝未知 trigger | 否 | 通过 | 0 | ok |
| `snap-005` | answer_replay | 拒绝空 trigger | 否 | 通过 | 0 | ok |
| `step-001` | multimodal_grade | 步骤切分#1 | 否 | 通过 | 0 | ok |
| `step-002` | multimodal_grade | 步骤切分#2 | 否 | 通过 | 0 | ok |
| `step-003` | multimodal_grade | 步骤切分#3 | 否 | 通过 | 0 | ok |
| `step-004` | multimodal_grade | 步骤切分#4 | 否 | 通过 | 0 | ok |
| `sug-001` | multimodal_grade | 高匹配三步骤 | 否 | 通过 | 0 | ok |
| `sug-002` | multimodal_grade | 部分匹配 | 否 | 通过 | 0 | ok |
| `sug-003` | multimodal_grade | 几乎无关 | 否 | 通过 | 0 | ok |
| `sug-004` | multimodal_grade | 空作答 | 否 | 通过 | 0 | ok |
| `sug-005` | multimodal_grade | 只有第一步 | 否 | 通过 | 0 | ok |
| `sug-006` | multimodal_grade | 英文混写 | 否 | 通过 | 0 | ok |
| `sug-007` | multimodal_grade | 重复啰嗦高覆盖 | 否 | 通过 | 0 | ok |
| `sug-008` | multimodal_grade | 漏写增广只写系数 | 否 | 通过 | 0 | ok |
| `tr-001` | multimodal_grade | 标准三步骤手写 | 是 | 通过 | 1537 | ok |
| `tr-002` | multimodal_grade | 含涂改痕迹文本 | 是 | 通过 | 1918 | ok |
| `tr-003` | multimodal_grade | 缺步骤 | 是 | 通过 | 1257 | ok |
| `tr-004` | multimodal_grade | 乱序步骤 | 是 | 通过 | 1403 | ok |
| `tr-005` | multimodal_grade | 含无法辨认占位 | 是 | 通过 | 2224 | ok |
| `tr-006` | multimodal_grade | 长文本作答 | 是 | 通过 | 1883 | ok |
| `tr-007` | multimodal_grade | 只有结论 | 是 | 通过 | 1092 | ok |
| `tr-008` | multimodal_grade | 计算错误自述 | 是 | 通过 | 1621 | ok |
| `tr-extra-001` | multimodal_grade | 转写边角#1 | 是 | 通过 | 946 | ok |
| `tr-extra-002` | multimodal_grade | 转写边角#2 | 是 | 通过 | 884 | ok |
| `tr-extra-003` | multimodal_grade | 转写边角#3 | 是 | 通过 | 1238 | ok |
| `tr-extra-004` | multimodal_grade | 转写边角#4 | 是 | 通过 | 1813 | ok |

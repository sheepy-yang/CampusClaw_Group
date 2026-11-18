# 杨瑾文模块 · 调大模型后的结果汇总

- 生成方式：`go test ./internal/teaching -run TestYangLLMOutputSummary`
- 网关模式：`http-gateway`（由 `.env` 中 LLM_* / EMBEDDING_* 决定；无完整密钥时为 offline-demo）
- 用例数：6（其中实际调 Chat：3）
- 通过：6
- 失败：0
- 原始证据：`docs/yang-evaluation/llm-output-results.jsonl`
- 生成时间：2026-11-18T01:45:37Z

> 说明：班级讲评、图片转写会调用模型网关；客观/主观建议分、诊断落库、相似证据按规约为规则或证据绑定，**不**调用大模型。诊断另附一条直接 Chat 对照。

## 班级讲评（调模型）

### `process-with-gateway` · 调模型 · 通过

- 说明：讲评 JSON 已落库
- 模型：`course-chat`

**输入**

```json
{
  "anonymousInputs": [
    "行变换不完整",
    "秩概念混淆",
    "行变换不完整",
    "增广矩阵漏变右端",
    "秩概念混淆"
  ],
  "gateway": "http-gateway"
}
```

**输出**

```json
{
  "anonymous": true,
  "anonymousInputs": [
    "行变换不完整",
    "秩概念混淆",
    "行变换不完整",
    "增广矩阵漏变右端",
    "秩概念混淆"
  ],
  "commonCauses": [
    "行变换不完整",
    "秩概念混淆",
    "增广矩阵漏变右端"
  ],
  "examples": [
    "行变换不完整",
    "秩概念混淆",
    "行变换不完整",
    "增广矩阵漏变右端",
    "秩概念混淆"
  ],
  "inputCount": 5,
  "model": "course-chat",
  "outline": [
    "行变换操作不完整",
    "矩阵核心概念误解",
    "增广矩阵处理疏漏"
  ],
  "remedial": [
    "强化行变换步骤训练，确保操作完整无遗漏",
    "深入理解秩的概念及相关性质，结合实例辨析",
    "练习增广矩阵完整变换，同步处理右端项"
  ],
  "teacherOnly": true
}
```

## 多模态批改（转写调模型 / 建议分不调）

### `image-transcription-chat` · 调模型 · 通过

- 说明：模型转写原始输出（text-fallback）
- 模型：`course-chat`

**输入**

```json
{
  "gateway": "http-gateway",
  "mode": "text-fallback",
  "system": "手写作答转写器"
}
```

**输出**

```json
{
  "mode": "text-fallback",
  "raw": {
    "confidence": 1,
    "text": "步骤1：写出增广矩阵\n步骤2：交换第1、2行\n步骤3：回代得 x=1（计算有误）"
  },
  "text": "{\"text\":\"步骤1：写出增广矩阵\\n步骤2：交换第1、2行\\n步骤3：回代得 x=1（计算有误）\",\"confidence\":1.0}",
  "tokens": 451
}
```

### `subjective-suggest-after-confirm` · 不调模型 · 通过

- 说明：确认转写后建议分走规则/评分细则，usedLLM=false

**输入**

```json
{
  "answer": "步骤1：写出增广矩阵\n步骤2：初等行变换求阶梯型\n步骤3：比较秩判断有解",
  "rubric": "写出增广矩阵并做初等行变换，比较系数矩阵与增广矩阵秩。\n秩相等则有解。"
}
```

**输出**

```json
{
  "confidence": 0.576923076923077,
  "errorCause": "与教师发布的评分依据匹配不足",
  "steps": [
    {
      "index": 1,
      "text": "步骤1：写出增广矩阵",
      "ok": false,
      "verified": false,
      "comment": "待教师依据已发布评分规则核验"
    },
    {
      "index": 2,
      "text": "步骤2：初等行变换求阶梯型",
      "ok": false,
      "verified": false,
      "comment": "待教师依据已发布评分规则核验"
    },
    {
      "index": 3,
      "text": "步骤3：比较秩判断有解",
      "ok": false,
      "verified": false,
      "comment": "待教师依据已发布评分规则核验"
    }
  ],
  "suggested": 4.2,
  "usedLLM": false
}
```

## 学习诊断（生产不调模型 + Chat 对照）

### `process-evidence-bound` · 不调模型 · 通过

- 说明：证据绑定 mastery/trend，未调用 Chat

**输入**

```json
{
  "evidenceEventIds": [
    "ev-1",
    "ev-2",
    "ev-3",
    "ev-4",
    "llm-ev-0",
    "llm-ev-1",
    "llm-ev-2"
  ]
}
```

**输出**

```json
{
  "evidenceEvents": [
    {
      "at": "2026-11-15T01:45:20.985839Z",
      "id": "ev-1",
      "kind": "assignment",
      "kp": "高斯消元",
      "outcome": "incorrect",
      "summary": "作业#1 行交换后未同步右端"
    },
    {
      "at": "2026-11-16T01:45:20.985839Z",
      "id": "ev-2",
      "kind": "practice",
      "kp": "矩阵秩",
      "outcome": "incorrect",
      "summary": "练习#2 混淆两种秩"
    },
    {
      "at": "2026-11-17T01:45:20.985839Z",
      "id": "ev-3",
      "kind": "tutoring",
      "kp": "高斯消元",
      "outcome": "understood",
      "summary": "辅导追问：增广矩阵概念"
    },
    {
      "at": "2026-11-17T13:45:20.985839Z",
      "id": "ev-4",
      "kind": "help",
      "kp": "高斯消元",
      "outcome": "completed",
      "summary": "课堂求助：回代计算"
    },
    {
      "at": "2026-11-18T01:45:32.022217Z",
      "id": "llm-ev-0",
      "kind": "assignment",
      "kp": "高斯消元",
      "outcome": "incorrect",
      "summary": "高斯消元练习 1：行变换后未同步右端"
    },
    {
      "at": "2026-11-18T00:45:32.022217Z",
      "id": "llm-ev-1",
      "kind": "assignment",
      "kp": "高斯消元",
      "outcome": "incorrect",
      "summary": "高斯消元练习 2：行变换后未同步右端"
    },
    {
      "at": "2026-11-17T23:45:32.022217Z",
      "id": "llm-ev-2",
      "kind": "assignment",
      "kp": "高斯消元",
      "outcome": "incorrect",
      "summary": "高斯消元练习 3：行变换后未同步右端"
    }
  ],
  "mastery": [
    {
      "evidence": [
        "ev-2"
      ],
      "kp": "矩阵秩",
      "level": "needs_support",
      "observedEvents": 1
    },
    {
      "evidence": [
        "ev-1",
        "ev-3",
        "ev-4",
        "llm-ev-0",
        "llm-ev-1",
        "llm-ev-2"
      ],
      "kp": "高斯消元",
      "level": "mixed",
      "observedEvents": 6
    }
  ],
  "requiresConfirm": true,
  "studentId": "stu-1",
  "studentVisible": false,
  "trend": "按时间顺序提供证据；未从未知结果推断趋势"
}
```

### `direct-chat-contrast` · 调模型 · 通过

- 说明：对照：直接 Chat（生产 ProcessDiagnosis 不走此路径）
- 模型：`course-chat`

**输入**

```json
{
  "gateway": "http-gateway",
  "purpose": "contrast-only"
}
```

**输出**

```json
{
  "raw": {
    "mastery": [
      {
        "evidence": [
          "行变换后未同步右端"
        ],
        "kp": "高斯消元",
        "level": "未掌握"
      }
    ],
    "trend": "待提升"
  },
  "text": "{\"mastery\":[{\"kp\":\"高斯消元\",\"level\":\"未掌握\",\"evidence\":[\"行变换后未同步右端\"]}],\"trend\":\"待提升\"}",
  "tokens": 771
}
```

## 作答回放·相似证据（不调模型）

### `similarity-evidence` · 不调模型 · 通过

- 说明：相似证据已生成，verdict 恒为 null，未调用 Chat/Embed

**输入**

```json
{
  "attemptId": "attempt-llm-1"
}
```

**输出**

```json
{
  "items": [
    {
      "fragment": "高斯消元通过初等行变换求阶梯型后比较秩。",
      "location": "约第1字起",
      "similarity": 0.71,
      "source": "同次作业匿名答案 #1"
    },
    {
      "fragment": "高斯消元通过初等行变换求阶梯型后比较秩",
      "location": "约第1字起",
      "similarity": 0.67,
      "source": "同次作业匿名答案 #1"
    },
    {
      "fragment": "高斯消元通过初等行变换",
      "location": "约第1字起",
      "similarity": 0.52,
      "source": "本人历史版本 v1"
    }
  ],
  "usedLLM": false,
  "verdict": null
}
```


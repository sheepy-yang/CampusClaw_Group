#!/usr/bin/env python3
"""按杨瑾文前端页面实际调用路径做输入/输出冒烟，写出 JSONL + Markdown 汇总。"""

from __future__ import annotations

import json
import os
import sys
import time
import urllib.error
import urllib.request
import uuid
from collections import defaultdict
from pathlib import Path

BASE = os.environ.get("BASE_URL", "http://127.0.0.1:8080").rstrip("/")
ROOT = Path(__file__).resolve().parents[1]
OUT_JSONL = ROOT / "docs" / "yang-evaluation" / "frontend-io-results.jsonl"
OUT_MD = ROOT / "docs" / "yang-evaluation" / "frontend-io-results.md"


def call(method: str, path: str, token: str | None = None, body: dict | None = None, headers: dict | None = None):
    data = None if body is None else json.dumps(body, ensure_ascii=False).encode()
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    for key, value in (headers or {}).items():
        req.add_header(key, value)
    try:
        with urllib.request.urlopen(req, timeout=20) as resp:
            raw = resp.read().decode()
            return resp.status, json.loads(raw) if raw else None
    except urllib.error.HTTPError as err:
        raw = err.read().decode()
        try:
            payload = json.loads(raw) if raw else {"message": err.reason}
        except json.JSONDecodeError:
            payload = {"message": raw or err.reason}
        return err.code, payload


def brief(resp) -> str:
    if not isinstance(resp, dict):
        return str(resp)[:140]
    parts = []
    for key in (
        "status",
        "submissionId",
        "attemptId",
        "contentId",
        "skipped",
        "intercepted",
        "studentVisible",
        "verdict",
        "code",
        "message",
        "authoritative",
        "after",
        "anonymized",
        "sampleCount",
        "token",
    ):
        if key in resp:
            value = resp[key]
            if key == "token" and isinstance(value, str):
                value = value[:12] + "…"
            parts.append(f"{key}={value!r}")
    return ", ".join(parts) if parts else json.dumps(resp, ensure_ascii=False)[:160]


def main() -> int:
    rows: list[dict] = []

    def record(case: str, page: str, method: str, path: str, note: str, token=None, body=None, headers=None, expect=None):
        status, resp = call(method, path, token=token, body=body, headers=headers)
        row = {
            "case": case,
            "page": page,
            "method": method,
            "path": path,
            "status": status,
            "note": note,
            "expect": expect,
            "request": body,
            "response": resp,
            "pass": None,
        }
        if expect is None:
            row["pass"] = 200 <= status < 300
        elif isinstance(expect, int):
            row["pass"] = status == expect
        elif callable(expect):
            row["pass"] = bool(expect(status, resp))
        rows.append(row)
        mark = "PASS" if row["pass"] else "FAIL"
        print(f"[{mark}] {status} {method} {path}  ({case})")
        print("  ", brief(resp))
        return status, resp

    # 首页登录
    _, teacher = record(
        "login-teacher", "首页登录", "POST", "/api/v1/auth/login",
        "教师账号密码登录", body={"username": "teacher", "password": "demo123"}, expect=200,
    )
    teacher_token = teacher["token"]
    _, student = record(
        "login-student", "首页登录", "POST", "/api/v1/auth/login",
        "学生账号密码登录", body={"username": "student", "password": "demo123"}, expect=200,
    )
    student_token = student["token"]
    _, student2 = record(
        "login-student2", "首页登录", "POST", "/api/v1/auth/login",
        "另一学生登录", body={"username": "student2", "password": "demo123"}, expect=200,
    )
    student2_token = student2["token"]

    # 学生主观题
    _, assignments = record(
        "list-assignments", "学生·主观题提交", "GET", "/api/v1/classes/class-demo/assignments",
        "列出已发布作业", token=student_token, expect=200,
    )
    assignment_id = next(a["id"] for a in assignments if a.get("status") == "published" and a.get("items"))
    key = f"multimodal-grade:{uuid.uuid4()}"
    _, queued = record(
        "grade-enqueue", "学生·主观题提交", "POST", "/api/v1/skills/multimodal-grade",
        "提交文本作答（带 Idempotency-Key）",
        token=student_token,
        body={
            "assignmentId": assignment_id,
            "answerText": "步骤1：写出增广矩阵\n步骤2：初等行变换求阶梯型\n步骤3：比较秩判断有解",
        },
        headers={"Idempotency-Key": key},
        expect=202,
    )
    submission_id = queued["submissionId"]

    status_name = queued.get("status")
    for i in range(1, 9):
        if status_name not in ("queued", "ocr_processing"):
            break
        time.sleep(0.25)
        _, polled = record(
            f"grade-poll-{i}", "学生·主观题提交", "GET", f"/api/v1/skills/multimodal-grade/{submission_id}",
            "轮询直到待确认转写", token=student_token, expect=200,
        )
        status_name = polled.get("status")

    record(
        "grade-confirm", "学生·确认转写", "POST", "/api/v1/skills/multimodal-grade/confirm-transcription",
        "确认转写并进入建议评分",
        token=student_token,
        body={"submissionId": submission_id, "text": ""},
        expect=lambda s, r: s == 200 and r.get("submission", {}).get("status") == "needs_teacher_review",
    )
    record(
        "grade-isolation", "学生·主观题提交", "GET", f"/api/v1/skills/multimodal-grade/{submission_id}",
        "其他学生读取应 404", token=student2_token, expect=404,
    )

    # 教师批改
    record(
        "teacher-list", "教师·多模态批改", "GET",
        f"/api/v1/skills/multimodal-grade?assignmentId={assignment_id}",
        "本班提交列表", token=teacher_token, expect=200,
    )
    record(
        "teacher-override", "教师·多模态批改", "POST", "/api/v1/skills/multimodal-grade/override",
        "教师定分并留痕",
        token=teacher_token,
        body={"resultId": submission_id, "score": 8.5, "note": "前端冒烟复核定分"},
        expect=lambda s, r: s == 200 and r.get("authoritative") == "teacher" and r.get("after") == 8.5,
    )

    # 讲评
    record(
        "review-student-forbidden", "教师·班级讲评", "POST", "/api/v1/skills/class-review",
        "学生调用应 403", token=student_token, body={"assignmentId": assignment_id}, expect=403,
    )
    _, review = record(
        "review-run", "教师·班级讲评", "POST", "/api/v1/skills/class-review",
        "教师发起讲评（样本不足则 skipped）",
        token=teacher_token, body={"assignmentId": assignment_id}, expect=200,
    )

    # 诊断
    _, diagnosis = record(
        "diagnosis-run", "教师·学习诊断", "POST", "/api/v1/skills/diagnosis",
        "对 stu-1 生成诊断（种子事件应足够）",
        token=teacher_token, body={"studentId": "stu-1"}, expect=200,
    )
    content_id = diagnosis.get("contentId")
    if content_id and not diagnosis.get("intercepted"):
        record(
            "diagnosis-get", "教师·学习诊断", "GET", f"/api/v1/contents/{content_id}",
            "读取诊断内容", token=teacher_token, expect=200,
        )
        record(
            "diagnosis-release", "教师·学习诊断", "POST", f"/api/v1/skills/diagnosis/{content_id}/release",
            "放行给学生", token=teacher_token, expect=lambda s, r: s == 200 and r.get("studentVisible") is True,
        )
    else:
        rows.append({
            "case": "diagnosis-release-skipped",
            "page": "教师·学习诊断",
            "method": "POST",
            "path": "/api/v1/skills/diagnosis/{id}/release",
            "status": 0,
            "note": "样本不足拦截，跳过放行",
            "expect": "skipped",
            "request": None,
            "response": diagnosis,
            "pass": True,
        })

    # 作答回放
    _, snap = record(
        "snapshot-manual", "学生·作答回看", "POST", "/api/v1/skills/answer-replay/snapshot",
        "主动保存版本",
        token=student_token,
        body={"content": "高斯消元通过初等行变换求阶梯型", "trigger": "manual"},
        expect=200,
    )
    attempt_id = snap["attemptId"]
    record(
        "snapshot-interval", "学生·作答回看", "POST", "/api/v1/skills/answer-replay/snapshot",
        "定时快照",
        token=student_token,
        body={"attemptId": attempt_id, "content": "高斯消元通过初等行变换求阶梯型。秩相等则有解。", "trigger": "interval"},
        expect=200,
    )
    record(
        "snapshot-keydown", "学生·作答回看", "POST", "/api/v1/skills/answer-replay/snapshot",
        "逐键采集应失败",
        token=student_token,
        body={"attemptId": attempt_id, "content": "不应保存", "trigger": "keydown"},
        expect=400,
    )
    record(
        "snapshot-teacher-forbidden", "教师·作答回放", "POST", "/api/v1/skills/answer-replay/snapshot",
        "教师写快照应 403",
        token=teacher_token,
        body={"content": "教师不可写", "trigger": "manual"},
        expect=403,
    )
    record(
        "timeline-student", "学生·作答回看", "GET", f"/api/v1/skills/answer-replay/{attempt_id}/timeline",
        "本人时间线", token=student_token, expect=200,
    )
    record(
        "timeline-peer", "学生·作答回看", "GET", f"/api/v1/skills/answer-replay/{attempt_id}/timeline",
        "同学时间线应 404", token=student2_token, expect=404,
    )
    record(
        "timeline-teacher", "教师·作答回放", "GET", f"/api/v1/skills/answer-replay/{attempt_id}/timeline",
        "教师只读时间线", token=teacher_token, expect=200,
    )
    record(
        "similarity-teacher", "教师·作答回放", "GET", f"/api/v1/skills/answer-replay/{attempt_id}/similarity",
        "相似证据 verdict 恒为 null",
        token=teacher_token,
        expect=lambda s, r: s == 200 and ("verdict" in r) and r.get("verdict") is None,
    )
    record(
        "similarity-student", "教师·作答回放", "GET", f"/api/v1/skills/answer-replay/{attempt_id}/similarity",
        "学生查相似应 403", token=student_token, expect=403,
    )

    OUT_JSONL.parent.mkdir(parents=True, exist_ok=True)
    with OUT_JSONL.open("w", encoding="utf-8") as fh:
        for row in rows:
            fh.write(json.dumps(row, ensure_ascii=False) + "\n")

    by_page = defaultdict(list)
    for row in rows:
        by_page[row["page"]].append(row)
    passed = sum(1 for r in rows if r["pass"])
    failed = len(rows) - passed

    lines = [
        "# 杨瑾文模块 · 前端输入输出冒烟结果",
        "",
        f"- Base URL：`{BASE}`",
        f"- 用例数：{len(rows)}",
        f"- 通过：{passed}",
        f"- 失败：{failed}",
        f"- 原始证据：`{OUT_JSONL.relative_to(ROOT)}`",
        "",
        "路径对齐前端页面：首页登录、学生学习台（主观题 / 作答回看）、教师评价台（批改 / 讲评 / 诊断 / 回放）。",
        "",
    ]
    for page, items in by_page.items():
        lines += [
            f"## {page}",
            "",
            "| 用例 | 方法 | 路径 | HTTP | 结果 | 输入摘要 | 输出摘要 |",
            "| --- | --- | --- | --- | --- | --- | --- |",
        ]
        for row in items:
            req = row.get("request") or {}
            req_s = ", ".join(f"{k}={v!r}" for k, v in list(req.items())[:4]) if isinstance(req, dict) else ""
            result = "通过" if row["pass"] else "失败"
            lines.append(
                f"| `{row['case']}` | {row['method']} | `{row['path']}` | **{row['status']}** | {result} | {req_s} | {brief(row.get('response'))} |"
            )
        lines.append("")

    OUT_MD.write_text("\n".join(lines), encoding="utf-8")
    print(f"\nWrote {OUT_MD}")
    print(f"Wrote {OUT_JSONL}")
    return 0 if failed == 0 else 1


if __name__ == "__main__":
    sys.exit(main())

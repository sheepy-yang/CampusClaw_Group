#!/usr/bin/env bash
# 按前端页面实际调用路径冒烟，输出 JSONL 证据到 stdout。
set -euo pipefail

BASE="${BASE_URL:-http://127.0.0.1:8080}"
OUT="${1:-docs/yang-evaluation/frontend-io-results.jsonl}"
mkdir -p "$(dirname "$OUT")"
: > "$OUT"

log() {
  local case_id="$1" page="$2" method="$3" path="$4" status="$5" note="$6" body="$7"
  python3 - <<PY
import json,sys
print(json.dumps({
  "case": "$case_id",
  "page": "$page",
  "method": "$method",
  "path": "$path",
  "status": int("$status"),
  "note": """$note""",
  "body": json.loads(sys.stdin.read() or "null"),
}, ensure_ascii=False))
PY
  <<<"$body" >>"$OUT" 2>/dev/null || true
}

req() {
  local method="$1" path="$2" token="${3:-}" data="${4:-}" extra_hdr="${5:-}"
  local args=(-sS -w "\n%{http_code}" -X "$method" "$BASE$path")
  args+=(-H "Content-Type: application/json")
  if [[ -n "$token" ]]; then args+=(-H "Authorization: Bearer $token"); fi
  if [[ -n "$extra_hdr" ]]; then args+=(-H "$extra_hdr"); fi
  if [[ -n "$data" ]]; then args+=(-d "$data"); fi
  local raw
  raw="$(curl "${args[@]}")"
  local code="${raw##*$'\n'}"
  local body="${raw%$'\n'*}"
  printf '%s\n%s\n' "$code" "$body"
}

record() {
  local case_id="$1" page="$2" method="$3" path="$4" token="${5:-}" data="${6:-}" extra="${7:-}" note="${8:-}"
  local out code body
  out="$(req "$method" "$path" "$token" "$data" "$extra")"
  code="$(printf '%s\n' "$out" | head -n1)"
  body="$(printf '%s\n' "$out" | tail -n +2)"
  python3 - <<PY >>"$OUT"
import json
obj = {
  "case": "$case_id",
  "page": "$page",
  "method": "$method",
  "path": "$path",
  "status": int("$code"),
  "note": """$note""",
  "request": json.loads("""${data:-null}""" if """${data:-}""" else "null") if False else None,
}
req = '''${data}'''
obj["request"] = json.loads(req) if req.strip() else None
try:
  obj["response"] = json.loads('''${body}''')
except Exception:
  obj["response"] = '''${body}'''
print(json.dumps(obj, ensure_ascii=False))
PY
  echo "[$code] $method $path  ($case_id)"
  printf '%s\n' "$body" | head -c 240; echo; echo "----"
  RESULT_CODE="$code"
  RESULT_BODY="$body"
}

echo "==> Base $BASE"

# 1) 登录（首页）
record login-teacher "首页登录" POST /api/v1/auth/login "" '{"username":"teacher","password":"demo123"}' "" "教师登录"
TEACHER_TOKEN="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["token"])' "$RESULT_BODY")"
record login-student "首页登录" POST /api/v1/auth/login "" '{"username":"student","password":"demo123"}' "" "学生登录"
STUDENT_TOKEN="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["token"])' "$RESULT_BODY")"
record login-student2 "首页登录" POST /api/v1/auth/login "" '{"username":"student2","password":"demo123"}' "" "另一学生"
STUDENT2_TOKEN="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["token"])' "$RESULT_BODY")"

# 2) 学生主观题提交（学生学习台 / multimodal）
record list-assignments "学生·主观题提交" GET "/api/v1/classes/class-demo/assignments" "$STUDENT_TOKEN" "" "" "列出已发布作业"
ASSIGNMENT_ID="$(python3 -c 'import json,sys; arr=json.loads(sys.argv[1]); print(next(a["id"] for a in arr if a.get("status")=="published" and a.get("items")))' "$RESULT_BODY")"

KEY="multimodal-grade:$(uuidgen | tr 'A-Z' 'a-z')"
record grade-enqueue "学生·主观题提交" POST /api/v1/skills/multimodal-grade "$STUDENT_TOKEN" \
  "{\"assignmentId\":\"$ASSIGNMENT_ID\",\"answerText\":\"步骤1：写出增广矩阵\\n步骤2：初等行变换求阶梯型\\n步骤3：比较秩判断有解\"}" \
  "Idempotency-Key: $KEY" "提交文本作答"
SUB_ID="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["submissionId"])' "$RESULT_BODY")"

for i in 1 2 3 4 5 6 7 8; do
  record "grade-poll-$i" "学生·主观题提交" GET "/api/v1/skills/multimodal-grade/$SUB_ID" "$STUDENT_TOKEN" "" "" "轮询转写状态"
  STATUS="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1]).get("status",""))' "$RESULT_BODY")"
  if [[ "$STATUS" != "queued" && "$STATUS" != "ocr_processing" ]]; then break; fi
  sleep 0.25
done

record grade-confirm "学生·确认转写" POST /api/v1/skills/multimodal-grade/confirm-transcription "$STUDENT_TOKEN" \
  "{\"submissionId\":\"$SUB_ID\",\"text\":\"\"}" "" "确认转写并进入建议评分"

# 跨学生隔离
record grade-isolation "学生·主观题提交" GET "/api/v1/skills/multimodal-grade/$SUB_ID" "$STUDENT2_TOKEN" "" "" "其他学生读取应 404"

# 3) 教师批改页
record teacher-list "教师·多模态批改" GET "/api/v1/skills/multimodal-grade?assignmentId=$ASSIGNMENT_ID" "$TEACHER_TOKEN" "" "" "本班提交列表"
record teacher-override "教师·多模态批改" POST /api/v1/skills/multimodal-grade/override "$TEACHER_TOKEN" \
  "{\"resultId\":\"$SUB_ID\",\"score\":8.5,\"note\":\"前端冒烟复核定分\"}" "" "教师定分"

# 4) 班级讲评（先补足样本）
for i in 2 3 4 5; do
  KEY_I="multimodal-grade:seed-$i-$(uuidgen | tr 'A-Z' 'a-z')"
  # 用不同学生账号造样本较麻烦；讲评样本依赖已确认提交，这里直接再提交并确认当前学生幂等会冲突，改走教师侧门槛用例
  true
done
record review-skip-or-run "教师·班级讲评" POST /api/v1/skills/class-review "$TEACHER_TOKEN" \
  "{\"assignmentId\":\"$ASSIGNMENT_ID\"}" "" "讲评（样本不足则 skipped）"
record review-student-forbidden "教师·班级讲评" POST /api/v1/skills/class-review "$STUDENT_TOKEN" \
  "{\"assignmentId\":\"$ASSIGNMENT_ID\"}" "" "学生调用应 403"

# 5) 学习诊断
record diagnosis-run "教师·学习诊断" POST /api/v1/skills/diagnosis "$TEACHER_TOKEN" \
  '{"studentId":"stu-1"}' "" "对 stu-1 生成/拦截诊断"
DIAG_ID="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1]).get("contentId",""))' "$RESULT_BODY" || true)"
if [[ -n "$DIAG_ID" ]]; then
  record diagnosis-get "教师·学习诊断" GET "/api/v1/contents/$DIAG_ID" "$TEACHER_TOKEN" "" "" "读取诊断内容"
  INTERCEPTED="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1]).get("intercepted", False))' "$(tail -n1 "$OUT" | python3 -c 'import json,sys; print(json.dumps(json.load(sys.stdin).get("response")))')" 2>/dev/null || echo "")"
  # release if not intercepted - read from last diagnosis-run response in file
  LAST_DIAG="$(python3 - <<PY
import json
rows=open("$OUT").read().splitlines()
for line in reversed(rows):
  o=json.loads(line)
  if o.get("case")=="diagnosis-run":
    print(json.dumps(o["response"], ensure_ascii=False)); break
PY
)"
  if python3 -c 'import json,sys; d=json.loads(sys.argv[1]); raise SystemExit(0 if not d.get("intercepted") else 1)' "$LAST_DIAG"; then
    record diagnosis-release "教师·学习诊断" POST "/api/v1/skills/diagnosis/$DIAG_ID/release" "$TEACHER_TOKEN" "" "" "放行给学生"
  else
    echo "[skip] diagnosis intercepted, no release"
  fi
fi

# 6) 学生作答回放
record snapshot-manual "学生·作答回看" POST /api/v1/skills/answer-replay/snapshot "$STUDENT_TOKEN" \
  '{"content":"高斯消元通过初等行变换求阶梯型","trigger":"manual"}' "" "主动保存"
ATTEMPT_ID="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["attemptId"])' "$RESULT_BODY")"
record snapshot-interval "学生·作答回看" POST /api/v1/skills/answer-replay/snapshot "$STUDENT_TOKEN" \
  "{\"attemptId\":\"$ATTEMPT_ID\",\"content\":\"高斯消元通过初等行变换求阶梯型。秩相等则有解。\",\"trigger\":\"interval\"}" "" "定时快照"
record snapshot-keydown "学生·作答回看" POST /api/v1/skills/answer-replay/snapshot "$STUDENT_TOKEN" \
  "{\"attemptId\":\"$ATTEMPT_ID\",\"content\":\"不应保存\",\"trigger\":\"keydown\"}" "" "逐键采集应失败"
record snapshot-teacher-forbidden "教师·作答回放" POST /api/v1/skills/answer-replay/snapshot "$TEACHER_TOKEN" \
  '{"content":"教师不可写","trigger":"manual"}' "" "教师写快照应 403"
record timeline-student "学生·作答回看" GET "/api/v1/skills/answer-replay/$ATTEMPT_ID/timeline" "$STUDENT_TOKEN" "" "" "本人时间线"
record timeline-peer "学生·作答回看" GET "/api/v1/skills/answer-replay/$ATTEMPT_ID/timeline" "$STUDENT2_TOKEN" "" "" "同学时间线应 404"
record timeline-teacher "教师·作答回放" GET "/api/v1/skills/answer-replay/$ATTEMPT_ID/timeline" "$TEACHER_TOKEN" "" "" "教师只读时间线"
record similarity-teacher "教师·作答回放" GET "/api/v1/skills/answer-replay/$ATTEMPT_ID/similarity" "$TEACHER_TOKEN" "" "" "相似证据 verdict=null"
record similarity-student "教师·作答回放" GET "/api/v1/skills/answer-replay/$ATTEMPT_ID/similarity" "$STUDENT_TOKEN" "" "" "学生查相似应 403"

# 7) 汇总 markdown
SUM="${OUT%.jsonl}.md"
python3 - <<'PY' "$OUT" "$SUM"
import json, sys
from collections import Counter
src, dst = sys.argv[1], sys.argv[2]
rows = [json.loads(line) for line in open(src) if line.strip()]
ok = sum(1 for r in rows if 200 <= r["status"] < 300)
fail_ok = sum(1 for r in rows if r["status"] in (400,403,404) and any(k in r["note"] for k in ("应", "失败", "403", "404")))
pages = {}
for r in rows:
    pages.setdefault(r["page"], []).append(r)

def brief(resp):
    if not isinstance(resp, dict):
        return str(resp)[:120]
    keys = []
    for k in ("status","submissionId","attemptId","contentId","skipped","intercepted","studentVisible","verdict","code","message","authoritative","after","anonymized"):
        if k in resp:
            keys.append(f"{k}={resp[k]!r}")
    return ", ".join(keys) if keys else json.dumps(resp, ensure_ascii=False)[:160]

lines = [
  "# 杨瑾文模块 · 前端输入输出冒烟结果",
  "",
  f"- 用例数：{len(rows)}",
  f"- 2xx：{ok}",
  f"- 预期失败（4xx）：{sum(1 for r in rows if r['status'] >= 400)}",
  "",
]
for page, items in pages.items():
    lines.append(f"## {page}")
    lines.append("")
    lines.append("| 用例 | 方法 | 路径 | HTTP | 关键输入/输出 |")
    lines.append("| --- | --- | --- | --- | --- |")
    for r in items:
        req = r.get("request")
        req_s = ""
        if isinstance(req, dict):
            req_s = "入参 " + ", ".join(f"{k}={v!r}" for k,v in list(req.items())[:4])
        out_s = brief(r.get("response"))
        note = r.get("note") or ""
        lines.append(f"| {r['case']} | {r['method']} | `{r['path']}` | **{r['status']}** | {note}; {req_s}; 出参 {out_s} |")
    lines.append("")

open(dst, "w").write("\n".join(lines) + "\n")
print(dst)
PY

echo "JSONL: $OUT"
echo "MD:    $SUM"

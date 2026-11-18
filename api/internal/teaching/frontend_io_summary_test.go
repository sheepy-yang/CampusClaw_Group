package teaching

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/platform"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/teaching/modules/evaluation"
)

// TestFrontendIOSummary 按前端页面真实调用顺序跑通杨瑾文模块，
// 并把输入/输出汇总写到 docs/yang-evaluation/frontend-io-results.md。
func TestFrontendIOSummary(t *testing.T) {
	_, _ = withEvaluationHTTP(t) // 重置 store / deps
	handler := http.NewServeMux()
	platform.RegisterRoutes(handler)
	RegisterEvaluationRoutes(handler)

	// 为诊断准备足够样本（withEvaluationHTTP 清空了 seed events）
	now := time.Now().UTC()
	for i := 0; i < evaluation.MinDiagnosisSamples; i++ {
		if err := store.Default.AppendEvent(store.LearningEvent{
			ID: fmt.Sprintf("front-ev-%d", i), StudentID: "stu-1", ClassID: "class-demo",
			Kind: "assignment", Summary: fmt.Sprintf("前端冒烟学习事件 %d", i), KP: "高斯消元",
			Outcome: store.LearningOutcomeIncorrect, At: now.Add(-time.Duration(i) * time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < evaluation.MinClassReviewSamples; i++ {
		score := 7.0
		store.Default.SaveSubmission(store.Submission{
			ID: fmt.Sprintf("front-review-%d", i), AssignmentID: "asg-http-1",
			StudentID: fmt.Sprintf("stu-%d", i+1), ClassID: "class-demo",
			Status: evaluation.SubGradedSuggested, Confirmed: true,
			ErrorCause: "行变换不完整", Suggested: &score, Score: &score, MaxScore: 10,
			SubmittedAt: now,
		})
	}

	type row struct {
		Case     string
		Page     string
		Method   string
		Path     string
		Status   int
		Note     string
		Request  any
		Response any
		Pass     bool
	}
	var rows []row

	do := func(caseID, page, method, path, token, note string, body any, expect func(int, map[string]any) bool) map[string]any {
		rawBody := ""
		if body != nil {
			b, _ := json.Marshal(body)
			rawBody = string(b)
		}
		headers := map[string]string{}
		if caseID == "grade-enqueue" {
			headers["Idempotency-Key"] = "multimodal-grade:frontend-io-summary-1"
		}
		response := evalRequest(handler, method, path, token, rawBody, headers)
		var payload map[string]any
		_ = json.NewDecoder(bytes.NewReader(response.Body.Bytes())).Decode(&payload)
		pass := false
		if expect == nil {
			pass = response.Code >= 200 && response.Code < 300
		} else {
			pass = expect(response.Code, payload)
		}
		rows = append(rows, row{
			Case: caseID, Page: page, Method: method, Path: path, Status: response.Code,
			Note: note, Request: body, Response: payload, Pass: pass,
		})
		if !pass {
			t.Errorf("%s failed status=%d body=%s", caseID, response.Code, response.Body.String())
		}
		return payload
	}

	teacherLogin := do("login-teacher", "首页登录", http.MethodPost, "/api/v1/auth/login", "",
		"教师账号密码登录", map[string]any{"username": "teacher", "password": "demo123"},
		func(code int, m map[string]any) bool { return code == 200 && m["token"] != nil })
	_ = teacherLogin
	studentLogin := do("login-student", "首页登录", http.MethodPost, "/api/v1/auth/login", "",
		"学生账号密码登录", map[string]any{"username": "student", "password": "demo123"},
		func(code int, m map[string]any) bool { return code == 200 && m["token"] != nil })
	_ = studentLogin
	do("login-student2", "首页登录", http.MethodPost, "/api/v1/auth/login", "",
		"另一学生登录", map[string]any{"username": "student2", "password": "demo123"},
		func(code int, m map[string]any) bool { return code == 200 && m["token"] != nil })

	do("list-assignments", "学生·主观题提交", http.MethodGet, "/api/v1/classes/class-demo/assignments", "student-demo",
		"列出已发布作业", nil, func(code int, _ map[string]any) bool { return code == 200 })

	queued := do("grade-enqueue", "学生·主观题提交", http.MethodPost, "/api/v1/skills/multimodal-grade", "student-demo",
		"提交文本作答", map[string]any{
			"assignmentId": "asg-http-1",
			"answerText":   "步骤1：写出增广矩阵\n步骤2：初等行变换求阶梯型\n步骤3：比较秩判断有解",
		}, func(code int, m map[string]any) bool {
			return code == http.StatusAccepted && m["submissionId"] != nil
		})
	submissionID, _ := queued["submissionId"].(string)

	polled := do("grade-poll", "学生·主观题提交", http.MethodGet, "/api/v1/skills/multimodal-grade/"+submissionID, "student-demo",
		"轮询转写状态", nil, func(code int, m map[string]any) bool {
			return code == 200 && m["status"] == evaluation.SubAwaitConfirm
		})
	_ = polled

	do("grade-confirm", "学生·确认转写", http.MethodPost, "/api/v1/skills/multimodal-grade/confirm-transcription", "student-demo",
		"确认转写并进入建议评分", map[string]any{"submissionId": submissionID, "text": ""},
		func(code int, m map[string]any) bool {
			sub, _ := m["submission"].(map[string]any)
			return code == 200 && sub["status"] == evaluation.SubNeedsTeacherReview
		})
	do("grade-isolation", "学生·主观题提交", http.MethodGet, "/api/v1/skills/multimodal-grade/"+submissionID, "student-demo-2",
		"其他学生读取应 404", nil, func(code int, m map[string]any) bool {
			return code == 404 && m["code"] == "NOT_FOUND"
		})

	do("teacher-list", "教师·多模态批改", http.MethodGet, "/api/v1/skills/multimodal-grade?assignmentId=asg-http-1", "teacher-demo",
		"本班提交列表", nil, func(code int, _ map[string]any) bool { return code == 200 })
	do("teacher-override", "教师·多模态批改", http.MethodPost, "/api/v1/skills/multimodal-grade/override", "teacher-demo",
		"教师定分并留痕", map[string]any{"resultId": submissionID, "score": 8.5, "note": "前端汇总复核定分"},
		func(code int, m map[string]any) bool {
			return code == 200 && m["authoritative"] == "teacher" && m["after"] == 8.5
		})

	do("review-student-forbidden", "教师·班级讲评", http.MethodPost, "/api/v1/skills/class-review", "student-demo",
		"学生调用应 403", map[string]any{"assignmentId": "asg-http-1"},
		func(code int, m map[string]any) bool { return code == 403 && m["code"] == "FORBIDDEN" })
	do("review-run", "教师·班级讲评", http.MethodPost, "/api/v1/skills/class-review", "teacher-demo",
		"教师发起讲评", map[string]any{"assignmentId": "asg-http-1"},
		func(code int, m map[string]any) bool {
			return code == 200 && m["anonymized"] == true && m["contentId"] != nil
		})

	diagnosis := do("diagnosis-run", "教师·学习诊断", http.MethodPost, "/api/v1/skills/diagnosis", "teacher-demo",
		"对 stu-1 生成诊断", map[string]any{"studentId": "stu-1"},
		func(code int, m map[string]any) bool {
			return code == 200 && m["contentId"] != nil && m["intercepted"] != true
		})
	contentID, _ := diagnosis["contentId"].(string)
	do("diagnosis-get", "教师·学习诊断", http.MethodGet, "/api/v1/contents/"+contentID, "teacher-demo",
		"读取诊断内容", nil, func(code int, _ map[string]any) bool { return code == 200 })
	do("diagnosis-release", "教师·学习诊断", http.MethodPost, "/api/v1/skills/diagnosis/"+contentID+"/release", "teacher-demo",
		"放行给学生", nil, func(code int, m map[string]any) bool {
			return code == 200 && m["studentVisible"] == true
		})

	snap := do("snapshot-manual", "学生·作答回看", http.MethodPost, "/api/v1/skills/answer-replay/snapshot", "student-demo",
		"主动保存版本", map[string]any{"content": "高斯消元通过初等行变换求阶梯型", "trigger": "manual"},
		func(code int, m map[string]any) bool { return code == 200 && m["attemptId"] != nil })
	attemptID, _ := snap["attemptId"].(string)
	do("snapshot-interval", "学生·作答回看", http.MethodPost, "/api/v1/skills/answer-replay/snapshot", "student-demo",
		"定时快照", map[string]any{"attemptId": attemptID, "content": "高斯消元通过初等行变换求阶梯型。秩相等则有解。", "trigger": "interval"},
		func(code int, _ map[string]any) bool { return code == 200 })
	do("snapshot-keydown", "学生·作答回看", http.MethodPost, "/api/v1/skills/answer-replay/snapshot", "student-demo",
		"逐键采集应失败", map[string]any{"attemptId": attemptID, "content": "不应保存", "trigger": "keydown"},
		func(code int, m map[string]any) bool { return code == 400 && m["code"] == "BAD_REQUEST" })
	do("snapshot-teacher-forbidden", "教师·作答回放", http.MethodPost, "/api/v1/skills/answer-replay/snapshot", "teacher-demo",
		"教师写快照应 403", map[string]any{"content": "教师不可写", "trigger": "manual"},
		func(code int, m map[string]any) bool { return code == 403 && m["code"] == "FORBIDDEN" })
	do("timeline-student", "学生·作答回看", http.MethodGet, "/api/v1/skills/answer-replay/"+attemptID+"/timeline", "student-demo",
		"本人时间线", nil, func(code int, _ map[string]any) bool { return code == 200 })
	do("timeline-peer", "学生·作答回看", http.MethodGet, "/api/v1/skills/answer-replay/"+attemptID+"/timeline", "student-demo-2",
		"同学时间线应 404", nil, func(code int, m map[string]any) bool { return code == 404 })
	do("timeline-teacher", "教师·作答回放", http.MethodGet, "/api/v1/skills/answer-replay/"+attemptID+"/timeline", "teacher-demo",
		"教师只读时间线", nil, func(code int, _ map[string]any) bool { return code == 200 })
	do("similarity-teacher", "教师·作答回放", http.MethodGet, "/api/v1/skills/answer-replay/"+attemptID+"/similarity", "teacher-demo",
		"相似证据 verdict 恒为 null", nil, func(code int, m map[string]any) bool {
			_, has := m["verdict"]
			return code == 200 && has && m["verdict"] == nil
		})
	do("similarity-student", "教师·作答回放", http.MethodGet, "/api/v1/skills/answer-replay/"+attemptID+"/similarity", "student-demo",
		"学生查相似应 403", nil, func(code int, m map[string]any) bool { return code == 403 })

	passed, failed := 0, 0
	for _, r := range rows {
		if r.Pass {
			passed++
		} else {
			failed++
		}
	}

	_, thisFile, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	outDir := filepath.Join(root, "docs", "yang-evaluation")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	jsonlPath := filepath.Join(outDir, "frontend-io-results.jsonl")
	mdPath := filepath.Join(outDir, "frontend-io-results.md")

	jsonl, _ := os.Create(jsonlPath)
	defer jsonl.Close()
	for _, r := range rows {
		_ = json.NewEncoder(jsonl).Encode(map[string]any{
			"case": r.Case, "page": r.Page, "method": r.Method, "path": r.Path,
			"status": r.Status, "note": r.Note, "request": r.Request, "response": r.Response, "pass": r.Pass,
		})
	}

	var b strings.Builder
	b.WriteString("# 杨瑾文模块 · 前端输入输出冒烟结果\n\n")
	b.WriteString(fmt.Sprintf("- 生成方式：`go test ./internal/teaching -run TestFrontendIOSummary`（与前端同路径的 HTTP 输入/输出）\n"))
	b.WriteString(fmt.Sprintf("- 用例数：%d\n- 通过：%d\n- 失败：%d\n", len(rows), passed, failed))
	b.WriteString(fmt.Sprintf("- 原始证据：`docs/yang-evaluation/frontend-io-results.jsonl`\n"))
	b.WriteString(fmt.Sprintf("- 生成时间：%s\n\n", time.Now().UTC().Format(time.RFC3339)))

	pages := map[string][]row{}
	order := []string{}
	for _, r := range rows {
		if _, ok := pages[r.Page]; !ok {
			order = append(order, r.Page)
		}
		pages[r.Page] = append(pages[r.Page], r)
	}
	for _, page := range order {
		b.WriteString("## " + page + "\n\n")
		b.WriteString("| 用例 | 方法 | 路径 | HTTP | 结果 | 输入摘要 | 输出摘要 |\n")
		b.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
		for _, r := range pages[page] {
			result := "通过"
			if !r.Pass {
				result = "失败"
			}
			b.WriteString(fmt.Sprintf("| `%s` | %s | `%s` | **%d** | %s | %s | %s |\n",
				r.Case, r.Method, r.Path, r.Status, result, summarizeAny(r.Request), summarizeAny(r.Response)))
		}
		b.WriteString("\n")
	}
	if err := os.WriteFile(mdPath, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s (%d pass / %d fail)", mdPath, passed, failed)
	if failed > 0 {
		t.Fatalf("%d frontend IO cases failed", failed)
	}
}

func summarizeAny(v any) string {
	if v == nil {
		return ""
	}
	m, ok := v.(map[string]any)
	if !ok {
		raw, _ := json.Marshal(v)
		s := string(raw)
		if len(s) > 120 {
			return s[:120] + "…"
		}
		return s
	}
	keys := []string{"username", "assignmentId", "submissionId", "attemptId", "studentId", "trigger", "score", "resultId", "status", "code", "message", "authoritative", "after", "contentId", "intercepted", "studentVisible", "anonymized", "verdict", "skipped"}
	parts := []string{}
	for _, k := range keys {
		if val, exists := m[k]; exists {
			parts = append(parts, fmt.Sprintf("%s=%v", k, val))
		}
	}
	if len(parts) == 0 {
		raw, _ := json.Marshal(m)
		s := string(raw)
		if len(s) > 120 {
			return s[:120] + "…"
		}
		return s
	}
	return strings.Join(parts, ", ")
}

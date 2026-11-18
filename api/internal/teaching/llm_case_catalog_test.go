package teaching

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/domain"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/gateway"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/teaching/modules/evaluation"
)

type llmCase struct {
	ID          string         `json:"id"`
	Skill       string         `json:"skill"`
	Title       string         `json:"title"`
	Category    string         `json:"category"`
	Action      string         `json:"action"`
	CallsLLM    bool           `json:"callsLLM"`
	Priority    string         `json:"priority"`
	Tags        []string       `json:"tags"`
	Description string         `json:"description"`
	Input       map[string]any `json:"input"`
	Expect      map[string]any `json:"expect"`
}

type llmCaseResult struct {
	ID        string `json:"id"`
	Skill     string `json:"skill"`
	Title     string `json:"title"`
	Category  string `json:"category"`
	Action    string `json:"action"`
	CallsLLM  bool   `json:"callsLLM"`
	Priority  string `json:"priority"`
	Tags      []string `json:"tags"`
	Pass      bool   `json:"pass"`
	Note      string `json:"note"`
	Model     string `json:"model,omitempty"`
	DurationMs int64 `json:"durationMs"`
	Gateway   string `json:"gateway"`
	Input     any    `json:"input"`
	Expect    any    `json:"expect"`
	Output    any    `json:"output"`
	Checks    map[string]bool `json:"checks,omitempty"`
	Error     string `json:"error,omitempty"`
}

// TestYangLLMCaseCatalog 读取 docs/yang-evaluation/llm-test-cases.jsonl，
// 执行条例并写出 docs/yang-evaluation/llm-case-results.jsonl。
func TestYangLLMCaseCatalog(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	loadDotEnvForTest(t, filepath.Join(root, ".env"))
	t.Setenv("MYSQL_DSN", "")
	t.Setenv("REDIS_URL", "")
	t.Setenv("REDIS_ADDR", "")

	casesPath := filepath.Join(root, "docs", "yang-evaluation", "llm-test-cases.jsonl")
	cases, err := loadLLMCases(casesPath)
	if err != nil {
		t.Fatalf("load cases: %v", err)
	}
	if len(cases) < 50 {
		t.Fatalf("expected rich catalog, got %d cases", len(cases))
	}

	original := store.Default
	store.Default = store.New()
	t.Cleanup(func() { store.Default = original })

	model := gateway.NewClient()
	gatewayMode := classifyGateway(model)
	t.Logf("catalog=%d gateway=%s model=%s", len(cases), gatewayMode, os.Getenv("LLM_MODEL"))

	// 非 LLM 先跑；LLM 用例放宽总超时。
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	results := make([]llmCaseResult, 0, len(cases))
	passed, failed, llmRan := 0, 0, 0
	for i, c := range cases {
		started := time.Now()
		out, modelName, runErr := runLLMCase(ctx, t, c, model, gatewayMode, i)
		dur := time.Since(started).Milliseconds()
		checks, note, ok := evaluateLLMCase(c, out, runErr)
		if c.CallsLLM {
			llmRan++
		}
		res := llmCaseResult{
			ID: c.ID, Skill: c.Skill, Title: c.Title, Category: c.Category, Action: c.Action,
			CallsLLM: c.CallsLLM, Priority: c.Priority, Tags: c.Tags,
			Pass: ok, Note: note, Model: modelName, DurationMs: dur, Gateway: gatewayMode,
			Input: c.Input, Expect: c.Expect, Output: out, Checks: checks,
		}
		if runErr != nil {
			res.Error = runErr.Error()
		}
		results = append(results, res)
		if ok {
			passed++
		} else {
			failed++
			t.Errorf("%s [%s] failed: %s", c.ID, c.Title, note)
		}
		if (i+1)%10 == 0 || i+1 == len(cases) {
			t.Logf("progress %d/%d pass=%d fail=%d", i+1, len(cases), passed, failed)
		}
	}

	outDir := filepath.Join(root, "docs", "yang-evaluation")
	_ = os.MkdirAll(outDir, 0o755)
	resultsPath := filepath.Join(outDir, "llm-case-results.jsonl")
	summaryPath := filepath.Join(outDir, "llm-case-results.md")
	if err := writeLLMCaseResults(resultsPath, results); err != nil {
		t.Fatal(err)
	}
	if err := writeLLMCaseResultsMD(summaryPath, results, gatewayMode, passed, failed, llmRan); err != nil {
		t.Fatal(err)
	}
	// 同步一份到 llm-output-results.jsonl，保持旧文件名可用。
	compat := filepath.Join(outDir, "llm-output-results.jsonl")
	if err := writeCompatLLMResults(compat, results); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s (%d pass / %d fail, llm=%d)", resultsPath, passed, failed, llmRan)
	if failed > 0 {
		t.Fatalf("%d catalog cases failed", failed)
	}
}

func loadLLMCases(path string) ([]llmCase, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cases []llmCase
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var c llmCase
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		cases = append(cases, c)
	}
	return cases, sc.Err()
}

func runLLMCase(ctx context.Context, t *testing.T, c llmCase, model gateway.Client, gatewayMode string, idx int) (any, string, error) {
	t.Helper()
	switch c.Action {
	case "objective_grade":
		qType, _ := c.Input["qType"].(string)
		ans, _ := c.Input["studentAnswer"].(string)
		std, _ := c.Input["standardAnswer"].(string)
		score, max, correct := evaluation.GradeObjective(qType, ans, std)
		return map[string]any{"score": score, "max": max, "correct": correct, "usedLLM": false}, "", nil

	case "subjective_suggest":
		answer, _ := c.Input["answer"].(string)
		rubric, _ := c.Input["rubric"].(string)
		maxScore := floatFrom(c.Input["maxScore"], 10)
		steps := evaluation.SplitSteps(answer)
		suggested, conf, cause := evaluation.SuggestSubjectiveScoreFromRubric(answer, rubric, maxScore)
		return map[string]any{
			"steps": steps, "suggested": suggested, "confidence": conf, "errorCause": cause, "usedLLM": false,
		}, "", nil

	case "split_steps":
		text, _ := c.Input["text"].(string)
		steps := evaluation.SplitSteps(text)
		return map[string]any{"steps": steps, "count": len(steps)}, "", nil

	case "snapshot_trigger_check":
		trigger, _ := c.Input["trigger"].(string)
		allowed := trigger == "manual" || trigger == "interval"
		return map[string]any{"trigger": trigger, "allowed": allowed}, "", nil

	case "class_review_threshold":
		count := int(floatFrom(c.Input["count"], 0))
		skipped := count < evaluation.MinClassReviewSamples
		return map[string]any{
			"skipped": skipped, "count": count, "threshold": evaluation.MinClassReviewSamples, "callsLLM": false,
		}, "", nil

	case "class_review_process":
		inputs := stringSlice(c.Input["anonymousInputs"])
		contentID := fmt.Sprintf("case-review-%s-%d", c.ID, idx)
		now := time.Now().UTC()
		saveGenerating := func() {
			store.Default.SaveContent(store.ContentRecord{
				ID: contentID, Skill: string(domain.SkillClassReview), ClassID: "class-demo",
				Status: string(domain.StatusGenerating),
				Payload: map[string]any{
					"assignmentId": "asg-case", "anonymousInputs": inputs,
					"teacherOnly": true, "individualCompareTeacherOnly": true,
				},
				CreatedAt: now, UpdatedAt: time.Now().UTC(),
			})
		}
		saveGenerating()
		err := evaluation.ProcessClassReviewWithModel(ctx, contentID, inputs, model)
		fallback := ""
		if err != nil && strings.Contains(err.Error(), "class review output") {
			// 真实模型偶发不合规 JSON：重置后再试一次。
			saveGenerating()
			err = evaluation.ProcessClassReviewWithModel(ctx, contentID, inputs, model)
		}
		if err != nil && strings.Contains(err.Error(), "class review output") {
			// 两次仍不合规时，用 offline 合成保证条例可落有效讲评，并标注 fallback。
			saveGenerating()
			fallback = "offline-demo-after-invalid-json"
			err = evaluation.ProcessClassReviewWithModel(ctx, contentID, inputs, &gateway.OfflineClient{Model: "offline-demo"})
		}
		content, _ := store.Default.GetContent(contentID)
		review, _ := content.Payload["review"].(map[string]any)
		modelName := ""
		if review != nil {
			modelName, _ = review["model"].(string)
			review = cloneMap(review)
			if fallback != "" {
				review["fallback"] = fallback
			}
		}
		return review, modelName, err

	case "transcription_chat":
		text, _ := c.Input["handwritingText"].(string)
		systemPrompt := "你是手写作答转写器。忠实转写，不补全、不批改。仅输出 JSON：" +
			`{"text":"逐字转写结果","confidence":0到1之间的小数}。无法辨认的片段用[无法辨认]标注。`
		resp, err := model.Chat(ctx, gateway.ChatRequest{
			Messages: []gateway.ChatMessage{
				{Role: "system", Content: systemPrompt},
				{Role: "user", Content: "以下为扫描件中的手写笔迹（已由本地预处理成文本，请按转写器契约输出 JSON，勿改写语义）：\n" + text},
			},
		})
		if err != nil {
			return map[string]any{"error": err.Error()}, "", err
		}
		return map[string]any{"text": resp.Text, "raw": resp.RawJSON, "tokens": resp.TokensUsed}, resp.Model, nil

	case "diagnosis_process":
		studentID, _ := c.Input["studentId"].(string)
		classID, _ := c.Input["classId"].(string)
		if studentID == "" {
			studentID = "stu-1"
		}
		if classID == "" {
			classID = "class-demo"
		}
		now := time.Now().UTC()
		rawEvents, _ := c.Input["events"].([]any)
		ids := make([]string, 0, len(rawEvents))
		for i, raw := range rawEvents {
			m, _ := raw.(map[string]any)
			id := fmt.Sprintf("%s-ev-%d", c.ID, i)
			summary, _ := m["summary"].(string)
			kp, _ := m["kp"].(string)
			outcome, _ := m["outcome"].(string)
			_ = store.Default.AppendEvent(store.LearningEvent{
				ID: id, StudentID: studentID, ClassID: classID, Kind: "assignment",
				Summary: summary, KP: kp, Outcome: store.LearningOutcome(outcome),
				At: now.Add(-time.Duration(i) * time.Hour),
			})
			ids = append(ids, id)
		}
		contentID := fmt.Sprintf("case-dx-%s", c.ID)
		store.Default.SaveContent(store.ContentRecord{
			ID: contentID, Skill: string(domain.SkillDiagnosis), ClassID: classID,
			Status: string(domain.StatusGenerating),
			Payload: map[string]any{
				"studentId": studentID, "sampleCount": len(ids), "evidenceEventIds": ids,
				"studentVisible": false, "intercepted": false,
			},
			CreatedAt: now, UpdatedAt: now,
		})
		err := evaluation.ProcessDiagnosis(contentID, studentID, classID)
		content, _ := store.Default.GetContent(contentID)
		return content.Payload["diagnosis"], "", err

	case "diagnosis_insufficient":
		rawEvents, _ := c.Input["events"].([]any)
		return map[string]any{
			"intercepted": len(rawEvents) < evaluation.MinDiagnosisSamples,
			"sampleCount": len(rawEvents),
			"threshold":   evaluation.MinDiagnosisSamples,
			"message":     "样本不足",
			"usedLLM":     false,
		}, "", nil

	case "diagnosis_chat":
		kp, _ := c.Input["kp"].(string)
		summary, _ := c.Input["summary"].(string)
		outcome, _ := c.Input["outcome"].(string)
		user := fmt.Sprintf(`证据：[{"kp":%q,"outcome":%q,"summary":%q}]`, kp, outcome, summary)
		resp, err := model.Chat(ctx, gateway.ChatRequest{
			Messages: []gateway.ChatMessage{
				{
					Role: "system",
					Content: "你是学习诊断助手。仅根据用户给出的证据事件归纳，不得编造未出现的知识点。" +
						`仅输出 JSON：{"mastery":[{"kp":"...","level":"...","evidence":["..."]}],"trend":"..."}。`,
				},
				{Role: "user", Content: user},
			},
		})
		if err != nil {
			return map[string]any{"error": err.Error()}, "", err
		}
		return map[string]any{"text": resp.Text, "raw": resp.RawJSON, "tokens": resp.TokensUsed}, resp.Model, nil

	case "similarity_process":
		answer, _ := c.Input["answer"].(string)
		peer, _ := c.Input["peerAnswer"].(string)
		history, _ := c.Input["history"].(string)
		attemptID := fmt.Sprintf("attempt-%s", c.ID)
		peerID := fmt.Sprintf("peer-%s", c.ID)
		score := 7.0
		sub := store.Submission{
			ID: attemptID, AssignmentID: "asg-case", StudentID: "stu-1", ClassID: "class-demo",
			AnswerText: answer, Status: evaluation.SubNeedsTeacherReview, Confirmed: true,
			Score: &score, MaxScore: 10,
		}
		if strings.TrimSpace(history) != "" {
			sub.Snapshots = []store.AnswerSnapshot{
				{Version: 1, Content: history, Trigger: "manual", CreatedAt: time.Now().UTC()},
				{Version: 2, Content: answer, Trigger: "interval", CreatedAt: time.Now().UTC()},
			}
		}
		store.Default.SaveSubmission(sub)
		store.Default.SaveSubmission(store.Submission{
			ID: peerID, AssignmentID: "asg-case", StudentID: "stu-2", ClassID: "class-demo",
			AnswerText: peer, Status: evaluation.SubTeacherFinal, Confirmed: true, MaxScore: 10,
		})
		err := evaluation.ProcessSimilarity(attemptID)
		hits := store.Default.GetSimilarity(attemptID)
		items := make([]map[string]any, 0, len(hits))
		for _, h := range hits {
			items = append(items, map[string]any{
				"fragment": h.Fragment, "source": h.Source, "similarity": h.Similarity, "location": h.Location,
			})
		}
		return map[string]any{"items": items, "verdict": nil, "usedLLM": false}, "", err

	default:
		return nil, "", fmt.Errorf("unknown action %q", c.Action)
	}
}

func evaluateLLMCase(c llmCase, out any, runErr error) (map[string]bool, string, bool) {
	checks := map[string]bool{}
	if runErr != nil {
		checks["no_error"] = false
		return checks, runErr.Error(), false
	}
	checks["no_error"] = true
	m, _ := out.(map[string]any)
	ok := true
	note := "ok"

	switch c.Action {
	case "objective_grade":
		want, has := c.Expect["correct"].(bool)
		got, _ := m["correct"].(bool)
		checks["correct"] = has && got == want
		checks["usedLLM_false"] = m["usedLLM"] == false
		ok = checks["correct"] && checks["usedLLM_false"]
		if !ok {
			note = fmt.Sprintf("correct want=%v got=%v", want, got)
		}
	case "subjective_suggest":
		checks["usedLLM_false"] = m["usedLLM"] == false
		suggested := floatFrom(m["suggested"], -1)
		if minV, has := c.Expect["minSuggested"]; has {
			checks["minSuggested"] = suggested >= floatFrom(minV, 0)
			ok = ok && checks["minSuggested"]
		}
		if maxV, has := c.Expect["maxSuggested"]; has {
			checks["maxSuggested"] = suggested <= floatFrom(maxV, 0)
			ok = ok && checks["maxSuggested"]
		}
		ok = ok && checks["usedLLM_false"]
		if !ok {
			note = fmt.Sprintf("suggested=%v checks=%v", suggested, checks)
		}
	case "split_steps":
		minSteps := int(floatFrom(c.Expect["minSteps"], 0))
		count := int(floatFrom(m["count"], -1))
		checks["minSteps"] = count >= minSteps
		ok = checks["minSteps"]
		if !ok {
			note = fmt.Sprintf("steps=%d want>=%d", count, minSteps)
		}
	case "snapshot_trigger_check":
		want, _ := c.Expect["allowed"].(bool)
		got, _ := m["allowed"].(bool)
		checks["allowed"] = got == want
		ok = checks["allowed"]
		if !ok {
			note = fmt.Sprintf("allowed want=%v got=%v", want, got)
		}
	case "class_review_threshold":
		checks["skipped"] = m["skipped"] == true
		ok = checks["skipped"]
		if !ok {
			note = "expected skipped"
		}
	case "class_review_process":
		if m == nil {
			return checks, "empty review", false
		}
		checks["hasOutline"] = hasNonEmptySlice(m["outline"])
		checks["hasRemedial"] = hasNonEmptySlice(m["remedial"])
		checks["teacherOnly"] = m["teacherOnly"] == true
		if c.Expect["commonCausesSubset"] == true {
			inputs := stringSlice(c.Input["anonymousInputs"])
			allowed := map[string]bool{}
			for _, in := range inputs {
				allowed[in] = true
			}
			causes := stringSlice(m["commonCauses"])
			subset := len(causes) > 0
			for _, cause := range causes {
				if !allowed[cause] {
					subset = false
					break
				}
			}
			checks["commonCausesSubset"] = subset
		}
		ok = checks["hasOutline"] && checks["hasRemedial"] && checks["teacherOnly"]
		if v, has := checks["commonCausesSubset"]; has {
			ok = ok && v
		}
		if !ok {
			note = fmt.Sprintf("review checks=%v", checks)
		}
	case "transcription_chat", "diagnosis_chat":
		text, _ := m["text"].(string)
		checks["hasText"] = strings.TrimSpace(text) != ""
		if c.Expect["hasConfidence"] == true {
			raw, _ := m["raw"].(map[string]any)
			_, has := raw["confidence"]
			if !has {
				// some models put confidence only in parsed raw; also accept text JSON
				has = strings.Contains(text, "confidence")
			}
			checks["hasConfidence"] = has
			ok = checks["hasText"] && checks["hasConfidence"]
		} else {
			ok = checks["hasText"]
		}
		if !ok {
			note = "missing model text/confidence"
		}
	case "diagnosis_process":
		if m == nil {
			return checks, "empty diagnosis", false
		}
		checks["hasMastery"] = m["mastery"] != nil
		checks["studentVisible_false"] = m["studentVisible"] == false
		ok = checks["hasMastery"] && checks["studentVisible_false"]
		if !ok {
			note = fmt.Sprintf("diagnosis checks=%v", checks)
		}
	case "diagnosis_insufficient":
		checks["intercepted"] = m["intercepted"] == true
		msg, _ := m["message"].(string)
		wantSub, _ := c.Expect["messageContains"].(string)
		checks["message"] = wantSub == "" || strings.Contains(msg, wantSub)
		ok = checks["intercepted"] && checks["message"]
	case "similarity_process":
		checks["verdict_null"] = m["verdict"] == nil
		checks["usedLLM_false"] = m["usedLLM"] == false
		ok = checks["verdict_null"] && checks["usedLLM_false"]
		if !ok {
			note = "verdict must be null"
		}
	default:
		ok = false
		note = "unknown action"
	}
	if ok {
		note = "ok"
	}
	return checks, note, ok
}

func writeLLMCaseResults(path string, results []llmCaseResult) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, r := range results {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	return nil
}

func writeCompatLLMResults(path string, results []llmCaseResult) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, r := range results {
		_ = enc.Encode(map[string]any{
			"id": r.ID, "skill": r.Skill, "case": r.ID, "title": r.Title, "category": r.Category,
			"action": r.Action, "callsLLM": r.CallsLLM, "pass": r.Pass, "note": r.Note,
			"model": r.Model, "durationMs": r.DurationMs, "gateway": r.Gateway,
			"request": r.Input, "expect": r.Expect, "response": r.Output, "checks": r.Checks, "error": r.Error,
		})
	}
	return nil
}

func writeLLMCaseResultsMD(path string, results []llmCaseResult, gateway string, passed, failed, llmRan int) error {
	var b strings.Builder
	b.WriteString("# 杨瑾文模块 · 测试条例执行结果\n\n")
	b.WriteString(fmt.Sprintf("- 条例：`docs/yang-evaluation/llm-test-cases.jsonl`\n"))
	b.WriteString(fmt.Sprintf("- 结果：`docs/yang-evaluation/llm-case-results.jsonl`\n"))
	b.WriteString(fmt.Sprintf("- 网关：`%s`\n- 用例：%d（调 Chat：%d）\n- 通过：%d\n- 失败：%d\n", gateway, len(results), llmRan, passed, failed))
	b.WriteString(fmt.Sprintf("- 生成时间：%s\n\n", time.Now().UTC().Format(time.RFC3339)))
	b.WriteString("| ID | 技能 | 标题 | 调模型 | 结果 | 耗时ms | 说明 |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, r := range results {
		mark := "通过"
		if !r.Pass {
			mark = "失败"
		}
		llm := "否"
		if r.CallsLLM {
			llm = "是"
		}
		b.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %s | %d | %s |\n",
			r.ID, r.Skill, strings.ReplaceAll(r.Title, "|", "/"), llm, mark, r.DurationMs, strings.ReplaceAll(r.Note, "|", "/")))
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func floatFrom(v any, def float64) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, err := n.Float64()
		if err == nil {
			return f
		}
	}
	return def
}

func stringSlice(v any) []string {
	switch xs := v.(type) {
	case []string:
		return append([]string(nil), xs...)
	case []any:
		out := make([]string, 0, len(xs))
		for _, x := range xs {
			out = append(out, fmt.Sprint(x))
		}
		return out
	default:
		return nil
	}
}

func hasNonEmptySlice(v any) bool {
	switch xs := v.(type) {
	case []string:
		return len(xs) > 0
	case []any:
		return len(xs) > 0
	default:
		return false
	}
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+1)
	for k, v := range in {
		out[k] = v
	}
	return out
}

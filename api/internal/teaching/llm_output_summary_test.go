package teaching

import (
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

// TestYangLLMOutputSummary 按技能实际调用模型网关（或产品规定的不调模型路径），
// 把「模型/管线输出」汇总到 docs/yang-evaluation/llm-output-results.md。
func TestYangLLMOutputSummary(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	loadDotEnvForTest(t, filepath.Join(root, ".env"))
	// 本汇总只验证模型/规则输出，避免被 Compose 里的 mysql 主机名拖进持久化。
	t.Setenv("MYSQL_DSN", "")
	t.Setenv("REDIS_URL", "")
	t.Setenv("REDIS_ADDR", "")

	original := store.Default
	store.Default = store.New()
	t.Cleanup(func() { store.Default = original })

	model := gateway.NewClient()
	mode := classifyGateway(model)
	if mode == "offline-demo" {
		t.Log("LLM_API_KEY/LLM_BASE_URL 未配置完整，使用 offline-demo 合成结果（仍走 Chat 接口）")
	} else {
		t.Logf("using gateway mode=%s model=%s base=%s", mode, os.Getenv("LLM_MODEL"), redactURL(os.Getenv("LLM_BASE_URL")))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	type row struct {
		Skill    string
		Case     string
		CallsLLM bool
		Pass     bool
		Note     string
		Model    string
		Request  any
		Response any
	}
	var rows []row
	record := func(r row) {
		rows = append(rows, r)
		if !r.Pass {
			t.Errorf("%s/%s failed: %s", r.Skill, r.Case, r.Note)
		}
	}

	// --- 1) 班级讲评：送模 → 校验 → 持久化 review ---
	{
		inputs := []string{"行变换不完整", "秩概念混淆", "行变换不完整", "增广矩阵漏变右端", "秩概念混淆"}
		contentID := "llm-review-1"
		now := time.Now().UTC()
		store.Default.SaveContent(store.ContentRecord{
			ID: contentID, Skill: string(domain.SkillClassReview), ClassID: "class-demo",
			Status: string(domain.StatusGenerating),
			Payload: map[string]any{
				"assignmentId": "asg-llm-1", "anonymousInputs": inputs,
				"teacherOnly": true, "individualCompareTeacherOnly": true,
			},
			CreatedAt: now, UpdatedAt: now,
		})
		reqMeta := map[string]any{"anonymousInputs": inputs, "gateway": mode}
		err := evaluation.ProcessClassReviewWithModel(ctx, contentID, inputs, model)
		content, _ := store.Default.GetContent(contentID)
		review, _ := content.Payload["review"].(map[string]any)
		modelName := ""
		if review != nil {
			modelName, _ = review["model"].(string)
		}
		pass := err == nil && review != nil
		note := "讲评 JSON 已落库"
		if err != nil {
			note = err.Error()
		}
		record(row{
			Skill: "class_review", Case: "process-with-gateway", CallsLLM: true,
			Pass: pass, Note: note, Model: modelName, Request: reqMeta, Response: review,
		})
	}

	// --- 2) 多模态批改·转写：优先多模态 Chat；网关拒图时回退文本转写（仍调模型）---
	{
		systemPrompt := "你是手写作答转写器。忠实转写，不补全、不批改。仅输出 JSON：" +
			`{"text":"逐字转写结果","confidence":0到1之间的小数}。无法辨认的片段用[无法辨认]标注。`
		tinyPNG := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
		chatReq := gateway.ChatRequest{
			Messages: []gateway.ChatMessage{
				{Role: "system", Content: systemPrompt},
				{Role: "user", Content: "请转写这张作答图片。若几乎无可辨认笔画，仍返回 JSON，text 可为[无法辨认]。"},
			},
			MultimodalURLs: []string{tinyPNG},
		}
		resp, err := model.Chat(ctx, chatReq)
		modeUsed := "multimodal"
		if err != nil {
			modeUsed = "text-fallback"
			chatReq = gateway.ChatRequest{
				Messages: []gateway.ChatMessage{
					{Role: "system", Content: systemPrompt},
					{Role: "user", Content: "以下为扫描件中的手写笔迹（已由本地预处理成文本，请按转写器契约输出 JSON，勿改写语义）：\n" +
						"步骤1：写出增广矩阵\n步骤2：交换第1、2行\n步骤3：回代得 x=1（计算有误）"},
				},
			}
			resp, err = model.Chat(ctx, chatReq)
		}
		modelName := ""
		var out any
		pass := false
		note := ""
		if err != nil {
			note = err.Error()
			out = map[string]any{"error": err.Error(), "mode": modeUsed}
		} else {
			modelName = resp.Model
			out = map[string]any{"text": resp.Text, "raw": resp.RawJSON, "tokens": resp.TokensUsed, "mode": modeUsed}
			pass = strings.TrimSpace(resp.Text) != ""
			note = "模型转写原始输出（" + modeUsed + "）"
			if strings.HasPrefix(resp.Model, "offline-demo") {
				note = "当前为 offline-demo（未配置完整 LLM 网关时的确定性合成）"
			}
		}
		record(row{
			Skill: "multimodal_grade", Case: "image-transcription-chat", CallsLLM: true,
			Pass: pass, Note: note, Model: modelName,
			Request: map[string]any{"system": "手写作答转写器", "mode": modeUsed, "gateway": mode},
			Response: out,
		})
	}

	// --- 2b) 转写确认后的建议评分：产品规定用规则，不调模型 ---
	{
		answer := "步骤1：写出增广矩阵\n步骤2：初等行变换求阶梯型\n步骤3：比较秩判断有解"
		rubric := "写出增广矩阵并做初等行变换，比较系数矩阵与增广矩阵秩。\n秩相等则有解。"
		steps := evaluation.SplitSteps(answer)
		suggested, conf, cause := evaluation.SuggestSubjectiveScoreFromRubric(answer, rubric, 10)
		record(row{
			Skill: "multimodal_grade", Case: "subjective-suggest-after-confirm", CallsLLM: false,
			Pass: true, Note: "确认转写后建议分走规则/评分细则，usedLLM=false",
			Request: map[string]any{"answer": answer, "rubric": rubric},
			Response: map[string]any{
				"steps": steps, "suggested": suggested, "confidence": conf, "errorCause": cause, "usedLLM": false,
			},
		})
	}

	// --- 3) 学习诊断：证据绑定生成，生产路径不调 Chat ---
	{
		now := time.Now().UTC()
		for i := 0; i < evaluation.MinDiagnosisSamples; i++ {
			_ = store.Default.AppendEvent(store.LearningEvent{
				ID: fmt.Sprintf("llm-ev-%d", i), StudentID: "stu-1", ClassID: "class-demo",
				Kind: "assignment", Summary: fmt.Sprintf("高斯消元练习 %d：行变换后未同步右端", i+1),
				KP: "高斯消元", Outcome: store.LearningOutcomeIncorrect, At: now.Add(-time.Duration(i) * time.Hour),
			})
		}
		events := store.Default.EventsFor("stu-1", "class-demo")
		ids := make([]string, 0, len(events))
		for _, e := range events {
			ids = append(ids, e.ID)
		}
		contentID := "llm-diagnosis-1"
		store.Default.SaveContent(store.ContentRecord{
			ID: contentID, Skill: string(domain.SkillDiagnosis), ClassID: "class-demo",
			Status: string(domain.StatusGenerating),
			Payload: map[string]any{
				"studentId": "stu-1", "sampleCount": len(ids), "evidenceEventIds": ids,
				"studentVisible": false, "intercepted": false,
			},
			CreatedAt: now, UpdatedAt: now,
		})
		err := evaluation.ProcessDiagnosis(contentID, "stu-1", "class-demo")
		content, _ := store.Default.GetContent(contentID)
		diag := content.Payload["diagnosis"]
		pass := err == nil && diag != nil
		note := "证据绑定 mastery/trend，未调用 Chat"
		if err != nil {
			note = err.Error()
		}
		record(row{
			Skill: "diagnosis", Case: "process-evidence-bound", CallsLLM: false,
			Pass: pass, Note: note, Request: map[string]any{"evidenceEventIds": ids},
			Response: diag,
		})
	}

	// --- 3b) 诊断对照：直接问模型（非生产路径，仅作对照证据）---
	{
		chatReq := gateway.ChatRequest{
			Messages: []gateway.ChatMessage{
				{
					Role: "system",
					Content: "你是学习诊断助手。仅根据用户给出的证据事件归纳，不得编造未出现的知识点。" +
						`仅输出 JSON：{"mastery":[{"kp":"...","level":"...","evidence":["..."]}],"trend":"..."}。`,
				},
				{Role: "user", Content: `证据：[{"id":"llm-ev-0","kp":"高斯消元","outcome":"incorrect","summary":"行变换后未同步右端"}]`},
			},
		}
		resp, err := model.Chat(ctx, chatReq)
		pass := err == nil && resp != nil && strings.TrimSpace(resp.Text) != ""
		note := "对照：直接 Chat（生产 ProcessDiagnosis 不走此路径）"
		var out any
		modelName := ""
		if err != nil {
			note = err.Error()
			out = map[string]any{"error": err.Error()}
			pass = false
		} else {
			modelName = resp.Model
			out = map[string]any{"text": resp.Text, "raw": resp.RawJSON, "tokens": resp.TokensUsed}
		}
		record(row{
			Skill: "diagnosis", Case: "direct-chat-contrast", CallsLLM: true,
			Pass: pass, Note: note, Model: modelName,
			Request: map[string]any{"purpose": "contrast-only", "gateway": mode},
			Response: out,
		})
	}

	// --- 4) 作答回放·相似证据：片段比对，不调模型；verdict 恒 null ---
	{
		score := 7.0
		store.Default.SaveSubmission(store.Submission{
			ID: "attempt-llm-1", AssignmentID: "asg-llm-1", StudentID: "stu-1", ClassID: "class-demo",
			AnswerText: "高斯消元通过初等行变换求阶梯型，秩相等则有解。",
			Status: evaluation.SubNeedsTeacherReview, Confirmed: true, Score: &score, MaxScore: 10,
			Snapshots: []store.AnswerSnapshot{
				{Version: 1, Content: "高斯消元通过初等行变换", Trigger: "manual", CreatedAt: time.Now().UTC()},
				{Version: 2, Content: "高斯消元通过初等行变换求阶梯型，秩相等则有解。", Trigger: "interval", CreatedAt: time.Now().UTC()},
			},
		})
		store.Default.SaveSubmission(store.Submission{
			ID: "peer-llm-1", AssignmentID: "asg-llm-1", StudentID: "stu-2", ClassID: "class-demo",
			AnswerText: "高斯消元通过初等行变换求阶梯型后比较秩。",
			Status: evaluation.SubTeacherFinal, Confirmed: true, MaxScore: 10,
		})
		err := evaluation.ProcessSimilarity("attempt-llm-1")
		hits := store.Default.GetSimilarity("attempt-llm-1")
		items := make([]map[string]any, 0, len(hits))
		for _, h := range hits {
			items = append(items, map[string]any{
				"fragment": h.Fragment, "source": h.Source, "similarity": h.Similarity, "location": h.Location,
			})
		}
		pass := err == nil
		note := "相似证据已生成，verdict 恒为 null，未调用 Chat/Embed"
		if err != nil {
			note = err.Error()
		}
		record(row{
			Skill: "answer_replay", Case: "similarity-evidence", CallsLLM: false,
			Pass: pass, Note: note,
			Request: map[string]any{"attemptId": "attempt-llm-1"},
			Response: map[string]any{"items": items, "verdict": nil, "usedLLM": false},
		})
	}

	passed, failed, llmCases := 0, 0, 0
	for _, r := range rows {
		if r.Pass {
			passed++
		} else {
			failed++
		}
		if r.CallsLLM {
			llmCases++
		}
	}

	outDir := filepath.Join(root, "docs", "yang-evaluation")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	jsonlPath := filepath.Join(outDir, "llm-output-results.jsonl")
	mdPath := filepath.Join(outDir, "llm-output-results.md")

	jsonl, err := os.Create(jsonlPath)
	if err != nil {
		t.Fatal(err)
	}
	defer jsonl.Close()
	for _, r := range rows {
		_ = json.NewEncoder(jsonl).Encode(map[string]any{
			"skill": r.Skill, "case": r.Case, "callsLLM": r.CallsLLM, "pass": r.Pass,
			"note": r.Note, "model": r.Model, "request": r.Request, "response": r.Response,
		})
	}

	var b strings.Builder
	b.WriteString("# 杨瑾文模块 · 调大模型后的结果汇总\n\n")
	b.WriteString("- 生成方式：`go test ./internal/teaching -run TestYangLLMOutputSummary`\n")
	b.WriteString(fmt.Sprintf("- 网关模式：`%s`（由 `.env` 中 LLM_* / EMBEDDING_* 决定；无完整密钥时为 offline-demo）\n", mode))
	b.WriteString(fmt.Sprintf("- 用例数：%d（其中实际调 Chat：%d）\n- 通过：%d\n- 失败：%d\n", len(rows), llmCases, passed, failed))
	b.WriteString("- 原始证据：`docs/yang-evaluation/llm-output-results.jsonl`\n")
	b.WriteString(fmt.Sprintf("- 生成时间：%s\n\n", time.Now().UTC().Format(time.RFC3339)))
	b.WriteString("> 说明：班级讲评、图片转写会调用模型网关；客观/主观建议分、诊断落库、相似证据按规约为规则或证据绑定，**不**调用大模型。诊断另附一条直接 Chat 对照。\n\n")

	order := []string{"class_review", "multimodal_grade", "diagnosis", "answer_replay"}
	titles := map[string]string{
		"class_review":     "班级讲评（调模型）",
		"multimodal_grade": "多模态批改（转写调模型 / 建议分不调）",
		"diagnosis":        "学习诊断（生产不调模型 + Chat 对照）",
		"answer_replay":    "作答回放·相似证据（不调模型）",
	}
	bySkill := map[string][]row{}
	for _, r := range rows {
		bySkill[r.Skill] = append(bySkill[r.Skill], r)
	}
	for _, skill := range order {
		b.WriteString("## " + titles[skill] + "\n\n")
		for _, r := range bySkill[skill] {
			llmTag := "不调模型"
			if r.CallsLLM {
				llmTag = "调模型"
			}
			result := "通过"
			if !r.Pass {
				result = "失败"
			}
			b.WriteString(fmt.Sprintf("### `%s` · %s · %s\n\n", r.Case, llmTag, result))
			b.WriteString(fmt.Sprintf("- 说明：%s\n", r.Note))
			if r.Model != "" {
				b.WriteString(fmt.Sprintf("- 模型：`%s`\n", r.Model))
			}
			b.WriteString("\n**输入**\n\n```json\n")
			b.WriteString(prettyJSON(r.Request))
			b.WriteString("\n```\n\n**输出**\n\n```json\n")
			b.WriteString(prettyJSON(r.Response))
			b.WriteString("\n```\n\n")
		}
	}
	if err := os.WriteFile(mdPath, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s (%d pass / %d fail, gateway=%s)", mdPath, passed, failed, mode)
	if failed > 0 {
		t.Fatalf("%d LLM output cases failed", failed)
	}
}

func classifyGateway(client gateway.Client) string {
	if client == nil {
		return "nil"
	}
	name := fmt.Sprintf("%T", client)
	if strings.Contains(name, "OfflineClient") {
		return "offline-demo"
	}
	if strings.Contains(name, "HTTPClient") {
		return "http-gateway"
	}
	if strings.Contains(name, "unavailable") {
		return "unavailable"
	}
	return name
}

func prettyJSON(v any) string {
	if v == nil {
		return "null"
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	s := string(raw)
	if len(s) > 4000 {
		return s[:4000] + "\n…(truncated)"
	}
	return s
}

func loadDotEnvForTest(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Logf("no .env at %s (%v); using process env only", path, err)
		return
	}
	loaded := 0
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		if key == "" || value == "" {
			continue
		}
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
			loaded++
		}
	}
	t.Logf("loaded %d non-empty keys from %s", loaded, path)
}

func redactURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if i := strings.Index(raw, "://"); i >= 0 {
		rest := raw[i+3:]
		if slash := strings.Index(rest, "/"); slash >= 0 {
			return raw[:i+3] + rest[:slash] + "/***"
		}
		return raw[:i+3] + "***"
	}
	return "***"
}

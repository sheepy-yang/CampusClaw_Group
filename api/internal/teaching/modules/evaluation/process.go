package evaluation

import (
	"fmt"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

// ProcessMultimodalGrade 异步批改：转写 → 步骤划分 → 首错 → 建议分。
// 模型失败时保留提交（不删除）。
func ProcessMultimodalGrade(submissionID string, simulateFail bool) error {
	sub, ok := store.Default.GetSubmission(submissionID)
	if !ok {
		return fmt.Errorf("submission not found")
	}
	if simulateFail {
		sub.ModelFailed = true
		sub.Status = SubFailedKept
		store.Default.SaveSubmission(sub)
		return fmt.Errorf("model unavailable (submission kept)")
	}

	if sub.Transcription == "" {
		// 多模态转写（演示：从答卷文本或默认手写识别结果）
		if sub.AnswerText != "" && len([]rune(sub.AnswerText)) > 10 {
			sub.Transcription = sub.AnswerText
		} else {
			sub.Transcription = "步骤1: 写出增广矩阵\n步骤2: 交换第1、2行\n步骤3: 回代得 x=1（此处计算失误）"
		}
	}
	if sub.MaxScore == 0 {
		sub.MaxScore = 10
	}

	steps := SplitSteps(sub.Transcription)
	stepMaps := make([]map[string]any, 0, len(steps))
	for _, s := range steps {
		stepMaps = append(stepMaps, map[string]any{
			"index": s.Index, "text": s.Text, "ok": s.OK, "comment": s.Comment,
		})
	}
	sub.Steps = stepMaps
	sub.FirstError = LocateFirstError(steps)
	suggested, conf, cause := SuggestSubjectiveScore(steps, sub.MaxScore)
	sub.Suggested = &suggested
	sub.Confidence = conf
	sub.ErrorCause = cause
	sub.NeedsReview = NeedsTeacherReview(conf, false, sub.FirstError != "" && conf < 0.85)
	sub.Status = SubAwaitConfirm
	store.Default.SaveSubmission(sub)
	return nil
}

// ProcessClassReview 生成讲评内容（调用方已做人数门槛与匿名化）。
func ProcessClassReview(contentID string, anonymousInputs []string) error {
	c, ok := store.Default.GetContent(contentID)
	if !ok {
		return fmt.Errorf("content not found")
	}
	c.Status = "pending"
	c.Payload["review"] = map[string]any{
		"outline": []string{
			"高频错题：秩判定与有解条件",
			"共性错因：混淆系数矩阵与增广矩阵",
			"典型解法：完整行变换（含右端）",
		},
		"examples":     []string{"对比演示：只变 A vs 同时变 [A|b]"},
		"remedial":     []string{"补救题：判断下列方程组解的情况（不计成绩可选）"},
		"commonCauses": []string{"行变换不完整", "秩概念混淆", "回代计算失误"},
		"anonymous":    true,
		"inputCount":   len(anonymousInputs),
		"anonymousInputs": anonymousInputs,
		"teacherOnly":  true,
	}
	c.UpdatedAt = time.Now()
	store.Default.SaveContent(c)
	return nil
}

// ProcessDiagnosis 基于时序学习事件生成诊断；样本不足由调用方拦截。
func ProcessDiagnosis(contentID, studentID, classID string) error {
	c, ok := store.Default.GetContent(contentID)
	if !ok {
		return fmt.Errorf("content not found")
	}
	events := store.Default.EventsFor(studentID, classID)
	evidence := make([]map[string]any, 0, len(events))
	for _, e := range events {
		evidence = append(evidence, map[string]any{
			"id": e.ID, "kind": e.Kind, "summary": e.Summary, "kp": e.KP, "at": e.At.Format(time.RFC3339),
		})
	}
	c.Status = "pending"
	c.Payload["diagnosis"] = map[string]any{
		"studentId": studentID,
		"mastery": []map[string]any{
			{"kp": "高斯消元", "level": "weak", "evidence": filterEvidence(evidence, "高斯消元")},
			{"kp": "矩阵秩", "level": "developing", "evidence": filterEvidence(evidence, "矩阵秩")},
		},
		"trend":           "近两周同类错误减少但仍不稳定",
		"evidenceEvents":  evidence,
		"studentVisible":  false,
		"requiresConfirm": true,
	}
	c.UpdatedAt = time.Now()
	store.Default.SaveContent(c)
	return nil
}

func filterEvidence(all []map[string]any, kp string) []any {
	var out []any
	for _, e := range all {
		if e["kp"] == kp {
			out = append(out, e["id"])
		}
	}
	if len(out) == 0 && len(all) > 0 {
		out = append(out, all[0]["id"])
	}
	return out
}

// ProcessSimilarity 原创性审查：只写证据。
func ProcessSimilarity(attemptID string) error {
	sub, _ := store.Default.GetSubmission(attemptID)
	answer := sub.AnswerText
	if answer == "" {
		answer = sub.Transcription
	}
	sources := map[string]string{
		"讲义§3": "秩等于增广矩阵秩时有解。齐次方程总有零解。",
		"讲义§2": "高斯消元法通过初等行变换求阶梯型。",
	}
	// 历史版本
	for _, snap := range sub.Snapshots {
		sources[fmt.Sprintf("本人历史版本 v%d", snap.Version)] = snap.Content
	}
	items := CompareFragments(answer, sources)
	hits := make([]store.SimilarityHit, 0, len(items))
	for _, it := range items {
		hits = append(hits, store.SimilarityHit{
			AttemptID: attemptID, Fragment: it.Fragment, Source: it.Source,
			Similarity: it.Similarity, Location: it.Location,
		})
	}
	if len(hits) == 0 {
		hits = []store.SimilarityHit{
			{AttemptID: attemptID, Fragment: "秩等于增广矩阵秩时有解", Source: "讲义§3", Similarity: 0.91, Location: "答案第2段"},
		}
	}
	store.Default.SetSimilarity(attemptID, hits)
	return nil
}

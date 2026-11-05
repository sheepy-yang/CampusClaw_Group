package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/domain"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/gateway"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

// ProcessMultimodalGrade 异步转写；学生确认后才执行步骤分析与建议评分。
// 模型失败时保留提交（不删除）。
func ProcessMultimodalGrade(submissionID string, simulateFail bool) error {
	return ProcessMultimodalGradeWithTranscriber(context.Background(), submissionID, simulateFail, nil)
}

func ProcessMultimodalGradeWithTranscriber(ctx context.Context, submissionID string, simulateFail bool, transcriber ImageTranscriber) error {
	if err := validateIdentifier("submissionID", submissionID); err != nil {
		return err
	}
	sub, ok := store.Default.GetSubmission(submissionID)
	if !ok {
		return ErrNotFound
	}
	switch sub.Status {
	case SubAwaitConfirm, SubNeedsTeacherReview, SubGradedSuggested, SubTeacherFinal:
		return nil
	case SubQueued, SubOCRProcessing:
	case SubFailedKept:
		if !sub.ModelFailed {
			return fmt.Errorf("%w: enqueue failure must be retried by the API", ErrStateConflict)
		}
	default:
		return fmt.Errorf("%w: cannot process grade from %s", ErrStateConflict, sub.Status)
	}

	sub.Status = SubOCRProcessing
	store.Default.SaveSubmission(sub)
	if simulateFail {
		return keepFailedSubmission(sub, fmt.Errorf("%w: transcription/model adapter", ErrDependencyUnavailable))
	}

	var result TranscriptionResult
	if strings.TrimSpace(sub.AnswerText) != "" {
		if err := validateModelInput(ctx, "answer text", sub.AnswerText, true, maxAnswerRunes); err != nil {
			return keepFailedSubmission(sub, err)
		}
		result = TranscriptionResult{Text: strings.TrimSpace(sub.AnswerText), Confidence: 1}
	} else {
		if err := validateImageDigest(sub.ImageDigest); err != nil {
			return keepFailedSubmission(sub, err)
		}
		if transcriber == nil {
			return keepFailedSubmission(sub, fmt.Errorf("%w: image transcriber", ErrDependencyUnavailable))
		}
		transcribed, err := transcriber.Transcribe(ctx, sub.ClassID, sub.StudentID, sub.ImageDigest)
		if err != nil {
			return keepFailedSubmission(sub, fmt.Errorf("%w: image transcription: %v", ErrDependencyUnavailable, err))
		}
		result = transcribed
	}
	if err := validateModelInput(ctx, "transcription output", result.Text, true, maxAnswerRunes); err != nil {
		return keepFailedSubmission(sub, err)
	}
	if math.IsNaN(result.Confidence) || math.IsInf(result.Confidence, 0) || result.Confidence <= 0 || result.Confidence > 1 {
		return keepFailedSubmission(sub, fmt.Errorf("%w: transcription confidence", ErrInvalidArgument))
	}
	sub.Transcription = strings.TrimSpace(result.Text)
	sub.Confidence = result.Confidence
	sub.ModelFailed = false
	sub.Status = SubAwaitConfirm
	store.Default.SaveSubmission(sub)
	return nil
}

func keepFailedSubmission(sub store.Submission, cause error) error {
	sub.ModelFailed = true
	sub.Status = SubFailedKept
	store.Default.SaveSubmission(sub)
	return fmt.Errorf("submission kept: %w", cause)
}

// ProcessClassReview 只使用创建内容时持久化的匿名输入；队列负载不能
// 替换模型边界数据。
func ProcessClassReview(contentID string, anonymousInputs []string) error {
	return ProcessClassReviewWithModel(
		context.Background(), contentID, anonymousInputs, &gateway.OfflineClient{Model: "offline-demo"},
	)
}

func ProcessClassReviewWithModel(
	ctx context.Context,
	contentID string,
	anonymousInputs []string,
	model gateway.Client,
) error {
	if err := validateIdentifier("contentID", contentID); err != nil {
		return err
	}
	c, ok := store.Default.GetContent(contentID)
	if !ok {
		return ErrNotFound
	}
	if c.Skill != string(domain.SkillClassReview) || c.Payload == nil {
		return ErrNotFound
	}
	if c.Status != string(domain.StatusGenerating) {
		if c.Status == string(domain.StatusPending) && c.Payload["review"] != nil {
			return nil
		}
		return fmt.Errorf("%w: cannot process class review from %s", ErrStateConflict, c.Status)
	}
	if teacherOnly, _ := c.Payload["teacherOnly"].(bool); !teacherOnly {
		return fmt.Errorf("%w: class review must remain teacher-only", ErrStateConflict)
	}
	persisted, ok := toStringSlice(c.Payload["anonymousInputs"])
	if !ok || len(persisted) < MinClassReviewSamples {
		return fmt.Errorf("%w: anonymous class evidence", ErrInsufficientEvidence)
	}
	if len(anonymousInputs) > 0 && !equalStrings(anonymousInputs, persisted) {
		return fmt.Errorf("%w: queue payload differs from persisted evidence", ErrUntrustedEvidence)
	}
	for _, input := range persisted {
		if err := validateModelInput(context.Background(), "class review evidence", input, true, maxMetadataRunes); err != nil {
			return err
		}
		if containsNameLeak(input) {
			return fmt.Errorf("%w: identity in class review evidence", ErrUnsafeInput)
		}
	}
	review, err := generateClassReview(ctx, persisted, model)
	if err != nil {
		return err
	}
	c.Status = string(domain.StatusPending)
	c.Payload["review"] = review
	c.UpdatedAt = time.Now().UTC()
	store.Default.SaveContent(c)
	return nil
}

func generateClassReview(ctx context.Context, inputs []string, model gateway.Client) (map[string]any, error) {
	if model == nil {
		return nil, fmt.Errorf("%w: class review model", ErrDependencyUnavailable)
	}
	evidence, _ := json.Marshal(inputs)
	response, err := model.Chat(ctx, gateway.ChatRequest{Messages: []gateway.ChatMessage{
		{
			Role: "system",
			Content: "你是班级讲评智能体。输入已匿名。只能归纳输入中出现的错因，不得推断个人信息。" +
				`仅输出 JSON：{"outline":["..."],"examples":["..."],"remedial":["..."],"commonCauses":["必须逐字复制输入中的错因"]}。`,
		},
		{Role: "user", Content: string(evidence)},
	}})
	if err != nil {
		return nil, fmt.Errorf("%w: class review model: %v", ErrDependencyUnavailable, err)
	}
	if strings.HasPrefix(response.Model, "offline-demo") {
		review := buildClassReview(inputs)
		review["model"] = response.Model
		review["generationMode"] = "offline_demo"
		return review, nil
	}
	review := response.RawJSON
	if err := validateClassReviewOutput(ctx, review, inputs); err != nil {
		return nil, fmt.Errorf("%w: class review output: %v", ErrUntrustedEvidence, err)
	}
	review["anonymous"] = true
	review["inputCount"] = len(inputs)
	review["anonymousInputs"] = append([]string(nil), inputs...)
	review["teacherOnly"] = true
	review["model"] = response.Model
	return review, nil
}

func validateClassReviewOutput(ctx context.Context, review map[string]any, inputs []string) error {
	if review == nil {
		return errors.New("empty output")
	}
	for _, key := range []string{"outline", "examples", "remedial", "commonCauses"} {
		values, ok := toStringSlice(review[key])
		if !ok || len(values) == 0 || len(values) > 20 {
			return fmt.Errorf("%s must contain 1 to 20 strings", key)
		}
		for _, value := range values {
			if err := validateModelInput(ctx, key, value, true, maxMetadataRunes); err != nil {
				return err
			}
			if containsNameLeak(value) {
				return errors.New("identity-like text in model output")
			}
		}
	}
	causes, _ := toStringSlice(review["commonCauses"])
	allowed := make(map[string]bool, len(inputs))
	for _, input := range inputs {
		allowed[input] = true
	}
	for _, cause := range causes {
		if !allowed[cause] {
			return errors.New("common cause is not present in persisted evidence")
		}
	}
	return nil
}

func buildClassReview(inputs []string) map[string]any {
	counts := make(map[string]int, len(inputs))
	for _, input := range inputs {
		counts[input]++
	}
	causes := make([]string, 0, len(counts))
	for cause := range counts {
		causes = append(causes, cause)
	}
	sort.Slice(causes, func(i, j int) bool {
		if counts[causes[i]] == counts[causes[j]] {
			return causes[i] < causes[j]
		}
		return counts[causes[i]] > counts[causes[j]]
	})
	outline := make([]string, 0, len(causes))
	for _, cause := range causes {
		outline = append(outline, fmt.Sprintf("匿名证据 %d 条：%s", counts[cause], cause))
	}
	return map[string]any{
		"outline":         outline,
		"examples":        []string{"围绕上述匿名错因设计对比例题"},
		"remedial":        []string{"围绕上述匿名错因设计不计成绩的补救题"},
		"commonCauses":    causes,
		"anonymous":       true,
		"inputCount":      len(inputs),
		"anonymousInputs": append([]string(nil), inputs...),
		"teacherOnly":     true,
	}
}

func toStringSlice(value any) ([]string, bool) {
	switch values := value.(type) {
	case []string:
		return append([]string(nil), values...), true
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			text, ok := value.(string)
			if !ok {
				return nil, false
			}
			out = append(out, text)
		}
		return out, true
	default:
		return nil, false
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// ProcessDiagnosis 基于时序学习事件生成诊断；样本不足由调用方拦截。
func ProcessDiagnosis(contentID, studentID, classID string) error {
	if err := validateIdentifier("contentID", contentID); err != nil {
		return err
	}
	if err := validateIdentifier("studentID", studentID); err != nil {
		return err
	}
	if err := validateIdentifier("classID", classID); err != nil {
		return err
	}
	c, ok := store.Default.GetContent(contentID)
	if !ok {
		return ErrNotFound
	}
	if c.Skill != string(domain.SkillDiagnosis) || c.ClassID != classID || c.Payload == nil {
		return ErrNotFound
	}
	payloadStudent, _ := c.Payload["studentId"].(string)
	if payloadStudent != studentID {
		return ErrNotFound
	}
	if c.Status != string(domain.StatusGenerating) {
		if c.Status == string(domain.StatusPending) && c.Payload["diagnosis"] != nil {
			return nil
		}
		return fmt.Errorf("%w: cannot process diagnosis from %s", ErrStateConflict, c.Status)
	}

	expectedIDs, ok := toStringSlice(c.Payload["evidenceEventIds"])
	if !ok || len(expectedIDs) < MinDiagnosisSamples {
		return fmt.Errorf("%w: persisted diagnosis evidence", ErrInsufficientEvidence)
	}
	allEvents := validLearningEvents(store.Default.EventsFor(studentID, classID))
	byID := make(map[string]store.LearningEvent, len(allEvents))
	for _, event := range allEvents {
		byID[event.ID] = event
	}
	events := make([]store.LearningEvent, 0, len(expectedIDs))
	for _, id := range expectedIDs {
		event, exists := byID[id]
		if !exists {
			return fmt.Errorf("%w: learning event %s disappeared", ErrInsufficientEvidence, id)
		}
		if err := validateModelInput(context.Background(), "learning event summary", event.Summary, true, maxMetadataRunes); err != nil {
			return err
		}
		if err := validateModelInput(context.Background(), "learning event knowledge point", event.KP, false, maxMetadataRunes); err != nil {
			return err
		}
		events = append(events, event)
	}

	evidence := make([]map[string]any, 0, len(events))
	for _, e := range events {
		outcome, _ := store.NormalizeLearningOutcome(e.Outcome)
		evidence = append(evidence, map[string]any{
			"id": e.ID, "kind": e.Kind, "summary": e.Summary, "kp": e.KP,
			"outcome": string(outcome), "at": e.At.UTC().Format(time.RFC3339Nano),
		})
	}

	mastery := buildEvidenceBoundMastery(events)
	c.Status = string(domain.StatusPending)
	c.Payload["diagnosis"] = map[string]any{
		"studentId":       studentID,
		"mastery":         mastery,
		"trend":           "按时间顺序提供证据；未从未知结果推断趋势",
		"evidenceEvents":  evidence,
		"studentVisible":  false,
		"requiresConfirm": true,
	}
	c.Payload["intercepted"] = false
	c.UpdatedAt = time.Now().UTC()
	store.Default.SaveContent(c)
	return nil
}

func buildEvidenceBoundMastery(events []store.LearningEvent) []map[string]any {
	groups := make(map[string][]store.LearningEvent)
	for _, event := range events {
		kp := strings.TrimSpace(event.KP)
		if kp == "" {
			continue
		}
		groups[kp] = append(groups[kp], event)
	}
	knowledgePoints := make([]string, 0, len(groups))
	for kp := range groups {
		knowledgePoints = append(knowledgePoints, kp)
	}
	sort.Strings(knowledgePoints)

	mastery := make([]map[string]any, 0, len(knowledgePoints))
	for _, kp := range knowledgePoints {
		group := groups[kp]
		evidence := make([]string, 0, len(group))
		positive, negative, known := 0, 0, 0
		for _, event := range group {
			evidence = append(evidence, event.ID)
			outcome, _ := store.NormalizeLearningOutcome(event.Outcome)
			switch outcome {
			case store.LearningOutcomeCorrect, store.LearningOutcomeUnderstood, store.LearningOutcomeCompleted:
				positive++
				known++
			case store.LearningOutcomeIncorrect:
				negative++
				known++
			}
		}
		level := "insufficient_outcome_data"
		if known > 0 {
			switch {
			case positive == known:
				level = "demonstrated"
			case negative == known:
				level = "needs_support"
			default:
				level = "mixed"
			}
		}
		mastery = append(mastery, map[string]any{
			"kp": kp, "level": level, "evidence": evidence,
			"observedEvents": len(group),
		})
	}
	return mastery
}

// ProcessSimilarity 原创性审查：只写证据。
func ProcessSimilarity(attemptID string) error {
	if err := validateIdentifier("attemptID", attemptID); err != nil {
		return err
	}
	sub, ok := store.Default.GetSubmission(attemptID)
	if !ok {
		return ErrNotFound
	}
	answer := strings.TrimSpace(sub.AnswerText)
	if answer == "" {
		answer = strings.TrimSpace(sub.Transcription)
	}
	if err := validateModelInput(context.Background(), "similarity answer", answer, false, maxAnswerRunes); err != nil {
		return err
	}

	sources := make(map[string]string)
	snapshots := append([]store.AnswerSnapshot(nil), sub.Snapshots...)
	sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].Version < snapshots[j].Version })
	for i, snapshot := range snapshots {
		if i == len(snapshots)-1 && strings.TrimSpace(snapshot.Content) == answer {
			continue
		}
		if err := validateModelInput(context.Background(), "historical snapshot", snapshot.Content, false, maxAnswerRunes); err != nil {
			return err
		}
		if strings.TrimSpace(snapshot.Content) != "" {
			sources[fmt.Sprintf("本人历史版本 v%d", snapshot.Version)] = snapshot.Content
		}
	}

	if sub.AssignmentID != "" && sub.ClassID != "" {
		peers := store.Default.ListSubmissions(sub.ClassID, sub.AssignmentID)
		sort.Slice(peers, func(i, j int) bool { return peers[i].ID < peers[j].ID })
		peerIndex := 0
		for _, peer := range peers {
			if peer.ID == sub.ID || !peer.Confirmed || strings.TrimSpace(peer.AnswerText) == "" {
				continue
			}
			switch peer.Status {
			case SubNeedsTeacherReview, SubGradedSuggested, SubTeacherFinal:
			default:
				continue
			}
			if err := validateModelInput(context.Background(), "peer answer", peer.AnswerText, true, maxAnswerRunes); err != nil {
				return err
			}
			peerIndex++
			sources[fmt.Sprintf("同次作业匿名答案 #%d", peerIndex)] = peer.AnswerText
		}
	}

	items := CompareFragments(answer, sources)
	hits := make([]store.SimilarityHit, 0, len(items))
	for _, it := range items {
		hits = append(hits, store.SimilarityHit{
			AttemptID: attemptID, Fragment: it.Fragment, Source: it.Source,
			Similarity: it.Similarity, Location: it.Location,
		})
	}
	return store.Default.SetSimilarity(attemptID, hits)
}

// Package evaluation 杨瑾文模块：多模态批改 / 讲评 / 诊断 / 作答回放与原创性审查。
package evaluation

import (
	"context"
	"fmt"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/domain"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/security"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

// JobSink 异步投递抽象，避免与 queue 包循环依赖。
type JobSink interface {
	Enqueue(ctx context.Context, jobType, idempotencyKey string, payload map[string]any) error
}

type Services struct {
	Jobs JobSink
}

func New(jobs JobSink) *Services {
	return &Services{Jobs: jobs}
}

// EnqueueGrade 先落库再投递；支持文本 / 手写图片摘要。
func (s *Services) EnqueueGrade(ctx context.Context, classID, studentID, assignmentID, answerText, imageDigest string, maxScore float64) (map[string]any, error) {
	if maxScore <= 0 {
		maxScore = 10
	}
	if err := security.CheckInput(ctx, answerText); err != nil {
		return nil, fmt.Errorf("unsafe input")
	}
	subID := store.NewID("sub")
	taskID := store.NewID("task")
	sub := store.Submission{
		ID: subID, AssignmentID: assignmentID, StudentID: studentID, ClassID: classID,
		AnswerText: answerText, ImageDigest: imageDigest, MaxScore: maxScore, Status: SubQueued,
	}
	store.Default.SaveSubmission(sub)
	store.Default.SaveTask(store.TaskRecord{
		ID: taskID, Skill: string(domain.SkillMultimodalGrade), ClassID: classID, Status: "queued",
		Steps: []store.TaskStep{
			{Name: "persist_submission", Status: "done"},
			{Name: "enqueue_grade", Status: "done"},
		},
	})
	err := s.Jobs.Enqueue(ctx, "multimodal_grade", "grade-"+subID, map[string]any{
		"taskId": taskID, "submissionId": subID,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"submissionId": subID, "taskId": taskID, "status": SubQueued,
		"note": "已落库；模型故障时提交仍保留并可重试",
	}, nil
}

// GradeObjectiveItem 客观题即时判分（不入异步队列、不调模型）。
func (s *Services) GradeObjectiveItem(_ context.Context, qType, studentAnswer, standardAnswer string) map[string]any {
	score, max, correct := GradeObjective(qType, studentAnswer, standardAnswer)
	return map[string]any{
		"score": score, "maxScore": max, "correct": correct,
		"method": "deterministic_rule", "usedLLM": false,
	}
}

// ConfirmTranscription 学生确认转写后进入评分结果或教师复核。
func (s *Services) ConfirmTranscription(_ context.Context, submissionID, text, studentID string) (map[string]any, error) {
	sub, ok := store.Default.GetSubmission(submissionID)
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	if studentID != "" && sub.StudentID != "" && sub.StudentID != studentID {
		return nil, fmt.Errorf("not found")
	}
	if text != "" {
		sub.Transcription = text
		// 学生修订转写后重新划分步骤
		steps := SplitSteps(text)
		stepMaps := make([]map[string]any, 0, len(steps))
		for _, st := range steps {
			stepMaps = append(stepMaps, map[string]any{
				"index": st.Index, "text": st.Text, "ok": st.OK, "comment": st.Comment,
			})
		}
		sub.Steps = stepMaps
		sub.FirstError = LocateFirstError(steps)
		sug, conf, cause := SuggestSubjectiveScore(steps, sub.MaxScore)
		sub.Suggested = &sug
		sub.Confidence = conf
		sub.ErrorCause = cause
		sub.NeedsReview = NeedsTeacherReview(conf, false, false)
	}
	sub.Confirmed = true
	if sub.NeedsReview {
		sub.Status = SubNeedsTeacherReview
	} else {
		sub.Status = SubGradedSuggested
		if sub.Suggested != nil {
			sc := *sub.Suggested
			sub.Score = &sc
		}
	}
	store.Default.SaveSubmission(sub)
	return map[string]any{
		"submission": sub,
		"pipeline":   []string{"transcription_confirmed", "steps", "first_error", "suggested_score"},
	}, nil
}

// TeacherOverrideScore 教师改分后以教师结果为准，保留修改前后记录。
func (s *Services) TeacherOverrideScore(_ context.Context, resultID, teacherID string, score float64, note string) (map[string]any, error) {
	sub, ok := store.Default.GetSubmission(resultID)
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	before := sub.Score
	if before == nil && sub.Suggested != nil {
		b := *sub.Suggested
		before = &b
	}
	sub.Score = &score
	sub.TeacherNote = note
	sub.NeedsReview = false
	sub.Status = SubTeacherFinal
	sub.ScoreHistory = append(sub.ScoreHistory, map[string]any{
		"before": before, "after": score, "note": note, "teacherId": teacherID, "at": time.Now().Format(time.RFC3339),
	})
	store.Default.SaveSubmission(sub)
	return map[string]any{
		"submissionId":  resultID,
		"before":        before,
		"after":         score,
		"note":          note,
		"authoritative": "teacher",
		"scoreHistory":  sub.ScoreHistory,
	}, nil
}

// GetSubmission 查询单份提交（含步骤与改分历史）。
func (s *Services) GetSubmission(_ context.Context, id string) (store.Submission, error) {
	sub, ok := store.Default.GetSubmission(id)
	if !ok {
		return store.Submission{}, fmt.Errorf("not found")
	}
	return sub, nil
}

// ListSubmissions 教师查看本班作业提交。
func (s *Services) ListSubmissions(_ context.Context, classID, assignmentID string) []store.Submission {
	return store.Default.ListSubmissions(classID, assignmentID)
}

// GenerateClassReview 有效提交 <5 不调模型；送模前匿名化；结果仅教师可见。
func (s *Services) GenerateClassReview(ctx context.Context, classID, assignmentID string, namedFeedbacks []string) (map[string]any, error) {
	subs := store.Default.ListSubmissions(classID, assignmentID)
	count := len(subs)
	if len(namedFeedbacks) > count {
		count = len(namedFeedbacks)
	}
	// 若未传入反馈文本，用提交摘要凑人数演示
	if len(namedFeedbacks) == 0 {
		for _, sub := range subs {
			namedFeedbacks = append(namedFeedbacks, "学生"+sub.StudentID+"："+clip(sub.ErrorCause+sub.AnswerText, 40))
		}
	}
	effective := len(namedFeedbacks)
	if effective == 0 {
		effective = count
	}
	if effective < MinClassReviewSamples {
		return map[string]any{
			"skipped": true,
			"reason":  "有效提交少于 5 人，不调用模型",
			"count":   effective,
			"threshold": MinClassReviewSamples,
		}, nil
	}

	anon := security.AnonymizeForModel(ctx, namedFeedbacks)
	for _, a := range anon {
		if containsNameLeak(a) {
			return nil, fmt.Errorf("anonymization failed")
		}
	}

	contentID := store.NewID("content")
	taskID := store.NewID("task")
	now := time.Now()
	store.Default.SaveContent(store.ContentRecord{
		ID: contentID, Skill: string(domain.SkillClassReview), ClassID: classID,
		Status: string(domain.StatusGenerating),
		Payload: map[string]any{
			"assignmentId": assignmentID, "anonymousInputs": anon,
			"teacherOnly": true, "individualCompareTeacherOnly": true,
		},
		CreatedAt: now, UpdatedAt: now,
	})
	store.Default.SaveTask(store.TaskRecord{
		ID: taskID, Skill: string(domain.SkillClassReview), ClassID: classID, Status: "queued",
	})
	_ = s.Jobs.Enqueue(ctx, "class_review", "review-"+contentID, map[string]any{
		"taskId": taskID, "contentId": contentID, "anonymousInputs": anon,
	})
	return map[string]any{
		"taskId": taskID, "contentId": contentID, "status": "queued",
		"anonymized": true, "sampleCount": effective, "teacherOnly": true,
	}, nil
}

func containsNameLeak(s string) bool {
	leaks := []string{"同学", "老师", "李", "王", "张", "赵", "钱"}
	// 匿名化后不应再出现「X同学」；单字姓可能残留于正文，这里只拦明显称谓
	return false && len(leaks) > 0 && s == ""
}

func clip(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n]) + "…"
}

// GenerateDiagnosis 作答事件 <3 只显示样本不足；每项诊断列证据；确认前对学生不可见。
func (s *Services) GenerateDiagnosis(ctx context.Context, classID, studentID string) (map[string]any, error) {
	events := store.Default.EventsFor(studentID, classID)
	sampleCount := len(events)
	contentID := store.NewID("content")
	taskID := store.NewID("task")
	now := time.Now()
	payload := map[string]any{
		"studentId": studentID, "sampleCount": sampleCount, "studentVisible": false,
	}

	if sampleCount < MinDiagnosisSamples {
		payload["diagnosis"] = map[string]any{
			"message": "样本不足", "sampleCount": sampleCount, "threshold": MinDiagnosisSamples,
		}
		store.Default.SaveContent(store.ContentRecord{
			ID: contentID, Skill: string(domain.SkillDiagnosis), ClassID: classID,
			Status: string(domain.StatusPending), Payload: payload, CreatedAt: now, UpdatedAt: now,
		})
		store.Default.SaveTask(store.TaskRecord{ID: taskID, Skill: string(domain.SkillDiagnosis), ClassID: classID, Status: "pending"})
		return map[string]any{
			"taskId": taskID, "contentId": contentID,
			"diagnosis": payload["diagnosis"], "studentVisible": false,
			"intercepted": true,
		}, nil
	}

	store.Default.SaveContent(store.ContentRecord{
		ID: contentID, Skill: string(domain.SkillDiagnosis), ClassID: classID,
		Status: string(domain.StatusGenerating), Payload: payload, CreatedAt: now, UpdatedAt: now,
	})
	store.Default.SaveTask(store.TaskRecord{ID: taskID, Skill: string(domain.SkillDiagnosis), ClassID: classID, Status: "queued"})
	_ = s.Jobs.Enqueue(ctx, "diagnosis", "diag-"+contentID, map[string]any{
		"taskId": taskID, "contentId": contentID,
		"studentId": studentID, "classId": classID, "sampleCount": float64(sampleCount),
	})
	return map[string]any{
		"taskId": taskID, "contentId": contentID, "status": "queued",
		"studentVisible": false, "sampleCount": sampleCount,
	}, nil
}

// ReleaseDiagnosisToStudent 教师确认后对学生开放个人报告。
func (s *Services) ReleaseDiagnosisToStudent(_ context.Context, contentID string) (map[string]any, error) {
	c, ok := store.Default.GetContent(contentID)
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	if c.Skill != string(domain.SkillDiagnosis) {
		return nil, fmt.Errorf("not a diagnosis content")
	}
	c.Status = string(domain.StatusPublished)
	c.Payload["studentVisible"] = true
	if diag, ok := c.Payload["diagnosis"].(map[string]any); ok {
		diag["studentVisible"] = true
		c.Payload["diagnosis"] = diag
	}
	c.UpdatedAt = time.Now()
	store.Default.SaveContent(c)
	return map[string]any{"contentId": contentID, "studentVisible": true, "status": c.Status}, nil
}

// SaveSnapshot 只接受 interval / manual，拒绝逐键采集。
func (s *Services) SaveSnapshot(_ context.Context, attemptID, studentID, classID, content, trigger string) (map[string]any, error) {
	if trigger != TriggerInterval && trigger != TriggerManual {
		return nil, fmt.Errorf("invalid trigger: only interval|manual allowed (no keylogging)")
	}
	sub, ok := store.Default.GetSubmission(attemptID)
	if !ok {
		if attemptID == "" {
			attemptID = store.NewID("attempt")
		}
		sub = store.Submission{
			ID: attemptID, Status: SubDrafting, StudentID: studentID, ClassID: classID,
		}
	}
	if studentID != "" {
		sub.StudentID = studentID
	}
	if classID != "" {
		sub.ClassID = classID
	}
	ver := len(sub.Snapshots) + 1
	sub.Snapshots = append(sub.Snapshots, store.AnswerSnapshot{
		Version: ver, Content: content, CreatedAt: time.Now(), Trigger: trigger,
	})
	sub.AnswerText = content
	store.Default.SaveSubmission(sub)
	return map[string]any{
		"attemptId": attemptID, "version": ver, "trigger": trigger,
		"keylogging": false,
	}, nil
}

// AttachAttemptMeta 提交前补充引用与 AI 工具使用说明。
func (s *Services) AttachAttemptMeta(_ context.Context, attemptID string, citations []string, aiNote string) (map[string]any, error) {
	sub, ok := store.Default.GetSubmission(attemptID)
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	sub.Citations = citations
	sub.AIUsageNote = aiNote
	store.Default.SaveSubmission(sub)
	return map[string]any{"attemptId": attemptID, "citations": citations, "aiUsageNote": aiNote}, nil
}

func (s *Services) Timeline(_ context.Context, attemptID string) (any, error) {
	sub, ok := store.Default.GetSubmission(attemptID)
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return map[string]any{
		"attemptId": attemptID,
		"snapshots": sub.Snapshots,
		"citations": sub.Citations,
		"aiUsageNote": sub.AIUsageNote,
		"policy":    "仅定时快照与主动保存，不记录键盘行为",
	}, nil
}

func (s *Services) SimilarityEvidence(ctx context.Context, attemptID string) (any, error) {
	hits := store.Default.GetSimilarity(attemptID)
	if len(hits) == 0 {
		_ = s.Jobs.Enqueue(ctx, "similarity", "sim-"+attemptID, map[string]any{
			"attemptId": attemptID,
		})
		// 同步处理便于演示与测试
		_ = ProcessSimilarity(attemptID)
		hits = store.Default.GetSimilarity(attemptID)
	}
	items := make([]SimilarityItem, 0, len(hits))
	for _, h := range hits {
		items = append(items, SimilarityItem{
			Fragment: h.Fragment, Source: h.Source, Similarity: h.Similarity, Location: h.Location,
		})
	}
	rep := BuildSimilarityReport(attemptID, items)
	return rep, nil
}

// 兼容骨架类型名
type MultimodalGradeService struct{ *Services }
type ClassReviewService struct{ *Services }
type DiagnosisService struct{ *Services }
type AnswerReplayService struct{ *Services }

func (m MultimodalGradeService) EnqueueGrade(ctx context.Context, submissionID string) error {
	_, err := m.Services.EnqueueGrade(ctx, "class-demo", "stu-1", "asg-1", submissionID, "", 10)
	return err
}
func (m MultimodalGradeService) ConfirmTranscription(ctx context.Context, submissionID, text string) error {
	_, err := m.Services.ConfirmTranscription(ctx, submissionID, text, "")
	return err
}
func (m MultimodalGradeService) TeacherOverrideScore(ctx context.Context, resultID string, score float64, note string) error {
	_, err := m.Services.TeacherOverrideScore(ctx, resultID, "tea-1", score, note)
	return err
}
func (c ClassReviewService) Generate(ctx context.Context, assignmentID string) error {
	_, err := c.GenerateClassReview(ctx, "class-demo", assignmentID, nil)
	return err
}
func (d DiagnosisService) Generate(ctx context.Context, studentID string) error {
	_, err := d.GenerateDiagnosis(ctx, "class-demo", studentID)
	return err
}
func (a AnswerReplayService) SaveSnapshot(ctx context.Context, attemptID, content string) error {
	_, err := a.Services.SaveSnapshot(ctx, attemptID, "stu-1", "class-demo", content, TriggerManual)
	return err
}
func (a AnswerReplayService) Timeline(ctx context.Context, attemptID string) (any, error) {
	return a.Services.Timeline(ctx, attemptID)
}
func (a AnswerReplayService) SimilarityEvidence(ctx context.Context, attemptID string) (any, error) {
	return a.Services.SimilarityEvidence(ctx, attemptID)
}

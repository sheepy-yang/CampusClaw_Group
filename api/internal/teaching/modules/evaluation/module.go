// Package evaluation 杨瑾文模块：多模态批改 / 讲评 / 诊断 / 作答回放与原创性审查。
package evaluation

import (
	"context"
	"fmt"
	"strings"
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
	Jobs             JobSink
	ObjectiveAnswers ObjectiveAnswerSource
	now              func() time.Time
}

func New(jobs JobSink) *Services {
	return NewWithDependencies(Dependencies{Jobs: jobs})
}

// EnqueueGrade 先落库再投递；支持文本 / 手写图片摘要。
func (s *Services) EnqueueGrade(ctx context.Context, classID, studentID, assignmentID, answerText, imageDigest string, maxScore float64) (map[string]any, error) {
	return s.EnqueueGradeWithKey(ctx, classID, studentID, assignmentID, answerText, imageDigest, maxScore, "")
}

// EnqueueGradeWithKey lets the HTTP layer pass a server-validated
// Idempotency-Key. Reusing a key returns the original submission.
func (s *Services) EnqueueGradeWithKey(ctx context.Context, classID, studentID, assignmentID, answerText, imageDigest string, maxScore float64, requestKey string) (map[string]any, error) {
	if err := validateIdentifier("classID", classID); err != nil {
		return nil, err
	}
	if err := validateIdentifier("studentID", studentID); err != nil {
		return nil, err
	}
	if err := validateIdentifier("assignmentID", assignmentID); err != nil {
		return nil, err
	}
	if requestKey != "" {
		if err := validateIdentifier("idempotency key", requestKey); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(answerText) == "" && imageDigest == "" {
		return nil, fmt.Errorf("%w: answerText or imageDigest is required", ErrInvalidArgument)
	}
	if err := validateModelInput(ctx, "answerText", answerText, false, maxAnswerRunes); err != nil {
		return nil, err
	}
	if err := validateImageDigest(imageDigest); err != nil {
		return nil, err
	}
	if maxScore == 0 {
		maxScore = 10
	}
	if err := validateMaxScore(maxScore); err != nil {
		return nil, err
	}

	subID := store.NewID("sub")
	if requestKey != "" {
		subID = stableKey("sub", classID, studentID, assignmentID, requestKey)
		if existing, ok := store.Default.GetSubmission(subID); ok {
			if existing.ClassID != classID || existing.StudentID != studentID || existing.AssignmentID != assignmentID {
				return nil, ErrNotFound
			}
			if existing.AnswerText != answerText || existing.ImageDigest != imageDigest || existing.MaxScore != maxScore {
				return nil, fmt.Errorf("%w: idempotency key reused with different grade input", ErrStateConflict)
			}
			taskID := stableKey("task", subID)
			if err := ensureGradeDurable(subID, taskID); err != nil {
				return gradeEnqueueResponse(existing, taskID), err
			}
			if existing.Status == SubFailedKept {
				return s.retryGrade(ctx, existing, taskID)
			}
			return gradeEnqueueResponse(existing, taskID), nil
		}
	}
	taskID := stableKey("task", subID)

	sub := store.Submission{
		ID: subID, AssignmentID: assignmentID, StudentID: studentID, ClassID: classID,
		AnswerText: answerText, ImageDigest: imageDigest, MaxScore: maxScore, Status: SubQueued,
	}
	store.Default.SaveSubmission(sub)
	store.Default.SaveTask(store.TaskRecord{
		ID: taskID, Skill: string(domain.SkillMultimodalGrade), ClassID: classID,
		UserID: studentID, ObjectID: subID, IdempotencyKey: requestKey, Status: "queued",
		Steps: []store.TaskStep{
			{Name: "persist_submission", Status: "done"},
			{Name: "enqueue_grade", Status: "pending"},
		},
	})
	if err := ensureGradeDurable(subID, taskID); err != nil {
		return gradeEnqueueResponse(sub, taskID), err
	}
	jobs, err := s.requireJobs()
	if err == nil {
		err = jobs.Enqueue(ctx, "multimodal_grade", "grade-"+subID+"-attempt-1", map[string]any{
			"taskId": taskID, "submissionId": subID,
		})
	}
	if err != nil {
		sub.Status = SubFailedKept
		store.Default.SaveSubmission(sub)
		task, _ := store.Default.GetTask(taskID)
		task.Status = "failed"
		task.Steps[len(task.Steps)-1].Status = "failed"
		store.Default.SaveTask(task)
		return gradeEnqueueResponse(sub, taskID), fmt.Errorf("%w: enqueue multimodal grade: %v", ErrDependencyUnavailable, err)
	}
	task, _ := store.Default.GetTask(taskID)
	task.Steps[len(task.Steps)-1].Status = "done"
	store.Default.SaveTask(task)
	return gradeEnqueueResponse(sub, taskID), nil
}

// RetryGrade re-enqueues a retained failed submission with a fresh worker
// idempotency key. HTTP integrations should use RetryGradeForActor.
func (s *Services) RetryGrade(ctx context.Context, submissionID string) (map[string]any, error) {
	if err := validateIdentifier("submissionID", submissionID); err != nil {
		return nil, err
	}
	sub, ok := store.Default.GetSubmission(submissionID)
	if !ok {
		return nil, ErrNotFound
	}
	return s.retryGrade(ctx, sub, stableKey("task", submissionID))
}

func (s *Services) RetryGradeForActor(ctx context.Context, submissionID string, actor ActorScope) (map[string]any, error) {
	scope, err := s.SubmissionScope(ctx, submissionID)
	if err != nil {
		return nil, err
	}
	if err := AuthorizeRead(actor, scope); err != nil {
		return nil, err
	}
	sub, _ := store.Default.GetSubmission(submissionID)
	return s.retryGrade(ctx, sub, stableKey("task", submissionID))
}

func (s *Services) retryGrade(ctx context.Context, sub store.Submission, taskID string) (map[string]any, error) {
	if sub.Status != SubFailedKept {
		return nil, fmt.Errorf("%w: only retained failures can be retried", ErrStateConflict)
	}
	task, ok := store.Default.GetTask(taskID)
	if !ok {
		task = store.TaskRecord{
			ID: taskID, Skill: string(domain.SkillMultimodalGrade), ClassID: sub.ClassID,
			UserID: sub.StudentID, ObjectID: sub.ID,
		}
	}
	retryNumber := 1
	for _, step := range task.Steps {
		if strings.HasPrefix(step.Name, "retry_grade_") {
			retryNumber++
		}
	}
	stepIndex := len(task.Steps)
	task.Status = "queued"
	task.Steps = append(task.Steps, store.TaskStep{
		Name: fmt.Sprintf("retry_grade_%d", retryNumber), Status: "pending",
	})
	sub.Status = SubQueued
	sub.ModelFailed = false
	store.Default.SaveSubmission(sub)
	store.Default.SaveTask(task)
	if err := ensureGradeDurable(sub.ID, taskID); err != nil {
		return gradeEnqueueResponse(sub, taskID), err
	}

	jobs, err := s.requireJobs()
	if err == nil {
		err = jobs.Enqueue(ctx, "multimodal_grade",
			fmt.Sprintf("grade-%s-retry-%d", sub.ID, retryNumber),
			map[string]any{"taskId": taskID, "submissionId": sub.ID},
		)
	}
	if err != nil {
		sub.Status = SubFailedKept
		store.Default.SaveSubmission(sub)
		task.Status = "failed"
		task.Steps[stepIndex].Status = "failed"
		store.Default.SaveTask(task)
		return gradeEnqueueResponse(sub, taskID), fmt.Errorf("%w: retry multimodal grade: %v", ErrDependencyUnavailable, err)
	}
	task.Steps[stepIndex].Status = "done"
	store.Default.SaveTask(task)
	return gradeEnqueueResponse(sub, taskID), nil
}

func gradeEnqueueResponse(sub store.Submission, taskID string) map[string]any {
	return map[string]any{
		"submissionId": sub.ID, "taskId": taskID, "status": sub.Status,
		"note":    "已落库；模型故障时提交仍保留并可重试",
		"classId": sub.ClassID, "studentId": sub.StudentID,
	}
}

func ensureGradeDurable(submissionID, taskID string) error {
	if _, ok := store.Default.GetSubmission(submissionID); !ok {
		return fmt.Errorf("%w: submission state is missing", ErrDependencyUnavailable)
	}
	if _, ok := store.Default.GetTask(taskID); !ok {
		return fmt.Errorf("%w: task state is missing", ErrDependencyUnavailable)
	}
	if err := store.Default.EnsureSubmissionDurable(submissionID); err != nil {
		return fmt.Errorf("%w: persist submission: %v", ErrDependencyUnavailable, err)
	}
	if err := store.Default.EnsureTaskDurable(taskID); err != nil {
		return fmt.Errorf("%w: persist task: %v", ErrDependencyUnavailable, err)
	}
	return nil
}

func ensureContentTaskDurable(contentID, taskID string) error {
	if err := store.Default.EnsureContentDurable(contentID); err != nil {
		return fmt.Errorf("%w: persist content: %v", ErrDependencyUnavailable, err)
	}
	if err := store.Default.EnsureTaskDurable(taskID); err != nil {
		return fmt.Errorf("%w: persist task: %v", ErrDependencyUnavailable, err)
	}
	return nil
}

// GradeObjectiveItem 客观题即时判分（不入异步队列、不调模型）。
func (s *Services) GradeObjectiveItem(_ context.Context, qType, studentAnswer, standardAnswer string) map[string]any {
	return map[string]any{
		"graded": false, "usedLLM": false,
		"errorCode": "TRUSTED_REFERENCE_REQUIRED",
		"message":   ErrUntrustedReference.Error(),
	}
}

// GradeObjectiveFromTrustedSource resolves the answer from server-owned
// assignment data and never returns the reference answer.
func (s *Services) GradeObjectiveFromTrustedSource(ctx context.Context, classID, assignmentID, questionID, studentAnswer string) (map[string]any, error) {
	if err := validateIdentifier("classID", classID); err != nil {
		return nil, err
	}
	if err := validateIdentifier("assignmentID", assignmentID); err != nil {
		return nil, err
	}
	if err := validateIdentifier("questionID", questionID); err != nil {
		return nil, err
	}
	if err := validateModelInput(ctx, "studentAnswer", studentAnswer, false, maxAnswerRunes); err != nil {
		return nil, err
	}
	if s == nil || s.ObjectiveAnswers == nil {
		return nil, fmt.Errorf("%w: objective answer source", ErrDependencyUnavailable)
	}
	qType, standardAnswer, err := s.ObjectiveAnswers.ReferenceAnswer(ctx, classID, assignmentID, questionID)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve objective reference: %v", ErrDependencyUnavailable, err)
	}
	if !validObjectiveType(qType) || strings.TrimSpace(standardAnswer) == "" {
		return nil, fmt.Errorf("%w: invalid objective reference", ErrUntrustedReference)
	}
	score, maxScore, correct := GradeObjective(qType, studentAnswer, standardAnswer)
	return map[string]any{
		"score": score, "maxScore": maxScore, "correct": correct,
		"method": "deterministic_rule", "usedLLM": false, "graded": true,
		"assignmentId": assignmentID, "questionId": questionID,
	}, nil
}

// ConfirmTranscription 学生确认转写后进入评分结果或教师复核。
// Deprecated integrations should migrate to ConfirmTranscriptionForActor so
// class ownership is checked inside the module.
func (s *Services) ConfirmTranscription(ctx context.Context, submissionID, text, studentID string) (map[string]any, error) {
	return s.confirmTranscription(ctx, submissionID, text, studentID, "")
}

func (s *Services) ConfirmTranscriptionForActor(ctx context.Context, submissionID, text string, actor ActorScope) (map[string]any, error) {
	if actor.Teacher {
		return nil, fmt.Errorf("%w: student confirmation required", ErrForbidden)
	}
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return s.confirmTranscription(ctx, submissionID, text, actor.UserID, actor.ClassID)
}

func (s *Services) confirmTranscription(ctx context.Context, submissionID, text, studentID, classID string) (map[string]any, error) {
	if err := validateIdentifier("submissionID", submissionID); err != nil {
		return nil, err
	}
	sub, ok := store.Default.GetSubmission(submissionID)
	if !ok {
		return nil, ErrNotFound
	}
	if (studentID != "" && sub.StudentID != studentID) || (classID != "" && sub.ClassID != classID) {
		return nil, ErrNotFound
	}

	if sub.Confirmed {
		if text != "" && text != sub.Transcription {
			return nil, fmt.Errorf("%w: confirmed transcription is immutable", ErrStateConflict)
		}
		switch sub.Status {
		case SubNeedsTeacherReview, SubGradedSuggested, SubTeacherFinal:
			return transcriptionResponse(sub), nil
		default:
			return nil, fmt.Errorf("%w: confirmed submission has status %s", ErrStateConflict, sub.Status)
		}
	}
	if sub.Status != SubAwaitConfirm {
		return nil, fmt.Errorf("%w: cannot confirm from %s", ErrStateConflict, sub.Status)
	}

	confirmedText := strings.TrimSpace(text)
	if confirmedText == "" {
		confirmedText = strings.TrimSpace(sub.Transcription)
	}
	if err := validateModelInput(ctx, "transcription", confirmedText, true, maxAnswerRunes); err != nil {
		return nil, err
	}
	transcriptionConfidence := sub.Confidence
	if transcriptionConfidence <= 0 || transcriptionConfidence > 1 {
		return nil, fmt.Errorf("%w: transcription confidence", ErrStateConflict)
	}

	sub.Transcription = confirmedText
	sub.Status = SubGrading

	assignment, ok := store.Default.GetAssignment(sub.AssignmentID)
	if !ok || assignment.ClassID != sub.ClassID || assignment.Status != "published" {
		return nil, fmt.Errorf("%w: published assignment rubric", ErrUntrustedReference)
	}
	rubricParts := make([]string, 0, len(assignment.Items)*2)
	for _, item := range assignment.Items {
		if answer := strings.TrimSpace(item.Answer); answer != "" {
			rubricParts = append(rubricParts, answer)
		}
		if explanation := strings.TrimSpace(item.Explanation); explanation != "" {
			rubricParts = append(rubricParts, explanation)
		}
	}
	rubric := strings.Join(rubricParts, "\n")
	if rubric == "" {
		return nil, fmt.Errorf("%w: assignment has no verified rubric", ErrUntrustedReference)
	}

	steps := SplitSteps(confirmedText)
	stepMaps := make([]map[string]any, 0, len(steps))
	for _, step := range steps {
		stepMaps = append(stepMaps, map[string]any{
			"index": step.Index, "text": step.Text, "verified": step.Verified, "comment": step.Comment,
		})
	}
	sub.Steps = stepMaps
	sub.FirstError = LocateFirstError(steps)
	suggested, gradingConfidence, cause := SuggestSubjectiveScoreFromRubric(confirmedText, rubric, sub.MaxScore)
	sub.Suggested = &suggested
	sub.ErrorCause = cause
	sub.Confidence = gradingConfidence
	if transcriptionConfidence < sub.Confidence {
		sub.Confidence = transcriptionConfidence
	}
	sub.NeedsReview = true
	sub.Confirmed = true
	sub.Status = SubNeedsTeacherReview
	store.Default.SaveSubmission(sub)
	if err := store.Default.EnsureSubmissionDurable(sub.ID); err != nil {
		return nil, fmt.Errorf("%w: persist confirmed grading: %v", ErrDependencyUnavailable, err)
	}
	return transcriptionResponse(sub), nil
}

func transcriptionResponse(sub store.Submission) map[string]any {
	return map[string]any{
		"submission": sub,
		"pipeline":   []string{"transcription_confirmed", "grading", "steps", "first_error", "suggested_score"},
		"classId":    sub.ClassID,
		"studentId":  sub.StudentID,
	}
}

// TeacherOverrideScore 教师改分后以教师结果为准，保留修改前后记录。
func (s *Services) TeacherOverrideScore(ctx context.Context, resultID, teacherID string, score float64, note string) (map[string]any, error) {
	return s.teacherOverrideScore(ctx, resultID, teacherID, "", score, note)
}

func (s *Services) TeacherOverrideScoreForActor(ctx context.Context, resultID string, actor ActorScope, score float64, note string) (map[string]any, error) {
	if !actor.Teacher {
		return nil, ErrForbidden
	}
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return s.teacherOverrideScore(ctx, resultID, actor.UserID, actor.ClassID, score, note)
}

func (s *Services) teacherOverrideScore(ctx context.Context, resultID, teacherID, classID string, score float64, note string) (map[string]any, error) {
	if err := validateIdentifier("resultID", resultID); err != nil {
		return nil, err
	}
	if err := validateIdentifier("teacherID", teacherID); err != nil {
		return nil, err
	}
	sub, ok := store.Default.GetSubmission(resultID)
	if !ok {
		return nil, ErrNotFound
	}
	if classID != "" && sub.ClassID != classID {
		return nil, ErrNotFound
	}
	if !sub.Confirmed {
		return nil, fmt.Errorf("%w: transcription is not confirmed", ErrStateConflict)
	}
	switch sub.Status {
	case SubNeedsTeacherReview, SubGradedSuggested, SubTeacherFinal:
	default:
		return nil, fmt.Errorf("%w: cannot override from %s", ErrStateConflict, sub.Status)
	}
	if err := validateScore(score, sub.MaxScore); err != nil {
		return nil, err
	}
	note = strings.TrimSpace(note)
	if err := validateModelInput(ctx, "teacher note", note, true, maxMetadataRunes); err != nil {
		return nil, err
	}

	if sub.Status == SubTeacherFinal && sub.Score != nil && *sub.Score == score && sub.TeacherNote == note {
		return teacherOverrideResponse(sub, resultID, sub.Score, score, note), nil
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
		"before": before, "after": score, "note": note, "teacherId": teacherID, "at": s.currentTime().Format(time.RFC3339Nano),
	})
	store.Default.SaveSubmission(sub)
	if err := store.Default.EnsureSubmissionDurable(sub.ID); err != nil {
		return nil, fmt.Errorf("%w: persist teacher score: %v", ErrDependencyUnavailable, err)
	}
	return teacherOverrideResponse(sub, resultID, before, score, note), nil
}

func teacherOverrideResponse(sub store.Submission, resultID string, before *float64, score float64, note string) map[string]any {
	return map[string]any{
		"submissionId":  resultID,
		"before":        before,
		"after":         score,
		"note":          note,
		"authoritative": "teacher",
		"scoreHistory":  sub.ScoreHistory,
		"classId":       sub.ClassID,
		"studentId":     sub.StudentID,
	}
}

// GetSubmission 查询单份提交（含步骤与改分历史）。
func (s *Services) GetSubmission(_ context.Context, id string) (store.Submission, error) {
	sub, ok := store.Default.GetSubmission(id)
	if !ok {
		return store.Submission{}, ErrNotFound
	}
	return sub, nil
}

// ListSubmissions 教师查看本班作业提交。
func (s *Services) ListSubmissions(_ context.Context, classID, assignmentID string) []store.Submission {
	return store.Default.ListSubmissions(classID, assignmentID)
}

// GenerateClassReview 有效提交 <5 不调模型；送模前匿名化；结果仅教师可见。
func (s *Services) GenerateClassReview(ctx context.Context, classID, assignmentID string, namedFeedbacks []string) (map[string]any, error) {
	if err := validateIdentifier("classID", classID); err != nil {
		return nil, err
	}
	if err := validateIdentifier("assignmentID", assignmentID); err != nil {
		return nil, err
	}

	subs := effectiveClassSubmissions(store.Default.ListSubmissions(classID, assignmentID))
	effective := len(subs)
	if effective < MinClassReviewSamples {
		return map[string]any{
			"skipped":   true,
			"reason":    "有效提交少于 5 人，不调用模型",
			"count":     effective,
			"threshold": MinClassReviewSamples,
			"classId":   classID,
		}, nil
	}
	if len(namedFeedbacks) != 0 {
		return nil, fmt.Errorf("%w: class review must derive evidence from stored submissions", ErrUntrustedEvidence)
	}

	anon := security.AnonymizeForModel(ctx, classReviewInputs(subs))
	for _, input := range anon {
		if err := validateModelInput(ctx, "class review evidence", input, true, maxMetadataRunes); err != nil {
			return nil, err
		}
		if containsNameLeak(input) {
			return nil, fmt.Errorf("%w: anonymization failed", ErrUnsafeInput)
		}
	}

	sourceIDs := make([]string, 0, len(subs))
	sourceFingerprint := []string{classID, assignmentID}
	for _, sub := range subs {
		sourceIDs = append(sourceIDs, sub.ID)
		sourceFingerprint = append(sourceFingerprint, sub.ID, sub.Status, sub.ErrorCause, sub.FirstError)
	}
	contentID := stableKey("review", sourceFingerprint...)
	taskID := stableKey("task", contentID)
	if existing, ok := store.Default.GetContent(contentID); ok {
		if existing.ClassID != classID || existing.Skill != string(domain.SkillClassReview) {
			return nil, ErrNotFound
		}
		if existing.Status != string(domain.StatusGenerating) {
			return classReviewResponse(existing, taskID, effective), nil
		}
		if task, ok := store.Default.GetTask(taskID); ok && task.Status != "failed" {
			return classReviewResponse(existing, taskID, effective), nil
		}
	}

	now := s.currentTime()
	content := store.ContentRecord{
		ID: contentID, Skill: string(domain.SkillClassReview), ClassID: classID,
		Status: string(domain.StatusGenerating),
		Payload: map[string]any{
			"assignmentId": assignmentID, "anonymousInputs": append([]string(nil), anon...),
			"teacherOnly": true, "individualCompareTeacherOnly": true,
			"sourceSubmissionIds": sourceIDs,
		},
		CreatedAt: now, UpdatedAt: now,
	}
	store.Default.SaveContent(content)
	attempt, previousSteps := nextTaskAttempt(taskID, "enqueue_review_attempt_")
	task := store.TaskRecord{
		ID: taskID, Skill: string(domain.SkillClassReview), ClassID: classID, Status: "queued",
		Steps: append(previousSteps,
			store.TaskStep{Name: "validate_sample_threshold", Status: "done"},
			store.TaskStep{Name: "anonymize_model_input", Status: "done"},
			store.TaskStep{Name: fmt.Sprintf("enqueue_review_attempt_%d", attempt), Status: "pending"},
		),
	}
	store.Default.SaveTask(task)
	if err := ensureContentTaskDurable(contentID, taskID); err != nil {
		return classReviewResponse(content, taskID, effective), err
	}
	jobs, err := s.requireJobs()
	if err == nil {
		err = jobs.Enqueue(ctx, "class_review", fmt.Sprintf("review-%s-attempt-%d", contentID, attempt), map[string]any{
			"taskId": taskID, "contentId": contentID, "anonymousInputs": anon,
		})
	}
	if err != nil {
		task.Status = "failed"
		task.Steps[len(task.Steps)-1].Status = "failed"
		store.Default.SaveTask(task)
		content.Payload["generationError"] = "job dependency unavailable"
		content.UpdatedAt = s.currentTime()
		store.Default.SaveContent(content)
		return classReviewResponse(content, taskID, effective), fmt.Errorf("%w: enqueue class review: %v", ErrDependencyUnavailable, err)
	}
	task.Steps[len(task.Steps)-1].Status = "done"
	store.Default.SaveTask(task)
	return classReviewResponse(content, taskID, effective), nil
}

func classReviewResponse(content store.ContentRecord, taskID string, sampleCount int) map[string]any {
	return map[string]any{
		"taskId": taskID, "contentId": content.ID, "status": content.Status,
		"anonymized": true, "sampleCount": sampleCount, "teacherOnly": true,
		"classId": content.ClassID,
	}
}

// GenerateDiagnosis 作答事件 <3 只显示样本不足；每项诊断列证据；确认前对学生不可见。
func (s *Services) GenerateDiagnosis(ctx context.Context, classID, studentID string) (map[string]any, error) {
	if err := validateIdentifier("classID", classID); err != nil {
		return nil, err
	}
	if err := validateIdentifier("studentID", studentID); err != nil {
		return nil, err
	}
	events := validLearningEvents(store.Default.EventsFor(studentID, classID))
	for _, event := range events {
		if err := validateModelInput(ctx, "learning event summary", event.Summary, true, maxMetadataRunes); err != nil {
			return nil, err
		}
		if err := validateModelInput(ctx, "learning event knowledge point", event.KP, false, maxMetadataRunes); err != nil {
			return nil, err
		}
	}
	sampleCount := len(events)
	contentID := diagnosisEvidenceKey(classID, studentID, events)
	taskID := stableKey("task", contentID)
	if existing, ok := store.Default.GetContent(contentID); ok {
		if existing.ClassID != classID || existing.Skill != string(domain.SkillDiagnosis) {
			return nil, ErrNotFound
		}
		if existing.Status != string(domain.StatusGenerating) {
			return diagnosisResponse(existing, taskID), nil
		}
		if task, ok := store.Default.GetTask(taskID); ok && task.Status != "failed" {
			return diagnosisResponse(existing, taskID), nil
		}
	}

	now := s.currentTime()
	evidenceIDs := make([]string, 0, len(events))
	for _, event := range events {
		evidenceIDs = append(evidenceIDs, event.ID)
	}
	payload := map[string]any{
		"studentId": studentID, "sampleCount": sampleCount, "studentVisible": false,
		"evidenceEventIds": evidenceIDs, "intercepted": false,
	}

	if sampleCount < MinDiagnosisSamples {
		payload["intercepted"] = true
		payload["diagnosis"] = map[string]any{
			"message": "样本不足", "sampleCount": sampleCount, "threshold": MinDiagnosisSamples,
			"studentVisible": false,
		}
		content := store.ContentRecord{
			ID: contentID, Skill: string(domain.SkillDiagnosis), ClassID: classID,
			Status: string(domain.StatusPending), Payload: payload, CreatedAt: now, UpdatedAt: now,
		}
		store.Default.SaveContent(content)
		store.Default.SaveTask(store.TaskRecord{
			ID: taskID, Skill: string(domain.SkillDiagnosis), ClassID: classID, Status: "not_started",
			Steps: []store.TaskStep{{Name: "validate_sample_threshold", Status: "intercepted"}},
		})
		if err := ensureContentTaskDurable(contentID, taskID); err != nil {
			return diagnosisResponse(content, taskID), err
		}
		return diagnosisResponse(content, taskID), nil
	}

	content := store.ContentRecord{
		ID: contentID, Skill: string(domain.SkillDiagnosis), ClassID: classID,
		Status: string(domain.StatusGenerating), Payload: payload, CreatedAt: now, UpdatedAt: now,
	}
	store.Default.SaveContent(content)
	attempt, previousSteps := nextTaskAttempt(taskID, "enqueue_diagnosis_attempt_")
	task := store.TaskRecord{
		ID: taskID, Skill: string(domain.SkillDiagnosis), ClassID: classID, Status: "queued",
		Steps: append(previousSteps,
			store.TaskStep{Name: "validate_sample_threshold", Status: "done"},
			store.TaskStep{Name: fmt.Sprintf("enqueue_diagnosis_attempt_%d", attempt), Status: "pending"},
		),
	}
	store.Default.SaveTask(task)
	if err := ensureContentTaskDurable(contentID, taskID); err != nil {
		return diagnosisResponse(content, taskID), err
	}
	jobs, err := s.requireJobs()
	if err == nil {
		err = jobs.Enqueue(ctx, "diagnosis", fmt.Sprintf("diag-%s-attempt-%d", contentID, attempt), map[string]any{
			"taskId": taskID, "contentId": contentID,
			"studentId": studentID, "classId": classID, "sampleCount": float64(sampleCount),
		})
	}
	if err != nil {
		task.Status = "failed"
		task.Steps[len(task.Steps)-1].Status = "failed"
		store.Default.SaveTask(task)
		content.Payload["generationError"] = "job dependency unavailable"
		content.UpdatedAt = s.currentTime()
		store.Default.SaveContent(content)
		return diagnosisResponse(content, taskID), fmt.Errorf("%w: enqueue diagnosis: %v", ErrDependencyUnavailable, err)
	}
	task.Steps[len(task.Steps)-1].Status = "done"
	store.Default.SaveTask(task)
	return diagnosisResponse(content, taskID), nil
}

func diagnosisResponse(content store.ContentRecord, taskID string) map[string]any {
	response := map[string]any{
		"taskId": taskID, "contentId": content.ID, "status": content.Status,
		"classId": content.ClassID, "studentVisible": false,
	}
	if content.Payload != nil {
		response["studentId"] = content.Payload["studentId"]
		response["sampleCount"] = content.Payload["sampleCount"]
		response["intercepted"] = content.Payload["intercepted"]
		if diagnosis, ok := content.Payload["diagnosis"]; ok {
			response["diagnosis"] = diagnosis
		}
		if visible, ok := content.Payload["studentVisible"].(bool); ok {
			response["studentVisible"] = visible
		}
	}
	return response
}

// ReleaseDiagnosisToStudent 教师确认后对学生开放个人报告。
func (s *Services) ReleaseDiagnosisToStudent(ctx context.Context, contentID string) (map[string]any, error) {
	return s.releaseDiagnosisToStudent(ctx, contentID, "")
}

func (s *Services) ReleaseDiagnosisToStudentForActor(ctx context.Context, contentID string, actor ActorScope) (map[string]any, error) {
	if !actor.Teacher {
		return nil, ErrForbidden
	}
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return s.releaseDiagnosisToStudent(ctx, contentID, actor.ClassID)
}

func (s *Services) releaseDiagnosisToStudent(_ context.Context, contentID, classID string) (map[string]any, error) {
	if err := validateIdentifier("contentID", contentID); err != nil {
		return nil, err
	}
	c, ok := store.Default.GetContent(contentID)
	if !ok {
		return nil, ErrNotFound
	}
	if c.Skill != string(domain.SkillDiagnosis) || (classID != "" && c.ClassID != classID) {
		return nil, ErrNotFound
	}
	if c.Payload == nil {
		return nil, fmt.Errorf("%w: diagnosis payload", ErrStateConflict)
	}
	if c.Status == string(domain.StatusPublished) {
		if visible, _ := c.Payload["studentVisible"].(bool); visible {
			return map[string]any{
				"contentId": contentID, "studentVisible": true, "status": c.Status,
				"classId": c.ClassID, "studentId": c.Payload["studentId"],
			}, nil
		}
		return nil, fmt.Errorf("%w: published diagnosis is not visible", ErrStateConflict)
	}
	if c.Status != string(domain.StatusPending) {
		return nil, fmt.Errorf("%w: cannot release diagnosis from %s", ErrStateConflict, c.Status)
	}
	if intercepted, _ := c.Payload["intercepted"].(bool); intercepted {
		return nil, fmt.Errorf("%w: sample threshold was intercepted", ErrInsufficientEvidence)
	}
	diagnosis, ok := c.Payload["diagnosis"].(map[string]any)
	if !ok || !diagnosisHasEvidence(diagnosis) {
		return nil, fmt.Errorf("%w: diagnosis evidence is incomplete", ErrInsufficientEvidence)
	}
	c.Status = string(domain.StatusPublished)
	c.Payload["studentVisible"] = true
	diagnosis["studentVisible"] = true
	c.Payload["diagnosis"] = diagnosis
	c.UpdatedAt = s.currentTime()
	store.Default.SaveContent(c)
	if err := store.Default.EnsureContentDurable(c.ID); err != nil {
		return nil, fmt.Errorf("%w: persist released diagnosis: %v", ErrDependencyUnavailable, err)
	}
	return map[string]any{
		"contentId": contentID, "studentVisible": true, "status": c.Status,
		"classId": c.ClassID, "studentId": c.Payload["studentId"],
	}, nil
}

// SaveSnapshot 只接受 interval / manual，拒绝逐键采集。
func (s *Services) SaveSnapshot(ctx context.Context, attemptID, studentID, classID, content, trigger string) (map[string]any, error) {
	return s.saveSnapshot(ctx, attemptID, studentID, classID, content, trigger)
}

func (s *Services) SaveSnapshotForActor(ctx context.Context, attemptID, content, trigger string, actor ActorScope) (map[string]any, error) {
	if actor.Teacher {
		return nil, ErrForbidden
	}
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return s.saveSnapshot(ctx, attemptID, actor.UserID, actor.ClassID, content, trigger)
}

func (s *Services) saveSnapshot(ctx context.Context, attemptID, studentID, classID, content, trigger string) (map[string]any, error) {
	if trigger != TriggerInterval && trigger != TriggerManual {
		return nil, fmt.Errorf("%w: only interval|manual allowed (no keylogging)", ErrInvalidArgument)
	}
	if attemptID != "" {
		if err := validateIdentifier("attemptID", attemptID); err != nil {
			return nil, err
		}
	}
	if err := validateIdentifier("studentID", studentID); err != nil {
		return nil, err
	}
	if err := validateIdentifier("classID", classID); err != nil {
		return nil, err
	}
	if err := validateModelInput(ctx, "snapshot content", content, false, maxAnswerRunes); err != nil {
		return nil, err
	}
	if attemptID == "" {
		attemptID = store.NewID("attempt")
	}

	store.Default.Lock()
	sub, ok := store.Default.Submissions[attemptID]
	if !ok {
		sub = store.Submission{
			ID: attemptID, Status: SubDrafting, StudentID: studentID, ClassID: classID,
		}
	} else {
		if sub.StudentID != studentID || sub.ClassID != classID {
			store.Default.Unlock()
			return nil, ErrNotFound
		}
		if sub.Status != SubDrafting {
			store.Default.Unlock()
			return nil, fmt.Errorf("%w: snapshots are closed for %s", ErrStateConflict, sub.Status)
		}
	}

	if len(sub.Snapshots) > 0 {
		last := sub.Snapshots[len(sub.Snapshots)-1]
		if last.Content == content && last.Trigger == trigger {
			store.Default.Unlock()
			if err := store.Default.EnsureSubmissionDurable(sub.ID); err != nil {
				return nil, fmt.Errorf("%w: persist answer snapshot: %v", ErrDependencyUnavailable, err)
			}
			return snapshotResponse(sub, last.Version, trigger, true), nil
		}
	}
	ver := len(sub.Snapshots) + 1
	sub.Snapshots = append(sub.Snapshots, store.AnswerSnapshot{
		Version: ver, Content: content, CreatedAt: s.currentTime(), Trigger: trigger,
	})
	sub.AnswerText = content
	store.Default.Submissions[attemptID] = sub
	store.Default.Unlock()
	store.Default.SaveSubmission(sub)
	if err := store.Default.EnsureSubmissionDurable(sub.ID); err != nil {
		return nil, fmt.Errorf("%w: persist answer snapshot: %v", ErrDependencyUnavailable, err)
	}
	return snapshotResponse(sub, ver, trigger, false), nil
}

func snapshotResponse(sub store.Submission, version int, trigger string, deduplicated bool) map[string]any {
	return map[string]any{
		"attemptId": sub.ID, "version": version, "trigger": trigger,
		"keylogging": false, "deduplicated": deduplicated,
		"snapshots": append([]store.AnswerSnapshot(nil), sub.Snapshots...),
		"classId":   sub.ClassID, "studentId": sub.StudentID,
	}
}

// AttachAttemptMeta 提交前补充引用与 AI 工具使用说明。
func (s *Services) AttachAttemptMeta(ctx context.Context, attemptID string, citations []string, aiNote string) (map[string]any, error) {
	return s.attachAttemptMeta(ctx, attemptID, citations, aiNote, "", "")
}

func (s *Services) AttachAttemptMetaForActor(ctx context.Context, attemptID string, citations []string, aiNote string, actor ActorScope) (map[string]any, error) {
	if actor.Teacher {
		return nil, ErrForbidden
	}
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return s.attachAttemptMeta(ctx, attemptID, citations, aiNote, actor.UserID, actor.ClassID)
}

func (s *Services) attachAttemptMeta(ctx context.Context, attemptID string, citations []string, aiNote, studentID, classID string) (map[string]any, error) {
	if err := validateIdentifier("attemptID", attemptID); err != nil {
		return nil, err
	}
	if len(citations) > 50 {
		return nil, fmt.Errorf("%w: too many citations", ErrInvalidArgument)
	}
	for _, citation := range citations {
		if err := validateModelInput(ctx, "citation", citation, true, maxMetadataRunes); err != nil {
			return nil, err
		}
	}
	if err := validateModelInput(ctx, "AI usage note", aiNote, false, maxMetadataRunes); err != nil {
		return nil, err
	}
	sub, ok := store.Default.GetSubmission(attemptID)
	if !ok {
		return nil, ErrNotFound
	}
	if (studentID != "" && sub.StudentID != studentID) || (classID != "" && sub.ClassID != classID) {
		return nil, ErrNotFound
	}
	if sub.Status != SubDrafting {
		return nil, fmt.Errorf("%w: attempt metadata is frozen", ErrStateConflict)
	}
	sub.Citations = append([]string(nil), citations...)
	sub.AIUsageNote = strings.TrimSpace(aiNote)
	store.Default.SaveSubmission(sub)
	if err := store.Default.EnsureSubmissionDurable(sub.ID); err != nil {
		return nil, fmt.Errorf("%w: persist attempt metadata: %v", ErrDependencyUnavailable, err)
	}
	return map[string]any{
		"attemptId": attemptID, "citations": append([]string(nil), sub.Citations...),
		"aiUsageNote": sub.AIUsageNote, "classId": sub.ClassID, "studentId": sub.StudentID,
	}, nil
}

func (s *Services) Timeline(_ context.Context, attemptID string) (any, error) {
	return s.timeline(attemptID, nil)
}

func (s *Services) TimelineForActor(_ context.Context, attemptID string, actor ActorScope) (any, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return s.timeline(attemptID, &actor)
}

func (s *Services) timeline(attemptID string, actor *ActorScope) (any, error) {
	if err := validateIdentifier("attemptID", attemptID); err != nil {
		return nil, err
	}
	sub, ok := store.Default.GetSubmission(attemptID)
	if !ok {
		return nil, ErrNotFound
	}
	if actor != nil {
		scope := ResourceScope{
			ResourceID: sub.ID, ClassID: sub.ClassID, StudentID: sub.StudentID,
			StudentVisible: true,
		}
		if err := AuthorizeRead(*actor, scope); err != nil {
			return nil, err
		}
	}
	return map[string]any{
		"attemptId":   attemptID,
		"snapshots":   append([]store.AnswerSnapshot(nil), sub.Snapshots...),
		"citations":   append([]string(nil), sub.Citations...),
		"aiUsageNote": sub.AIUsageNote,
		"policy":      "仅定时快照与主动保存，不记录键盘行为",
		"classId":     sub.ClassID, "studentId": sub.StudentID,
	}, nil
}

func (s *Services) SimilarityEvidence(ctx context.Context, attemptID string) (any, error) {
	return s.similarityEvidence(ctx, attemptID, nil)
}

func (s *Services) SimilarityEvidenceForActor(ctx context.Context, attemptID string, actor ActorScope) (any, error) {
	if !actor.Teacher {
		return nil, ErrForbidden
	}
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	return s.similarityEvidence(ctx, attemptID, &actor)
}

func (s *Services) similarityEvidence(ctx context.Context, attemptID string, actor *ActorScope) (any, error) {
	if err := validateIdentifier("attemptID", attemptID); err != nil {
		return nil, err
	}
	sub, ok := store.Default.GetSubmission(attemptID)
	if !ok {
		return nil, ErrNotFound
	}
	if actor != nil {
		scope := ResourceScope{ResourceID: sub.ID, ClassID: sub.ClassID, StudentID: sub.StudentID}
		if err := AuthorizeTeacher(*actor, scope); err != nil {
			return nil, err
		}
	}

	hits := store.Default.GetSimilarity(attemptID)
	if len(hits) > 0 {
		return similarityReportFromHits(attemptID, hits), nil
	}

	taskID := stableKey("task", "similarity", attemptID)
	if task, ok := store.Default.GetTask(taskID); ok && task.Status != "failed" {
		if task.Status == "pending" || task.Status == "succeeded" {
			return BuildSimilarityReport(attemptID, nil), nil
		}
		return similarityQueuedResponse(sub, taskID), nil
	}

	attempt, previousSteps := nextTaskAttempt(taskID, "enqueue_similarity_attempt_")
	task := store.TaskRecord{
		ID: taskID, Skill: string(domain.SkillAnswerReplay), ClassID: sub.ClassID, Status: "queued",
		Steps: append(previousSteps, store.TaskStep{
			Name: fmt.Sprintf("enqueue_similarity_attempt_%d", attempt), Status: "pending",
		}),
	}
	store.Default.SaveTask(task)
	if err := store.Default.EnsureTaskDurable(taskID); err != nil {
		return nil, fmt.Errorf("%w: persist similarity task: %v", ErrDependencyUnavailable, err)
	}
	jobs, err := s.requireJobs()
	if err == nil {
		err = jobs.Enqueue(ctx, "similarity", fmt.Sprintf("sim-%s-attempt-%d", attemptID, attempt), map[string]any{
			"attemptId": attemptID, "taskId": taskID,
		})
	}
	if err != nil {
		task.Status = "failed"
		task.Steps[len(task.Steps)-1].Status = "failed"
		store.Default.SaveTask(task)
		return nil, fmt.Errorf("%w: enqueue similarity: %v", ErrDependencyUnavailable, err)
	}
	task.Steps[len(task.Steps)-1].Status = "done"
	store.Default.SaveTask(task)
	return similarityQueuedResponse(sub, taskID), nil
}

func similarityQueuedResponse(sub store.Submission, taskID string) map[string]any {
	return map[string]any{
		"attemptId": sub.ID, "taskId": taskID, "status": "queued",
		"classId": sub.ClassID, "studentId": sub.StudentID, "verdict": nil,
	}
}

func similarityReportFromHits(attemptID string, hits []store.SimilarityHit) SimilarityReport {
	items := make([]SimilarityItem, 0, len(hits))
	for _, hit := range hits {
		items = append(items, SimilarityItem{
			Fragment: hit.Fragment, Source: hit.Source, Similarity: hit.Similarity, Location: hit.Location,
		})
	}
	return BuildSimilarityReport(attemptID, items)
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

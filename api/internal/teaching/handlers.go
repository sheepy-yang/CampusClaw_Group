package teaching

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"regexp"
	"strings"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/auth"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/security"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/teaching/modules/evaluation"
)

var teachingIdempotencyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{7,127}$`)

func evaluationActor(principal auth.Principal) evaluation.ActorScope {
	return evaluation.ActorScope{
		UserID: principal.UserID, ClassID: principal.ClassID,
		Teacher: principal.Role == auth.RoleTeacher,
	}
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeErr(w, r, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Content-Type must be application/json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeErr(w, r, http.StatusBadRequest, "INVALID_JSON", "invalid JSON request")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeErr(w, r, http.StatusBadRequest, "INVALID_JSON", "request must contain one JSON object")
		return false
	}
	return true
}

func (d *apiDeps) audit(p auth.Principal, action string, detail map[string]any) {
	skill := ""
	if strings.HasPrefix(action, "skill.") {
		skill = strings.TrimPrefix(action, "skill.")
	}
	if err := d.Audit.Log(context.Background(), security.AuditEvent{
		ActorID: p.UserID, ClassID: p.ClassID, Action: action,
		Object: "teaching", Result: "succeeded", Skill: skill, RequestID: "internal", Detail: detail,
	}); err != nil {
		log.Printf("teaching audit persistence failed action=%s actor=%s: %v", action, p.UserID, err)
	}
}

func requireSkill(w http.ResponseWriter, r *http.Request, p auth.Principal, skill SkillID) bool {
	if !store.Default.SkillEnabled(p.ClassID, string(skill)) {
		writeErr(w, r, http.StatusForbidden, "FORBIDDEN", "skill disabled")
		return false
	}
	return true
}

func writeEvaluationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, evaluation.ErrNotFound):
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
	case errors.Is(err, evaluation.ErrForbidden):
		writeErr(w, r, http.StatusForbidden, "FORBIDDEN", "forbidden")
	case errors.Is(err, evaluation.ErrDependencyUnavailable):
		writeErr(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "evaluation service is temporarily unavailable")
	case errors.Is(err, evaluation.ErrStateConflict),
		errors.Is(err, evaluation.ErrUntrustedEvidence),
		errors.Is(err, evaluation.ErrInsufficientEvidence):
		writeErr(w, r, http.StatusConflict, "CONFLICT", err.Error())
	case errors.Is(err, evaluation.ErrInvalidArgument),
		errors.Is(err, evaluation.ErrUnsafeInput),
		errors.Is(err, evaluation.ErrUntrustedReference):
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	default:
		writeErr(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "evaluation request failed")
	}
}

func handleMultimodalGrade(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	if !requireSkill(w, r, p, SkillMultimodalGrade) {
		return
	}
	if p.Role != auth.RoleStudent {
		writeErr(w, r, http.StatusForbidden, "FORBIDDEN", "student role required")
		return
	}
	var body struct {
		AssignmentID string `json:"assignmentId"`
		AnswerText   string `json:"answerText"`
		ImageDigest  string `json:"imageDigest"`
	}
	if !decode(w, r, &body) {
		return
	}
	assignment, ok := store.Default.GetAssignment(strings.TrimSpace(body.AssignmentID))
	if !ok || assignment.ClassID != p.ClassID || assignment.Status != "published" {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "published assignment not found")
		return
	}
	if body.ImageDigest != "" {
		if _, ok := store.Default.GetAttachment(body.ImageDigest, p.ClassID, p.UserID); !ok {
			writeErr(w, r, http.StatusNotFound, "ATTACHMENT_NOT_FOUND", "uploaded attachment not found")
			return
		}
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if !teachingIdempotencyPattern.MatchString(idempotencyKey) {
		writeErr(w, r, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key must contain 8 to 128 safe characters")
		return
	}
	maxScore := assignment.MaxScore
	if maxScore <= 0 {
		maxScore = 10
	}
	res, err := deps.Eval.EnqueueGradeWithKey(
		r.Context(), p.ClassID, p.UserID, assignment.ID,
		body.AnswerText, body.ImageDigest, maxScore, idempotencyKey,
	)
	if err != nil {
		writeEvaluationError(w, r, err)
		return
	}
	deps.audit(p, "skill.multimodal_grade", res)
	writeJSON(w, r, http.StatusAccepted, res)
}

func handleObjectiveGrade(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	if p.Role != auth.RoleStudent {
		writeErr(w, r, http.StatusForbidden, "FORBIDDEN", "student role required")
		return
	}
	var body struct {
		AssignmentID  string `json:"assignmentId"`
		QuestionID    string `json:"questionId"`
		StudentAnswer string `json:"studentAnswer"`
	}
	if !decode(w, r, &body) {
		return
	}
	result, err := deps.Eval.GradeObjectiveFromTrustedSource(
		r.Context(), p.ClassID, body.AssignmentID, body.QuestionID, body.StudentAnswer,
	)
	if err != nil {
		writeEvaluationError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, result)
}

func handleConfirmTranscription(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	if p.Role != auth.RoleStudent {
		writeErr(w, r, http.StatusForbidden, "FORBIDDEN", "student role required")
		return
	}
	var body struct {
		SubmissionID string `json:"submissionId"`
		Text         string `json:"text"`
	}
	if !decode(w, r, &body) {
		return
	}
	res, err := deps.Eval.ConfirmTranscriptionForActor(
		r.Context(), body.SubmissionID, body.Text, evaluationActor(p),
	)
	if err != nil {
		writeEvaluationError(w, r, err)
		return
	}
	deps.audit(p, "grade.confirm_transcription", map[string]any{"submissionId": body.SubmissionID})
	writeJSON(w, r, http.StatusOK, res)
}

func handleOverrideScore(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	var body struct {
		ResultID string  `json:"resultId"`
		Score    float64 `json:"score"`
		Note     string  `json:"note"`
	}
	if !decode(w, r, &body) {
		return
	}
	res, err := deps.Eval.TeacherOverrideScoreForActor(
		r.Context(), body.ResultID, evaluationActor(p), body.Score, body.Note,
	)
	if err != nil {
		writeEvaluationError(w, r, err)
		return
	}
	deps.audit(p, "grade.teacher_override", res)
	writeJSON(w, r, http.StatusOK, res)
}

func handleGetSubmission(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	id := r.PathValue("submissionID")
	sub, err := deps.Eval.GetSubmission(r.Context(), id)
	if err != nil {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	if p.Role == auth.RoleStudent && sub.StudentID != p.UserID {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	if p.ClassID != "" && sub.ClassID != "" && sub.ClassID != p.ClassID {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	writeJSON(w, r, http.StatusOK, sub)
}

func handleRetryMultimodalGrade(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	result, err := deps.Eval.RetryGradeForActor(
		r.Context(), r.PathValue("submissionID"), evaluationActor(p),
	)
	if err != nil {
		writeEvaluationError(w, r, err)
		return
	}
	deps.audit(p, "grade.retry", map[string]any{"submissionId": r.PathValue("submissionID")})
	writeJSON(w, r, http.StatusAccepted, result)
}

func handleListSubmissions(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	asg := r.URL.Query().Get("assignmentId")
	writeJSON(w, r, http.StatusOK, deps.Eval.ListSubmissions(r.Context(), p.ClassID, asg))
}

func handleClassReview(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	if !requireSkill(w, r, p, SkillClassReview) {
		return
	}
	var body struct {
		AssignmentID string `json:"assignmentId"`
	}
	if !decode(w, r, &body) {
		return
	}
	assignment, ok := store.Default.GetAssignment(strings.TrimSpace(body.AssignmentID))
	if !ok || assignment.ClassID != p.ClassID {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "assignment not found")
		return
	}
	res, err := deps.Eval.GenerateClassReview(r.Context(), p.ClassID, assignment.ID, nil)
	if err != nil {
		writeEvaluationError(w, r, err)
		return
	}
	deps.audit(p, "skill.class_review", res)
	writeJSON(w, r, http.StatusOK, res)
}

func handleDiagnosis(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	if !requireSkill(w, r, p, SkillDiagnosis) {
		return
	}
	var body struct {
		StudentID string `json:"studentId"`
	}
	if !decode(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.StudentID) == "" {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "studentId is required")
		return
	}
	res, err := deps.Eval.GenerateDiagnosis(r.Context(), p.ClassID, body.StudentID)
	if err != nil {
		writeEvaluationError(w, r, err)
		return
	}
	deps.audit(p, "skill.diagnosis", res)
	writeJSON(w, r, http.StatusOK, res)
}

func handleReleaseDiagnosis(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	id := r.PathValue("contentID")
	res, err := deps.Eval.ReleaseDiagnosisToStudentForActor(r.Context(), id, evaluationActor(p))
	if err != nil {
		writeEvaluationError(w, r, err)
		return
	}
	deps.audit(p, "diagnosis.release", res)
	writeJSON(w, r, http.StatusOK, res)
}

func handleSnapshot(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	if !requireSkill(w, r, p, SkillAnswerReplay) {
		return
	}
	var body struct {
		AttemptID string `json:"attemptId"`
		Content   string `json:"content"`
		Trigger   string `json:"trigger"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Trigger == "" {
		body.Trigger = "manual"
	}
	res, err := deps.Eval.SaveSnapshotForActor(
		r.Context(), body.AttemptID, body.Content, body.Trigger, evaluationActor(p),
	)
	if err != nil {
		writeEvaluationError(w, r, err)
		return
	}
	deps.audit(p, "answer.snapshot", res)
	writeJSON(w, r, http.StatusOK, res)
}

func handleAttemptMeta(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	var body struct {
		AttemptID   string   `json:"attemptId"`
		Citations   []string `json:"citations"`
		AIUsageNote string   `json:"aiUsageNote"`
	}
	if !decode(w, r, &body) {
		return
	}
	res, err := deps.Eval.AttachAttemptMetaForActor(
		r.Context(), body.AttemptID, body.Citations, body.AIUsageNote, evaluationActor(p),
	)
	if err != nil {
		writeEvaluationError(w, r, err)
		return
	}
	deps.audit(p, "answer.meta", map[string]any{"attemptId": body.AttemptID, "actor": p.UserID})
	writeJSON(w, r, http.StatusOK, res)
}

func handleTimeline(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	id := r.PathValue("attemptID")
	res, err := deps.Eval.TimelineForActor(r.Context(), id, evaluationActor(p))
	if err != nil {
		writeEvaluationError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, res)
}

func handleSimilarity(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	id := r.PathValue("attemptID")
	res, err := deps.Eval.SimilarityEvidenceForActor(r.Context(), id, evaluationActor(p))
	if err != nil {
		writeEvaluationError(w, r, err)
		return
	}
	deps.audit(p, "answer.similarity", map[string]any{"attemptId": id, "actor": p.UserID})
	writeJSON(w, r, http.StatusOK, res)
}

func handleListContents(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	classID := strings.TrimSpace(r.PathValue("classID"))
	if classID != p.ClassID {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	skill := strings.TrimSpace(r.URL.Query().Get("skill"))
	if skill == "" || len(skill) > 64 {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "a valid skill query is required")
		return
	}
	if p.Role == auth.RoleStudent && skill != string(SkillPreviewSheet) {
		writeErr(w, r, http.StatusForbidden, "FORBIDDEN", "students may only list published preview sheets")
		return
	}
	raw := store.Default.ListContents(classID, skill)
	out := make([]any, 0, len(raw))
	for _, item := range raw {
		if p.Role == auth.RoleStudent && item.Status != "published" {
			continue
		}
		if production, ok := item.Payload["contentProductionRecord"]; ok {
			out = append(out, production)
			continue
		}
		out = append(out, item)
	}
	writeJSON(w, r, http.StatusOK, out)
}

func handleConfirm(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	id := r.PathValue("id")
	if !teacherOwnsContent(w, r, p, id) {
		return
	}
	var body map[string]any
	if !decode(w, r, &body) {
		return
	}
	if err := ConfirmContent(id, body); err != nil {
		writeErr(w, r, http.StatusConflict, "CONFLICT", err.Error())
		return
	}
	deps.audit(p, "content.confirm", map[string]any{"contentId": id})
	writeJSON(w, r, http.StatusOK, map[string]any{"ok": true, "contentId": id, "status": StatusConfirmed})
}

func handlePublish(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	id := r.PathValue("id")
	if !teacherOwnsContent(w, r, p, id) {
		return
	}
	if err := PublishContent(id); err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	deps.audit(p, "content.publish", map[string]any{"contentId": id})
	writeJSON(w, r, http.StatusOK, map[string]any{"ok": true, "contentId": id, "status": StatusPublished, "frozen": true})
}

func handleReject(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	id := r.PathValue("id")
	if !teacherOwnsContent(w, r, p, id) {
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if !decode(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Reason) == "" || len([]rune(body.Reason)) > 1000 {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "reason is required and must not exceed 1000 characters")
		return
	}
	if err := RejectContent(id, body.Reason); err != nil {
		writeErr(w, r, http.StatusConflict, "CONFLICT", err.Error())
		return
	}
	deps.audit(p, "content.reject", map[string]any{"contentId": id})
	writeJSON(w, r, http.StatusOK, map[string]any{"ok": true, "status": StatusRejected})
}

func handleGetContent(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	id := r.PathValue("id")
	c, ok := store.Default.GetContent(id)
	if !ok || c.ClassID != p.ClassID {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	encoded, err := json.Marshal(c)
	if err != nil || json.Unmarshal(encoded, &c) != nil {
		writeErr(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "content could not be read")
		return
	}
	if p.Role == auth.RoleStudent {
		if c.Status != string(StatusPublished) {
			writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
			return
		}
		if SkillID(c.Skill) == SkillDiagnosis {
			studentID, _ := c.Payload["studentId"].(string)
			if studentID != p.UserID {
				writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
				return
			}
		}
		if SkillID(c.Skill) == SkillClassReview {
			delete(c.Payload, "anonymousInputs")
			delete(c.Payload, "teacherOnly")
			if review, ok := c.Payload["review"].(map[string]any); ok {
				delete(review, "anonymousInputs")
				delete(review, "teacherOnly")
				c.Payload["review"] = review
			}
		}
		if qs, ok := c.Payload["questions"].([]any); ok {
			for _, q := range qs {
				if m, ok := q.(map[string]any); ok {
					delete(m, "answer")
					delete(m, "explanation")
				}
			}
		}
	}
	writeJSON(w, r, http.StatusOK, c)
}

func teacherOwnsContent(w http.ResponseWriter, r *http.Request, principal auth.Principal, id string) bool {
	content, ok := store.Default.GetContent(id)
	if !ok || content.ClassID != principal.ClassID {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return false
	}
	switch SkillID(content.Skill) {
	case SkillClassReview, SkillVirtualClassroom:
	default:
		writeErr(w, r, http.StatusConflict, "CONFLICT", "resource does not use the publish lifecycle")
		return false
	}
	return true
}

func handleSetSkills(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	classID := r.PathValue("classID")
	if classID != p.ClassID {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	var body map[string]bool
	if !decode(w, r, &body) {
		return
	}
	allowed := map[string]bool{}
	for _, s := range AllSkills() {
		allowed[string(s)] = true
	}
	enabled := store.Default.GetSkills(classID)
	for k, v := range body {
		if !allowed[k] {
			writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "unknown skill: "+k)
			return
		}
		enabled[k] = v
	}
	store.Default.SetSkills(classID, enabled)
	deps.audit(p, "class.skills.update", map[string]any{"skills": body})
	writeJSON(w, r, http.StatusOK, store.Default.GetSkills(classID))
}

func handleGetSkills(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	classID := r.PathValue("classID")
	if classID != p.ClassID {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	writeJSON(w, r, http.StatusOK, store.Default.GetSkills(classID))
}

func handleGetTask(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	id := r.PathValue("taskID")
	t, ok := store.Default.GetTask(id)
	if !ok || t.ClassID != p.ClassID {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	if p.Role == auth.RoleStudent && t.UserID != "" && t.UserID != p.UserID {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	writeJSON(w, r, http.StatusOK, t)
}

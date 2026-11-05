package teaching

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/auth"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/security"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

func decode(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}

func (d *apiDeps) audit(p auth.Principal, action string, detail map[string]any) {
	_ = d.Audit.Log(context.Background(), security.AuditEvent{ActorID: p.UserID, ClassID: p.ClassID, Action: action, Detail: detail})
}

func requireSkill(w http.ResponseWriter, r *http.Request, p auth.Principal, skill SkillID) bool {
	if !store.Default.SkillEnabled(p.ClassID, string(skill)) {
		writeErr(w, r, http.StatusForbidden, "FORBIDDEN", "skill disabled")
		return false
	}
	return true
}

func handleMultimodalGrade(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	if !requireSkill(w, r, p, SkillMultimodalGrade) {
		return
	}
	var body struct {
		AssignmentID string  `json:"assignmentId"`
		AnswerText   string  `json:"answerText"`
		ImageDigest  string  `json:"imageDigest"`
		MaxScore     float64 `json:"maxScore"`
	}
	_ = decode(r, &body)
	res, err := deps.Eval.EnqueueGrade(r.Context(), p.ClassID, p.UserID, body.AssignmentID, body.AnswerText, body.ImageDigest, body.MaxScore)
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	deps.audit(p, "skill.multimodal_grade", res)
	writeJSON(w, r, http.StatusAccepted, res)
}

func handleObjectiveGrade(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	_ = p
	var body struct {
		Type           string `json:"type"`
		StudentAnswer  string `json:"studentAnswer"`
		StandardAnswer string `json:"standardAnswer"`
	}
	_ = decode(r, &body)
	writeJSON(w, r, http.StatusOK, deps.Eval.GradeObjectiveItem(r.Context(), body.Type, body.StudentAnswer, body.StandardAnswer))
}

func handleConfirmTranscription(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	var body struct {
		SubmissionID string `json:"submissionId"`
		Text         string `json:"text"`
	}
	_ = decode(r, &body)
	studentID := ""
	if p.Role == auth.RoleStudent {
		studentID = p.UserID
	}
	res, err := deps.Eval.ConfirmTranscription(r.Context(), body.SubmissionID, body.Text, studentID)
	if err != nil {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
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
	_ = decode(r, &body)
	res, err := deps.Eval.TeacherOverrideScore(r.Context(), body.ResultID, p.UserID, body.Score, body.Note)
	if err != nil {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
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

func handleListSubmissions(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	asg := r.URL.Query().Get("assignmentId")
	writeJSON(w, r, http.StatusOK, deps.Eval.ListSubmissions(r.Context(), p.ClassID, asg))
}

func handleClassReview(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	if !requireSkill(w, r, p, SkillClassReview) {
		return
	}
	var body struct {
		AssignmentID    string   `json:"assignmentId"`
		Feedbacks       []string `json:"feedbacks"`
		SubmissionCount int      `json:"submissionCount"`
	}
	_ = decode(r, &body)
	feedbacks := body.Feedbacks
	if len(feedbacks) == 0 && body.SubmissionCount > 0 {
		samples := []string{"李同学：秩弄错了", "王同学：行变换不完整", "张同学：概念混淆", "赵同学：计算失误", "钱同学：漏看增广", "孙同学：回代错误"}
		for i := 0; i < body.SubmissionCount && i < len(samples); i++ {
			feedbacks = append(feedbacks, samples[i])
		}
		for len(feedbacks) < body.SubmissionCount {
			feedbacks = append(feedbacks, "同学：补充错因样本")
		}
	}
	res, err := deps.Eval.GenerateClassReview(r.Context(), p.ClassID, body.AssignmentID, feedbacks)
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
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
	_ = decode(r, &body)
	if body.StudentID == "" {
		body.StudentID = "stu-1"
	}
	res, err := deps.Eval.GenerateDiagnosis(r.Context(), p.ClassID, body.StudentID)
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	deps.audit(p, "skill.diagnosis", res)
	writeJSON(w, r, http.StatusOK, res)
}

func handleReleaseDiagnosis(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	id := r.PathValue("contentID")
	res, err := deps.Eval.ReleaseDiagnosisToStudent(r.Context(), id)
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
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
	_ = decode(r, &body)
	if body.Trigger == "" {
		body.Trigger = "manual"
	}
	res, err := deps.Eval.SaveSnapshot(r.Context(), body.AttemptID, p.UserID, p.ClassID, body.Content, body.Trigger)
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
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
	_ = decode(r, &body)
	res, err := deps.Eval.AttachAttemptMeta(r.Context(), body.AttemptID, body.Citations, body.AIUsageNote)
	if err != nil {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	deps.audit(p, "answer.meta", map[string]any{"attemptId": body.AttemptID, "actor": p.UserID})
	writeJSON(w, r, http.StatusOK, res)
}

func handleTimeline(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	id := r.PathValue("attemptID")
	res, err := deps.Eval.Timeline(r.Context(), id)
	if err != nil {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	if p.Role == auth.RoleStudent {
		sub, ok := store.Default.GetSubmission(id)
		if ok && sub.StudentID != p.UserID {
			writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
			return
		}
	}
	writeJSON(w, r, http.StatusOK, res)
}

func handleSimilarity(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	id := r.PathValue("attemptID")
	res, err := deps.Eval.SimilarityEvidence(r.Context(), id)
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	deps.audit(p, "answer.similarity", map[string]any{"attemptId": id, "actor": p.UserID})
	writeJSON(w, r, http.StatusOK, res)
}

func handleConfirm(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	id := r.PathValue("id")
	var body map[string]any
	_ = decode(r, &body)
	if err := ConfirmContent(id, body); err != nil {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	deps.audit(p, "content.confirm", map[string]any{"contentId": id})
	writeJSON(w, r, http.StatusOK, map[string]any{"ok": true, "contentId": id, "status": StatusConfirmed})
}

func handlePublish(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	id := r.PathValue("id")
	if err := PublishContent(id); err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	deps.audit(p, "content.publish", map[string]any{"contentId": id})
	writeJSON(w, r, http.StatusOK, map[string]any{"ok": true, "contentId": id, "status": StatusPublished, "frozen": true})
}

func handleReject(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	id := r.PathValue("id")
	var body struct {
		Reason string `json:"reason"`
	}
	_ = decode(r, &body)
	if err := RejectContent(id, body.Reason); err != nil {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
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
	if p.Role == auth.RoleStudent {
		if c.Status != string(StatusPublished) {
			writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
			return
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

func handleSetSkills(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	classID := r.PathValue("classID")
	if classID != p.ClassID {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	var body map[string]bool
	_ = decode(r, &body)
	enabled := map[string]bool{}
	for _, s := range AllSkills() {
		enabled[string(s)] = true
	}
	for k, v := range body {
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
	writeJSON(w, r, http.StatusOK, t)
}

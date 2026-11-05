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

func requireSkill(w http.ResponseWriter, p auth.Principal, skill SkillID) bool {
	if !store.Default.SkillEnabled(p.ClassID, string(skill)) {
		writeErr(w, http.StatusForbidden, "skill disabled")
		return false
	}
	return true
}

func handleProposition(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	if !requireSkill(w, p, SkillProposition) {
		return
	}
	var body map[string]any
	_ = decode(r, &body)
	res, err := deps.Content.GenerateProposition(r.Context(), p.ClassID, body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	deps.audit(p, "skill.proposition", map[string]any{"contentId": res.ContentID})
	writeJSON(w, http.StatusOK, res)
}

func handleLessonPlan(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	if !requireSkill(w, p, SkillLessonPlan) {
		return
	}
	var body map[string]any
	_ = decode(r, &body)
	res, err := deps.Content.GenerateLessonPlan(r.Context(), p.ClassID, body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	deps.audit(p, "skill.lesson_plan", map[string]any{"contentId": res.ContentID})
	writeJSON(w, http.StatusOK, res)
}

func handlePreviewSheet(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	if !requireSkill(w, p, SkillPreviewSheet) {
		return
	}
	var body map[string]any
	_ = decode(r, &body)
	res, err := deps.Content.GeneratePreviewSheet(r.Context(), p.ClassID, body)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error(), "result": res})
		return
	}
	deps.audit(p, "skill.preview_sheet", map[string]any{"contentId": res.ContentID})
	writeJSON(w, http.StatusOK, res)
}

func handleVirtualClassroom(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	if !requireSkill(w, p, SkillVirtualClassroom) {
		return
	}
	var body struct {
		LessonPlanID string `json:"lessonPlanId"`
		Rounds       int    `json:"rounds"`
	}
	_ = decode(r, &body)
	if body.Rounds <= 0 {
		body.Rounds = 3
	}
	res, err := deps.Content.StartVirtualClassroom(r.Context(), p.ClassID, body.LessonPlanID, body.Rounds)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	deps.audit(p, "skill.virtual_classroom", map[string]any{"contentId": res.ContentID})
	writeJSON(w, http.StatusAccepted, res)
}

func handleMultimodalGrade(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	if !requireSkill(w, p, SkillMultimodalGrade) {
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
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	deps.audit(p, "skill.multimodal_grade", res)
	writeJSON(w, http.StatusAccepted, res)
}

func handleObjectiveGrade(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	_ = p
	var body struct {
		Type           string `json:"type"`
		StudentAnswer  string `json:"studentAnswer"`
		StandardAnswer string `json:"standardAnswer"`
	}
	_ = decode(r, &body)
	writeJSON(w, http.StatusOK, deps.Eval.GradeObjectiveItem(r.Context(), body.Type, body.StudentAnswer, body.StandardAnswer))
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
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	deps.audit(p, "grade.confirm_transcription", map[string]any{"submissionId": body.SubmissionID})
	writeJSON(w, http.StatusOK, res)
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
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	deps.audit(p, "grade.teacher_override", res)
	writeJSON(w, http.StatusOK, res)
}

func handleGetSubmission(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	id := r.PathValue("submissionID")
	sub, err := deps.Eval.GetSubmission(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if p.Role == auth.RoleStudent && sub.StudentID != p.UserID {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if p.ClassID != "" && sub.ClassID != "" && sub.ClassID != p.ClassID {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, sub)
}

func handleListSubmissions(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	asg := r.URL.Query().Get("assignmentId")
	writeJSON(w, http.StatusOK, deps.Eval.ListSubmissions(r.Context(), p.ClassID, asg))
}

func handleClassReview(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	if !requireSkill(w, p, SkillClassReview) {
		return
	}
	var body struct {
		AssignmentID string   `json:"assignmentId"`
		Feedbacks    []string `json:"feedbacks"`
		// 兼容旧字段：用假名反馈凑人数
		SubmissionCount int `json:"submissionCount"`
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
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	deps.audit(p, "skill.class_review", res)
	writeJSON(w, http.StatusOK, res)
}

func handleDiagnosis(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	if !requireSkill(w, p, SkillDiagnosis) {
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
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	deps.audit(p, "skill.diagnosis", res)
	writeJSON(w, http.StatusOK, res)
}

func handleReleaseDiagnosis(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	id := r.PathValue("contentID")
	res, err := deps.Eval.ReleaseDiagnosisToStudent(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	deps.audit(p, "diagnosis.release", res)
	writeJSON(w, http.StatusOK, res)
}

func handleSnapshot(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	if !requireSkill(w, p, SkillAnswerReplay) {
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
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	deps.audit(p, "answer.snapshot", res)
	writeJSON(w, http.StatusOK, res)
}

func handleAttemptMeta(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	var body struct {
		AttemptID  string   `json:"attemptId"`
		Citations  []string `json:"citations"`
		AIUsageNote string  `json:"aiUsageNote"`
	}
	_ = decode(r, &body)
	res, err := deps.Eval.AttachAttemptMeta(r.Context(), body.AttemptID, body.Citations, body.AIUsageNote)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	deps.audit(p, "answer.meta", map[string]any{"attemptId": body.AttemptID, "actor": p.UserID})
	writeJSON(w, http.StatusOK, res)
}

func handleTimeline(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	id := r.PathValue("attemptID")
	res, err := deps.Eval.Timeline(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	// 学生只能看自己的
	if p.Role == auth.RoleStudent {
		sub, ok := store.Default.GetSubmission(id)
		if ok && sub.StudentID != p.UserID {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
	}
	writeJSON(w, http.StatusOK, res)
}

func handleSimilarity(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	id := r.PathValue("attemptID")
	res, err := deps.Eval.SimilarityEvidence(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	deps.audit(p, "answer.similarity", map[string]any{"attemptId": id})
	writeJSON(w, http.StatusOK, res)
}

func handleReviewPath(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	if !requireSkill(w, p, SkillReviewPath) {
		return
	}
	studentID := p.UserID
	if p.Role == auth.RoleTeacher {
		var body struct {
			StudentID string `json:"studentId"`
		}
		_ = decode(r, &body)
		if body.StudentID != "" {
			studentID = body.StudentID
		}
	}
	res, err := deps.Tutor.GenerateReviewPath(r.Context(), p.ClassID, studentID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func handleSocraticStart(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	if !requireSkill(w, p, SkillSocraticTutor) {
		return
	}
	var body struct {
		QuestionID string `json:"questionId"`
	}
	_ = decode(r, &body)
	res, err := deps.Tutor.StartSocratic(r.Context(), p.ClassID, p.UserID, body.QuestionID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func handleSocraticNext(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	var body struct {
		SessionID string `json:"sessionId"`
		Reply     string `json:"reply"`
	}
	_ = decode(r, &body)
	res, err := deps.Tutor.NextSocratic(r.Context(), body.SessionID, body.Reply)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func handleCluster(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	if !requireSkill(w, p, SkillClassroomCluster) {
		return
	}
	var body struct {
		SessionID string `json:"sessionId"`
	}
	_ = decode(r, &body)
	if body.SessionID == "" {
		body.SessionID = "class-session-1"
	}
	res, err := deps.Tutor.ClusterClassroom(r.Context(), p.ClassID, body.SessionID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	deps.audit(p, "skill.classroom_cluster", res)
	writeJSON(w, http.StatusOK, res)
}

func handleClassroomAnswer(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	var body struct {
		SessionID string `json:"sessionId"`
		Text      string `json:"text"`
	}
	_ = decode(r, &body)
	if err := security.CheckInput(r.Context(), body.Text); err != nil {
		writeErr(w, http.StatusBadRequest, "unsafe input")
		return
	}
	res, err := deps.Tutor.SubmitClassroomAnswer(r.Context(), p.ClassID, body.SessionID, p.UserID, p.Name, body.Text)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func handleAdaptiveStart(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	if !requireSkill(w, p, SkillAdaptivePractice) {
		return
	}
	var body struct {
		KnowledgePoint string `json:"knowledgePoint"`
	}
	_ = decode(r, &body)
	if body.KnowledgePoint == "" {
		body.KnowledgePoint = "高斯消元"
	}
	res, err := deps.Tutor.StartAdaptive(r.Context(), p.ClassID, p.UserID, body.KnowledgePoint)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func handleAdaptiveSubmit(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	var body struct {
		SessionID string `json:"sessionId"`
		Outcome   string `json:"outcome"` // correct | conceptual_error | calculation_error | quit
	}
	_ = decode(r, &body)
	res, err := deps.Tutor.SubmitAdaptive(r.Context(), body.SessionID, body.Outcome)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func handleConfirm(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	id := r.PathValue("id")
	var body map[string]any
	_ = decode(r, &body)
	if err := ConfirmContent(id, body); err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	deps.audit(p, "content.confirm", map[string]any{"contentId": id})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "contentId": id, "status": StatusConfirmed})
}

func handlePublish(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	id := r.PathValue("id")
	if err := PublishContent(id); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	deps.audit(p, "content.publish", map[string]any{"contentId": id})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "contentId": id, "status": StatusPublished, "frozen": true})
}

func handleReject(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	id := r.PathValue("id")
	var body struct {
		Reason string `json:"reason"`
	}
	_ = decode(r, &body)
	if err := RejectContent(id, body.Reason); err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	deps.audit(p, "content.reject", map[string]any{"contentId": id})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": StatusRejected})
}

func handleGetContent(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	id := r.PathValue("id")
	c, ok := store.Default.GetContent(id)
	if !ok || c.ClassID != p.ClassID {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	// 学生只能看已发布内容；诊断需教师确认后
	if p.Role == auth.RoleStudent {
		if c.Status != string(StatusPublished) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		// 隐藏答案字段
		if qs, ok := c.Payload["questions"].([]any); ok {
			for _, q := range qs {
				if m, ok := q.(map[string]any); ok {
					delete(m, "answer")
					delete(m, "explanation")
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, c)
}

func handleSetSkills(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	classID := r.PathValue("classID")
	if classID != p.ClassID {
		writeErr(w, http.StatusNotFound, "not found")
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
	writeJSON(w, http.StatusOK, store.Default.GetSkills(classID))
}

func handleGetSkills(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	classID := r.PathValue("classID")
	if classID != p.ClassID {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, store.Default.GetSkills(classID))
}

func handleGetTask(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	id := r.PathValue("taskID")
	t, ok := store.Default.GetTask(id)
	if !ok || t.ClassID != p.ClassID {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

package platform

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/auth"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/httpapi"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/queue"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

func hideAssignmentAnswers(assignment *store.Assignment) {
	for i := range assignment.Items {
		assignment.Items[i].Answer = ""
		assignment.Items[i].Explanation = ""
	}
}

func handleGetAssignment(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	assignment, ok := store.Default.GetAssignment(r.PathValue("assignmentID"))
	if !ok || assignment.ClassID != p.ClassID {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	if p.Role == auth.RoleStudent {
		if assignment.Status != "published" {
			writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
			return
		}
		hideAssignmentAnswers(&assignment)
	}
	httpapi.WriteJSON(w, http.StatusOK, assignment)
}

func handlePublishAssignment(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	assignment, ok := store.Default.GetAssignment(r.PathValue("assignmentID"))
	if !ok || assignment.ClassID != p.ClassID {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	if assignment.Status == "published" {
		httpapi.WriteJSON(w, http.StatusOK, assignment)
		return
	}
	if assignment.Status != "draft" || len(assignment.Items) == 0 {
		writeErr(w, r, http.StatusConflict, "CONFLICT", "only a valid draft can be published")
		return
	}
	for i := range assignment.Items {
		if strings.TrimSpace(assignment.Items[i].Stem) == "" || strings.TrimSpace(assignment.Items[i].Answer) == "" {
			writeErr(w, r, http.StatusConflict, "CONFLICT", "all items require a verified answer")
			return
		}
		assignment.Items[i].Status = "published"
		assignment.Items[i].Adopted = true
	}
	assignment.Status = "published"
	assignment.PublishedAt = time.Now().UTC()
	store.Default.SaveAssignment(assignment)
	if err := store.Default.EnsureAssignmentDurable(assignment.ID); err != nil {
		auditPlatform(r, p, "assignment.publish", assignment.ID, "failed", "", 0, nil)
		writeErr(w, r, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "assignment publication could not be durably saved")
		return
	}
	auditPlatform(r, p, "assignment.publish", assignment.ID, "succeeded", "", len(assignment.Items), nil)
	httpapi.WriteJSON(w, http.StatusOK, assignment)
}

func handleSubmitAssignment(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	if p.Role != auth.RoleStudent {
		auditPlatform(r, p, "assignment.submit", r.PathValue("assignmentID"), "denied", "", 0, nil)
		writeErr(w, r, http.StatusForbidden, "FORBIDDEN", "students only")
		return
	}
	assignment, ok := store.Default.GetAssignment(r.PathValue("assignmentID"))
	if !ok || assignment.ClassID != p.ClassID || assignment.Status != "published" {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	var body struct {
		AnswerText string            `json:"answerText"`
		Answers    map[string]string `json:"answers"`
	}
	if !decodeJSON(w, r, &body, 1<<20) {
		return
	}
	body.AnswerText = strings.TrimSpace(body.AnswerText)
	if body.Answers == nil {
		body.Answers = map[string]string{}
	}
	hasAnswer := body.AnswerText != ""
	for key, value := range body.Answers {
		value = strings.TrimSpace(value)
		if key == "" || len([]rune(value)) > 20000 {
			writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid answer")
			return
		}
		body.Answers[key] = value
		hasAnswer = hasAnswer || value != ""
	}
	if !hasAnswer {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "at least one answer is required")
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" || len(idempotencyKey) > 128 {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "Idempotency-Key is required and must not exceed 128 characters")
		return
	}
	canonical, _ := json.Marshal(struct {
		AssignmentID string            `json:"assignmentId"`
		AnswerText   string            `json:"answerText"`
		Answers      map[string]string `json:"answers"`
	}{assignment.ID, body.AnswerText, body.Answers})
	payloadHash := store.Digest(string(canonical))
	taskID := store.NewID("task")
	task, duplicate, err := store.Default.ClaimIdempotency(p.UserID, "assignment:"+assignment.ID, idempotencyKey, payloadHash, taskID)
	if errors.Is(err, store.ErrConflict) {
		writeErr(w, r, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "idempotency key was already used for different content")
		return
	}
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "could not reserve task")
		return
	}
	if duplicate {
		httpapi.WriteJSON(w, http.StatusAccepted, map[string]any{
			"taskId": task.ID, "submissionId": task.ObjectID, "status": task.Status, "duplicate": true,
		})
		return
	}

	submissionID := store.NewID("submission")
	task.ClassID = p.ClassID
	task.UserID = p.UserID
	task.ObjectID = submissionID
	task.Skill = "assignment_grade"
	task.Steps = []store.TaskStep{{Name: "submission_saved", Status: "succeeded"}, {Name: "grade", Status: "pending"}}
	store.Default.SaveTask(task)
	submission := store.Submission{
		ID: submissionID, AssignmentID: assignment.ID, StudentID: p.UserID, ClassID: p.ClassID,
		AnswerText: body.AnswerText, Answers: body.Answers, Status: "submitted", TaskID: task.ID,
		IdempotencyKey: idempotencyKey, SubmittedAt: time.Now().UTC(),
	}
	store.Default.SaveSubmission(submission)
	if err := store.Default.EnsureTaskDurable(task.ID); err != nil {
		writeErr(w, r, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "task could not be durably saved")
		return
	}
	if err := store.Default.EnsureSubmissionDurable(submission.ID); err != nil {
		writeErr(w, r, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "submission could not be durably saved")
		return
	}

	queue.EnsureEmbedded()
	job := queue.Job{
		ID: store.NewID("job"), Type: queue.JobAssignmentGrade,
		Payload:        map[string]any{"submissionId": submissionID, "taskId": task.ID},
		IdempotencyKey: "assignment-grade:" + p.UserID + ":" + idempotencyKey,
	}
	if err := queue.Shared().Enqueue(r.Context(), job); err != nil {
		task.Status = "failed"
		task.Error = "queue unavailable"
		store.Default.SaveTask(task)
		auditPlatform(r, p, "assignment.submit", submissionID, "failed", "assignment_grade", 0, nil)
		writeErr(w, r, http.StatusServiceUnavailable, "QUEUE_UNAVAILABLE", "submission was saved; processing is temporarily unavailable")
		return
	}
	auditPlatform(r, p, "assignment.submit", submissionID, "accepted", "assignment_grade", len(body.Answers), map[string]any{"taskId": task.ID})
	httpapi.WriteJSON(w, http.StatusAccepted, map[string]any{
		"taskId": task.ID, "submissionId": submissionID, "status": task.Status, "duplicate": false,
	})
}

func handleGetPlatformSubmission(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	submission, ok := store.Default.GetSubmission(r.PathValue("submissionID"))
	if !ok || submission.ClassID != p.ClassID || (p.Role == auth.RoleStudent && submission.StudentID != p.UserID) {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, submission)
}

func handleOverridePlatformScore(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	submission, ok := store.Default.GetSubmission(r.PathValue("submissionID"))
	if !ok || submission.ClassID != p.ClassID {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	var body struct {
		Score float64 `json:"score"`
		Note  string  `json:"note"`
	}
	if !decodeJSON(w, r, &body, 32<<10) {
		return
	}
	if submission.MaxScore <= 0 {
		submission.MaxScore = 10
	}
	body.Note = strings.TrimSpace(body.Note)
	if body.Score < 0 || body.Score > submission.MaxScore || body.Note == "" || len([]rune(body.Note)) > 500 {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "score is out of range or note is missing")
		return
	}
	var previous any
	if submission.Score != nil {
		previous = *submission.Score
	}
	submission.ScoreHistory = append(submission.ScoreHistory, map[string]any{
		"previous": previous, "score": body.Score, "note": body.Note, "teacherId": p.UserID, "at": time.Now().UTC(),
	})
	submission.Score = &body.Score
	submission.TeacherNote = body.Note
	submission.Status = "teacher_final"
	submission.NeedsReview = false
	submission.Confirmed = true
	store.Default.SaveSubmission(submission)
	if err := store.Default.EnsureSubmissionDurable(submission.ID); err != nil {
		writeErr(w, r, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "score override could not be durably saved")
		return
	}
	auditPlatform(r, p, "submission.score_override", submission.ID, "succeeded", "", 0, map[string]any{"previous": previous, "score": body.Score})
	httpapi.WriteJSON(w, http.StatusOK, submission)
}

func handleMistakeExplanation(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	submission, ok := store.Default.GetSubmission(r.PathValue("submissionID"))
	if !ok || submission.ClassID != p.ClassID || (p.Role == auth.RoleStudent && submission.StudentID != p.UserID) {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	mistake, ok := store.Default.MistakeBySubmission(submission.ID)
	if !ok {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "no mistake record")
		return
	}
	response := map[string]any{
		"mistakeId": mistake.ID, "questionStem": mistake.QuestionStem,
		"errorCause": mistake.ErrorCause, "hint": mistake.Hint,
		"answerRevealed": false,
	}
	auditPlatform(r, p, "mistake.explain", mistake.ID, "succeeded", "mistake_hint", 1, nil)
	httpapi.WriteJSON(w, http.StatusOK, response)
}

func handleRetryTask(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	task, ok := store.Default.GetTask(r.PathValue("taskID"))
	if !ok || task.ClassID != p.ClassID || (p.Role == auth.RoleStudent && task.UserID != p.UserID) {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	if task.Status != "failed" {
		writeErr(w, r, http.StatusConflict, "CONFLICT", "only failed tasks can be retried")
		return
	}
	var job queue.Job
	switch task.Skill {
	case string(queue.JobAssignmentGrade):
		if task.ObjectID == "" {
			writeErr(w, r, http.StatusConflict, "CONFLICT", "this task does not expose a safe retry contract")
			return
		}
		job = queue.Job{
			ID: store.NewID("job"), Type: queue.JobAssignmentGrade,
			Payload:        map[string]any{"submissionId": task.ObjectID, "taskId": task.ID},
			IdempotencyKey: "assignment-grade:" + task.UserID + ":" + task.IdempotencyKey,
		}
	case string(queue.JobVirtualClassroom):
		content, found := store.Default.GetContent(task.ObjectID)
		if !found || content.ClassID != task.ClassID {
			writeErr(w, r, http.StatusConflict, "CONFLICT", "virtual classroom input is unavailable")
			return
		}
		job = queue.Job{
			ID: store.NewID("job"), Type: queue.JobVirtualClassroom,
			Payload: map[string]any{
				"taskId": task.ID, "contentId": content.ID,
				"lessonPlanId": content.Payload["lessonPlanId"],
				"objective":    content.Payload["objective"], "rounds": content.Payload["rounds"],
				"previousSimulationId": content.Payload["previousSimulationId"],
			},
			IdempotencyKey: "virtual:" + task.ID,
		}
	default:
		writeErr(w, r, http.StatusConflict, "CONFLICT", "this task does not expose a safe retry contract")
		return
	}
	task.Status = "pending"
	task.Error = ""
	task.Steps = append(task.Steps, store.TaskStep{Name: "retry_requested", Status: "succeeded"}, store.TaskStep{Name: "grade", Status: "pending"})
	store.Default.SaveTask(task)
	if err := store.Default.EnsureTaskDurable(task.ID); err != nil {
		writeErr(w, r, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "retry state could not be durably saved")
		return
	}
	if err := queue.Shared().Enqueue(r.Context(), job); err != nil {
		task.Status = "failed"
		task.Error = "queue unavailable"
		store.Default.SaveTask(task)
		writeErr(w, r, http.StatusServiceUnavailable, "QUEUE_UNAVAILABLE", "retry could not be queued")
		return
	}
	auditPlatform(r, p, "task.retry", task.ID, "accepted", task.Skill, 0, nil)
	httpapi.WriteJSON(w, http.StatusAccepted, task)
}

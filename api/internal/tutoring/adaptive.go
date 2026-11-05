package tutoring

import (
	"context"
	"strconv"
	"strings"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/auth"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/security"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

func (s *Service) StartPractice(ctx context.Context, principal auth.Principal, knowledgePoint string) (PracticeSession, error) {
	if principal.Role != auth.RoleStudent {
		return PracticeSession{}, forbidden("adaptive practice is student-only")
	}
	if !s.repository.skillEnabled(principal.ClassID, "adaptive_practice") {
		return PracticeSession{}, forbidden("adaptive practice is disabled for this class")
	}
	knowledgePoint = strings.TrimSpace(knowledgePoint)
	if runeLength(knowledgePoint) < 1 || runeLength(knowledgePoint) > 100 {
		return PracticeSession{}, invalid("knowledgePoint must contain 1 to 100 characters")
	}
	if err := security.CheckInput(ctx, knowledgePoint); err != nil {
		return PracticeSession{}, unsafeInput(err)
	}
	question, ok := s.repository.selectPracticeQuestion(principal.ClassID, knowledgePoint, "medium", 0)
	if !ok {
		s.repository.importPublishedPracticeQuestions(principal.ClassID, knowledgePoint)
		question, ok = s.repository.selectPracticeQuestion(principal.ClassID, knowledgePoint, "medium", 0)
	}
	if !ok {
		return PracticeSession{}, notFound()
	}

	now := s.now().UTC()
	public := publicQuestion(question)
	session := PracticeSession{
		ID:              s.repository.newID("practice"),
		StudentID:       principal.UserID,
		ClassID:         principal.ClassID,
		KnowledgePoint:  knowledgePoint,
		Difficulty:      "medium",
		CurrentQuestion: question,
		Question:        &public,
		Submissions:     make(map[string]PracticeSubmission),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.repository.savePractice(session); err != nil {
		return PracticeSession{}, conflict("practice session changed concurrently")
	}
	s.log(ctx, principal, "tutoring.practice.started", map[string]any{
		"sessionId": session.ID, "knowledgePoint": knowledgePoint,
	})
	return clonePracticeSession(session), nil
}

func (s *Service) SubmitPractice(ctx context.Context, principal auth.Principal, sessionID, submissionID, answerText string) (PracticeResult, error) {
	if principal.Role != auth.RoleStudent {
		return PracticeResult{}, forbidden("adaptive practice is student-only")
	}
	if !s.repository.skillEnabled(principal.ClassID, "adaptive_practice") {
		return PracticeResult{}, forbidden("adaptive practice is disabled for this class")
	}
	sessionID = strings.TrimSpace(sessionID)
	submissionID = strings.TrimSpace(submissionID)
	answerText = strings.TrimSpace(answerText)
	if sessionID == "" || submissionID == "" || runeLength(submissionID) > 128 {
		return PracticeResult{}, invalid("sessionId and a valid submissionId are required")
	}
	if runeLength(answerText) < 1 || runeLength(answerText) > 1000 {
		return PracticeResult{}, invalid("answerText must contain 1 to 1000 characters")
	}
	if err := security.CheckInput(ctx, answerText); err != nil {
		return PracticeResult{}, unsafeInput(err)
	}

	var result PracticeResult
	var appendEvent bool
	var eventKnowledgePoint string
	_, err := s.repository.updatePractice(sessionID, func(session *PracticeSession) error {
		if session.StudentID != principal.UserID || session.ClassID != principal.ClassID {
			return notFound()
		}
		if previous, ok := session.Submissions[submissionID]; ok {
			if previous.AnswerText != answerText {
				return conflict("submissionId was already used with different content")
			}
			result = clonePracticeResult(previous.Result)
			return nil
		}
		if session.Finished {
			return conflict("practice session is already finished")
		}

		outcome := gradePracticeAnswer(session.CurrentQuestion, answerText)
		session.QuestionCount++
		if outcome == "correct" {
			session.CorrectCount++
		}
		eventKnowledgePoint = session.CurrentQuestion.KnowledgePoint
		transition := "same_level_variation"
		finishReason := ""

		switch outcome {
		case "correct":
			session.ConsecutiveCorrect++
			session.MasteryEvidence = append(session.MasteryEvidence,
				"correct@"+session.CurrentQuestion.KnowledgePoint+"/"+session.CurrentQuestion.Difficulty)
			if session.Difficulty == "hard" {
				session.HardCorrect++
				if session.HardCorrect >= 3 {
					finishReason = "mastered"
					transition = "mastered"
				}
			} else if session.ConsecutiveCorrect >= 2 {
				session.Difficulty = nextDifficulty(session.Difficulty)
				session.ConsecutiveCorrect = 0
				session.HardCorrect = 0
				transition = "level_up"
			}
		case "conceptual_error":
			session.ConsecutiveCorrect = 0
			session.HardCorrect = 0
			session.Difficulty = "easy"
			prerequisite := strings.TrimSpace(session.CurrentQuestion.Prerequisite)
			if prerequisite != "" && hasQuestionInBank(s.repository.practiceBank, session.ClassID, prerequisite, "easy") {
				session.KnowledgePoint = prerequisite
				transition = "prerequisite"
			} else {
				session.KnowledgePoint = session.CurrentQuestion.KnowledgePoint
				transition = "fallback_easy"
			}
		case "calculation_error":
			session.ConsecutiveCorrect = 0
			session.HardCorrect = 0
			transition = "same_level_variation"
		}
		if session.QuestionCount >= 10 && finishReason == "" {
			finishReason = "question_limit"
			transition = "question_limit"
		}

		attempt := PracticeAttempt{
			QuestionID:     session.CurrentQuestion.ID,
			KnowledgePoint: session.CurrentQuestion.KnowledgePoint,
			Difficulty:     session.CurrentQuestion.Difficulty,
			Outcome:        outcome,
			Transition:     transition,
		}
		session.Attempts = append(session.Attempts, attempt)
		session.UpdatedAt = s.now().UTC()
		result = PracticeResult{
			SubmissionID: submissionID,
			Outcome:      outcome,
			Transition:   transition,
		}
		if finishReason != "" {
			session.Finished = true
			session.FinishReason = finishReason
			session.Question = nil
			session.CurrentQuestion = PracticeQuestion{}
			result.Finished = true
			result.FinishReason = finishReason
			report := practiceReport(*session)
			result.Summary = &report
		} else {
			next, ok := selectQuestionFromBank(s.repository.practiceBank, session.ClassID, session.KnowledgePoint, session.Difficulty, session.QuestionCount)
			if !ok {
				return newError(500, "QUESTION_BANK_INVALID", "no validated next question is available", nil)
			}
			session.Difficulty = next.Difficulty
			session.CurrentQuestion = next
			public := publicQuestion(next)
			session.Question = &public
			result.Question = &public
		}
		session.Submissions[submissionID] = PracticeSubmission{
			ID:         submissionID,
			AnswerText: answerText,
			Result:     clonePracticeResult(result),
		}
		appendEvent = true
		return nil
	})
	if err != nil {
		return PracticeResult{}, err
	}
	if appendEvent {
		eventOutcome := store.LearningOutcomeIncorrect
		if result.Outcome == "correct" {
			eventOutcome = store.LearningOutcomeCorrect
		}
		s.repository.memory.AppendEvent(store.LearningEvent{
			ID:        s.repository.newID("event"),
			StudentID: principal.UserID,
			ClassID:   principal.ClassID,
			Kind:      "adaptive_practice",
			Summary:   "server-graded adaptive practice answer",
			KP:        eventKnowledgePoint,
			Outcome:   eventOutcome,
			At:        s.now().UTC(),
		})
		s.log(ctx, principal, "tutoring.practice.submitted", map[string]any{
			"sessionId": sessionID, "submissionId": submissionID,
			"outcome": result.Outcome, "transition": result.Transition,
		})
	}
	return result, nil
}

func (s *Service) QuitPractice(ctx context.Context, principal auth.Principal, sessionID, submissionID string) (PracticeResult, error) {
	if principal.Role != auth.RoleStudent {
		return PracticeResult{}, forbidden("adaptive practice is student-only")
	}
	if !s.repository.skillEnabled(principal.ClassID, "adaptive_practice") {
		return PracticeResult{}, forbidden("adaptive practice is disabled for this class")
	}
	submissionID = strings.TrimSpace(submissionID)
	if submissionID == "" || runeLength(submissionID) > 128 {
		return PracticeResult{}, invalid("a valid submissionId is required")
	}
	var result PracticeResult
	_, err := s.repository.updatePractice(strings.TrimSpace(sessionID), func(session *PracticeSession) error {
		if session.StudentID != principal.UserID || session.ClassID != principal.ClassID {
			return notFound()
		}
		if previous, ok := session.Submissions[submissionID]; ok {
			if previous.AnswerText != "__quit__" {
				return conflict("submissionId was already used with different content")
			}
			result = clonePracticeResult(previous.Result)
			return nil
		}
		if session.Finished {
			return conflict("practice session is already finished")
		}
		session.Finished = true
		session.FinishReason = "student_quit"
		session.Question = nil
		session.CurrentQuestion = PracticeQuestion{}
		session.UpdatedAt = s.now().UTC()
		report := practiceReport(*session)
		result = PracticeResult{
			SubmissionID: submissionID,
			Outcome:      "quit",
			Transition:   "quit",
			Finished:     true,
			FinishReason: "student_quit",
			Summary:      &report,
		}
		session.Submissions[submissionID] = PracticeSubmission{
			ID: submissionID, AnswerText: "__quit__", Result: clonePracticeResult(result),
		}
		return nil
	})
	if err != nil {
		return PracticeResult{}, err
	}
	s.log(ctx, principal, "tutoring.practice.quit", map[string]any{
		"sessionId": sessionID, "submissionId": submissionID,
	})
	return result, nil
}

func (s *Service) PracticeHint(ctx context.Context, principal auth.Principal, sessionID string) (map[string]any, error) {
	if principal.Role != auth.RoleStudent {
		return nil, forbidden("adaptive practice is student-only")
	}
	if !s.repository.skillEnabled(principal.ClassID, "adaptive_practice") {
		return nil, forbidden("adaptive practice is disabled for this class")
	}
	var hint string
	var count int
	_, err := s.repository.updatePractice(strings.TrimSpace(sessionID), func(session *PracticeSession) error {
		if session.StudentID != principal.UserID || session.ClassID != principal.ClassID {
			return notFound()
		}
		if session.Finished {
			return conflict("practice session is already finished")
		}
		hint = session.CurrentQuestion.Hint
		if strings.TrimSpace(hint) == "" {
			hint = "回顾当前知识点的定义，再检查每一步。"
		}
		session.HintsUsed++
		count = session.HintsUsed
		session.UpdatedAt = s.now().UTC()
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, principal, "tutoring.practice.hint", map[string]any{
		"sessionId": sessionID, "hintsUsed": count,
	})
	return map[string]any{"hint": hint, "hintsUsed": count}, nil
}

func (s *Service) PracticeSession(_ context.Context, principal auth.Principal, sessionID string) (PracticeSession, error) {
	if !s.repository.skillEnabled(principal.ClassID, "adaptive_practice") {
		return PracticeSession{}, forbidden("adaptive practice is disabled for this class")
	}
	session, ok := s.repository.practice(strings.TrimSpace(sessionID))
	if !ok || principal.Role != auth.RoleStudent || session.StudentID != principal.UserID || session.ClassID != principal.ClassID {
		return PracticeSession{}, notFound()
	}
	return session, nil
}

func gradePracticeAnswer(question PracticeQuestion, answer string) string {
	normalized := normalizeText(answer)
	if normalized == normalizeText(question.Answer) {
		return "correct"
	}
	for _, acceptable := range question.AcceptableAnswers {
		if normalized == normalizeText(acceptable) {
			return "correct"
		}
	}
	if _, expectedErr := strconv.ParseFloat(strings.TrimSpace(question.Answer), 64); expectedErr == nil {
		if _, answerErr := strconv.ParseFloat(strings.TrimSpace(answer), 64); answerErr == nil {
			return "calculation_error"
		}
	}
	return "conceptual_error"
}

func nextDifficulty(current string) string {
	switch current {
	case "easy":
		return "medium"
	case "medium":
		return "hard"
	default:
		return "hard"
	}
}

func hasQuestionInBank(bank map[string]PracticeQuestion, classID, knowledgePoint, difficulty string) bool {
	for _, question := range bank {
		if (question.ClassID == "" || question.ClassID == classID) &&
			question.KnowledgePoint == knowledgePoint && question.Difficulty == difficulty {
			return true
		}
	}
	return false
}

func selectQuestionFromBank(bank map[string]PracticeQuestion, classID, knowledgePoint, difficulty string, offset int) (PracticeQuestion, bool) {
	for _, candidateDifficulty := range []string{difficulty, "easy", "medium", "hard"} {
		var candidates []PracticeQuestion
		for _, question := range bank {
			if (question.ClassID == "" || question.ClassID == classID) &&
				question.KnowledgePoint == knowledgePoint && question.Difficulty == candidateDifficulty {
				candidates = append(candidates, question)
			}
		}
		if len(candidates) == 0 {
			continue
		}
		sortPracticeQuestions(candidates)
		return candidates[offset%len(candidates)], true
	}
	return PracticeQuestion{}, false
}

func sortPracticeQuestions(questions []PracticeQuestion) {
	for i := 1; i < len(questions); i++ {
		for j := i; j > 0 && questions[j].ID < questions[j-1].ID; j-- {
			questions[j], questions[j-1] = questions[j-1], questions[j]
		}
	}
}

func publicQuestion(question PracticeQuestion) PublicQuestion {
	return question.PublicQuestion
}

func practiceReport(session PracticeSession) PracticeReport {
	return PracticeReport{
		QuestionCount:       session.QuestionCount,
		CorrectCount:        session.CorrectCount,
		MasteryEvidence:     append([]string(nil), session.MasteryEvidence...),
		CountsTowardGrade:   false,
		FinalKnowledgePoint: session.KnowledgePoint,
	}
}

func clonePracticeResult(result PracticeResult) PracticeResult {
	if result.Question != nil {
		question := *result.Question
		result.Question = &question
	}
	if result.Summary != nil {
		summary := *result.Summary
		summary.MasteryEvidence = append([]string(nil), result.Summary.MasteryEvidence...)
		result.Summary = &summary
	}
	return result
}

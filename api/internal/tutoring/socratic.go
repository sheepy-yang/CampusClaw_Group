package tutoring

import (
	"context"
	"errors"
	"strings"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/auth"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/security"
)

func (s *Service) StartTutor(ctx context.Context, principal auth.Principal, questionID string) (TutorSession, error) {
	if principal.Role != auth.RoleStudent {
		return TutorSession{}, forbidden("socratic tutoring is student-only")
	}
	if !s.repository.skillEnabled(principal.ClassID, "socratic_tutor") {
		return TutorSession{}, forbidden("socratic tutoring is disabled for this class")
	}
	questionID = strings.TrimSpace(questionID)
	var question TutorQuestion
	var ok bool
	if questionID == "" {
		question, ok = s.repository.latestMistakeTutorQuestion(principal.UserID, principal.ClassID)
	} else {
		question, ok = s.repository.tutorQuestion(questionID)
	}
	if !ok || (question.ClassID != "" && question.ClassID != principal.ClassID) ||
		(question.StudentID != "" && question.StudentID != principal.UserID) {
		return TutorSession{}, notFound()
	}
	if s.tutorModel == nil {
		return TutorSession{}, modelUnavailable(errors.New("tutor model is not configured"))
	}
	prompt, err := s.tutorModel.NextTutorQuestion(ctx, TutorModelInput{
		QuestionStem: question.Stem,
	})
	if err != nil {
		return TutorSession{}, modelUnavailable(err)
	}
	if err := validateTutorPrompt(ctx, prompt, question.FullSolution); err != nil {
		return TutorSession{}, invalidModelOutput(err)
	}

	now := s.now().UTC()
	session := TutorSession{
		ID:           s.repository.newID("tutor"),
		StudentID:    principal.UserID,
		ClassID:      principal.ClassID,
		QuestionID:   question.ID,
		QuestionStem: question.Stem,
		Demo:         question.Demo,
		MaxRounds:    5,
		LastPrompt:   strings.TrimSpace(prompt),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.repository.saveTutor(session, 0); err != nil {
		return TutorSession{}, conflict("tutoring session changed concurrently")
	}
	session.Version = 1
	s.log(ctx, principal, "tutoring.socratic.started", map[string]any{
		"sessionId": session.ID, "questionId": question.ID, "demo": question.Demo,
	})
	return publicTutorSession(session), nil
}

func (s *Service) ContinueTutor(ctx context.Context, principal auth.Principal, sessionID, reply string) (TutorSession, error) {
	if principal.Role != auth.RoleStudent {
		return TutorSession{}, forbidden("socratic tutoring is student-only")
	}
	if !s.repository.skillEnabled(principal.ClassID, "socratic_tutor") {
		return TutorSession{}, forbidden("socratic tutoring is disabled for this class")
	}
	reply = strings.TrimSpace(reply)
	if runeLength(reply) < 1 || runeLength(reply) > 1000 {
		return TutorSession{}, invalid("reply must contain 1 to 1000 characters")
	}
	if err := security.CheckInput(ctx, reply); err != nil {
		return TutorSession{}, unsafeInput(err)
	}

	session, ok := s.repository.tutor(strings.TrimSpace(sessionID))
	if !ok || session.StudentID != principal.UserID || session.ClassID != principal.ClassID {
		return TutorSession{}, notFound()
	}
	if session.Released {
		return publicTutorSession(session), nil
	}
	question, ok := s.repository.tutorQuestion(session.QuestionID)
	if !ok {
		return TutorSession{}, newError(500, "QUESTION_DATA_INVALID", "stored tutoring question is unavailable", nil)
	}

	hit := evaluateConceptHit(reply, question)
	explicit := requestsExplanation(reply)
	next := cloneTutorSession(session)
	next.Round++
	if hit {
		next.HitStreak++
	} else {
		next.HitStreak = 0
	}
	turn := TutorTurn{Round: next.Round, Reply: reply, Hit: hit, CreatedAt: s.now().UTC()}
	next.UpdatedAt = turn.CreatedAt

	switch {
	case explicit:
		next.Released = true
		next.ReleaseReason = "student_requested"
	case next.HitStreak >= 2:
		next.Released = true
		next.ReleaseReason = "two_consecutive_hits"
	case next.Round >= next.MaxRounds:
		next.Released = true
		next.ReleaseReason = "max_rounds"
	default:
		if s.tutorModel == nil {
			return TutorSession{}, modelUnavailable(errors.New("tutor model is not configured"))
		}
		prompt, err := s.tutorModel.NextTutorQuestion(ctx, TutorModelInput{
			QuestionStem: question.Stem,
			Round:        next.Round,
			HitStreak:    next.HitStreak,
			StudentReply: reply,
		})
		if err != nil {
			return TutorSession{}, modelUnavailable(err)
		}
		if err := validateTutorPrompt(ctx, prompt, question.FullSolution); err != nil {
			return TutorSession{}, invalidModelOutput(err)
		}
		next.LastPrompt = strings.TrimSpace(prompt)
		turn.Prompt = next.LastPrompt
	}
	next.History = append(next.History, turn)
	if next.Released {
		next.FullSolution = question.FullSolution
		next.LastPrompt = ""
	}

	if err := s.repository.saveTutor(next, session.Version); err != nil {
		return TutorSession{}, conflict("tutoring session changed concurrently")
	}
	next.Version = session.Version + 1
	s.log(ctx, principal, "tutoring.socratic.turn", map[string]any{
		"sessionId": next.ID,
		"round":     next.Round,
		"hit":       hit,
		"released":  next.Released,
		"reason":    next.ReleaseReason,
	})
	return publicTutorSession(next), nil
}

func (s *Service) TutorSession(_ context.Context, principal auth.Principal, sessionID string) (TutorSession, error) {
	if !s.repository.skillEnabled(principal.ClassID, "socratic_tutor") {
		return TutorSession{}, forbidden("socratic tutoring is disabled for this class")
	}
	session, ok := s.repository.tutor(strings.TrimSpace(sessionID))
	if !ok || principal.Role != auth.RoleStudent || session.StudentID != principal.UserID || session.ClassID != principal.ClassID {
		return TutorSession{}, notFound()
	}
	return publicTutorSession(session), nil
}

func evaluateConceptHit(reply string, question TutorQuestion) bool {
	normalized := normalizeText(reply)
	for _, negative := range question.NegativeTerms {
		if strings.Contains(normalized, normalizeText(negative)) {
			return false
		}
	}
	for _, group := range question.KeyConceptGroups {
		matched := false
		for _, concept := range group {
			if strings.Contains(normalized, normalizeText(concept)) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func requestsExplanation(reply string) bool {
	normalized := normalizeText(reply)
	for _, phrase := range []string{"直接讲解", "我要答案", "给我答案", "请讲解", "看解析", "完整解析"} {
		if strings.Contains(normalized, normalizeText(phrase)) {
			return true
		}
	}
	return false
}

func validateTutorPrompt(ctx context.Context, prompt, fullSolution string) error {
	prompt = strings.TrimSpace(prompt)
	if runeLength(prompt) < 1 || runeLength(prompt) > 300 {
		return errors.New("tutor prompt has an invalid length")
	}
	if err := security.CheckInput(ctx, prompt); err != nil {
		return err
	}
	lower := strings.ToLower(prompt)
	for _, marker := range []string{"answerreleased", "answer_released", "fullsolution", "完整解析", "标准答案"} {
		if strings.Contains(lower, marker) {
			return errors.New("tutor prompt attempts to release the answer")
		}
	}
	normalizedSolution := normalizeText(fullSolution)
	if len([]rune(normalizedSolution)) >= 12 && strings.Contains(normalizeText(prompt), normalizedSolution) {
		return errors.New("tutor prompt contains the full solution")
	}
	for _, fragment := range strings.FieldsFunc(fullSolution, func(character rune) bool {
		return strings.ContainsRune("。！？；.!?;", character)
	}) {
		normalizedFragment := normalizeText(fragment)
		if len([]rune(normalizedFragment)) >= 12 && strings.Contains(normalizeText(prompt), normalizedFragment) {
			return errors.New("tutor prompt contains a solution fragment")
		}
	}
	return nil
}

func normalizeText(value string) string {
	replacer := strings.NewReplacer(
		" ", "", "\t", "", "\n", "", "\r", "",
		"，", "", "。", "", "？", "", "！", "",
		",", "", ".", "", "?", "", "!", "",
	)
	return strings.ToLower(replacer.Replace(strings.TrimSpace(value)))
}

func publicTutorSession(session TutorSession) TutorSession {
	if !session.Released {
		session.FullSolution = ""
		session.ReleaseReason = ""
	}
	return session
}

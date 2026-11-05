package tutoring

import (
	"context"
	"errors"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/auth"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/security"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

type ServiceDependencies struct {
	Repository   *Repository
	ReviewModel  ReviewPathModel
	TutorModel   TutorModel
	ClusterModel ClassroomClusterModel
	Audit        security.AuditLogger
	Now          func() time.Time
}

type Service struct {
	repository   *Repository
	reviewModel  ReviewPathModel
	tutorModel   TutorModel
	clusterModel ClassroomClusterModel
	audit        security.AuditLogger
	now          func() time.Time
}

func NewService(deps ServiceDependencies) *Service {
	if deps.Repository == nil {
		deps.Repository = NewRepository(nil)
	}
	if deps.Audit == nil {
		deps.Audit = discardAuditLogger{}
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Service{
		repository:   deps.Repository,
		reviewModel:  deps.ReviewModel,
		tutorModel:   deps.TutorModel,
		clusterModel: deps.ClusterModel,
		audit:        deps.Audit,
		now:          deps.Now,
	}
}

type discardAuditLogger struct{}

func (discardAuditLogger) Log(context.Context, security.AuditEvent) error { return nil }

func (s *Service) LearningState(_ context.Context, principal auth.Principal) (LearningState, error) {
	if principal.Role != auth.RoleStudent {
		return LearningState{}, forbidden("learning state is student-only")
	}
	if !s.repository.skillEnabled(principal.ClassID, "review_path") {
		return LearningState{}, forbidden("review path is disabled for this class")
	}
	return s.learningState(principal.UserID, principal.ClassID, s.now().UTC())
}

func (s *Service) learningState(studentID, classID string, now time.Time) (LearningState, error) {
	if strings.TrimSpace(studentID) == "" || strings.TrimSpace(classID) == "" {
		return LearningState{}, invalid("student and class scope are required")
	}

	byKnowledgePoint := make(map[string]*KnowledgePointState)
	windowStart := now.Add(-learningStateWindow)
	for _, event := range s.repository.learningEvents(studentID, classID) {
		if event.StudentID != studentID || event.ClassID != classID || event.At.Before(windowStart) || event.At.After(now) {
			continue
		}
		knowledgePoint := strings.TrimSpace(event.KP)
		if knowledgePoint == "" {
			continue
		}
		outcome, err := store.NormalizeLearningOutcome(event.Outcome)
		if err != nil {
			return LearningState{}, newError(500, "LEARNING_DATA_INVALID", "stored learning data is invalid", err)
		}
		point := byKnowledgePoint[knowledgePoint]
		if point == nil {
			point = &KnowledgePointState{KnowledgePoint: knowledgePoint}
			byKnowledgePoint[knowledgePoint] = point
		}
		point.EventCount++
		switch outcome {
		case store.LearningOutcomeCorrect:
			point.CorrectCount++
		case store.LearningOutcomeIncorrect:
			point.IncorrectCount++
		case store.LearningOutcomeUnderstood:
			point.UnderstoodCount++
		}
	}

	// Mistakes identify a relevant knowledge point for review, but deliberately
	// do not alter event-based mastery counts or the success-rate denominator.
	for _, mistake := range s.repository.mistakes(studentID, classID) {
		if mistake.StudentID != studentID || mistake.ClassID != classID || mistake.CreatedAt.Before(windowStart) || mistake.CreatedAt.After(now) {
			continue
		}
		knowledgePoint := strings.TrimSpace(mistake.KnowledgePoint)
		if knowledgePoint != "" && byKnowledgePoint[knowledgePoint] == nil {
			byKnowledgePoint[knowledgePoint] = &KnowledgePointState{KnowledgePoint: knowledgePoint}
		}
	}

	keys := make([]string, 0, len(byKnowledgePoint))
	for knowledgePoint := range byKnowledgePoint {
		keys = append(keys, knowledgePoint)
	}
	sort.Strings(keys)
	points := make([]KnowledgePointState, 0, len(keys))
	for _, knowledgePoint := range keys {
		point := byKnowledgePoint[knowledgePoint]
		point.EvidenceCount = point.CorrectCount + point.IncorrectCount + point.UnderstoodCount
		if point.EvidenceCount > 0 {
			rate := float64(point.CorrectCount+point.UnderstoodCount) / float64(point.EvidenceCount)
			point.SuccessRate = math.Round(rate*10000) / 10000
		}
		point.MasteryLevel = masteryLevel(point.EvidenceCount, point.SuccessRate)
		points = append(points, *point)
	}
	return LearningState{
		StudentID:       studentID,
		ClassID:         classID,
		WindowDays:      14,
		KnowledgePoints: points,
	}, nil
}

func masteryLevel(evidenceCount int, successRate float64) MasteryLevel {
	if evidenceCount < 3 {
		return MasteryInsufficientData
	}
	if successRate < 0.5 {
		return MasteryWeak
	}
	if successRate < 0.8 {
		return MasteryDeveloping
	}
	return MasterySolid
}

func (s *Service) GenerateReviewPath(ctx context.Context, principal auth.Principal) (ReviewPathResult, error) {
	if principal.Role != auth.RoleStudent {
		return ReviewPathResult{}, forbidden("review path generation is student-only")
	}
	if !s.repository.skillEnabled(principal.ClassID, "review_path") {
		return ReviewPathResult{}, forbidden("review path is disabled for this class")
	}

	now := s.now().UTC()
	state, err := s.learningState(principal.UserID, principal.ClassID, now)
	if err != nil {
		return ReviewPathResult{}, err
	}
	mistakes, err := s.recentReviewMistakes(ctx, principal.UserID, principal.ClassID, now)
	if err != nil {
		return ReviewPathResult{}, err
	}
	for _, point := range state.KnowledgePoints {
		if runeLength(point.KnowledgePoint) > 100 {
			return ReviewPathResult{}, invalid("stored knowledge point is too long")
		}
		if err := security.CheckInput(ctx, point.KnowledgePoint); err != nil {
			return ReviewPathResult{}, unsafeInput(err)
		}
	}
	if len(state.KnowledgePoints) == 0 && len(mistakes) == 0 {
		result := ReviewPathResult{Skipped: true, Reason: "no recent learning evidence"}
		s.log(ctx, principal, "tutoring.review_path.skipped", map[string]any{"reason": "no_recent_evidence"})
		return result, nil
	}
	if s.reviewModel == nil {
		return ReviewPathResult{}, modelUnavailable(errors.New("review path model is not configured"))
	}

	target := selectReviewTarget(state.KnowledgePoints)
	if target == "" && len(mistakes) > 0 {
		target = mistakes[0].KnowledgePoint
	}
	drafts, err := s.reviewModel.GenerateReviewPath(ctx, ReviewModelInput{
		KnowledgePoints: state.KnowledgePoints,
		Mistakes:        mistakes,
		Target:          target,
	})
	if err != nil {
		return ReviewPathResult{}, modelUnavailable(err)
	}
	if err := validateReviewDrafts(ctx, drafts, state.KnowledgePoints, mistakes); err != nil {
		return ReviewPathResult{}, invalidModelOutput(err)
	}

	plan := ReviewPlan{
		ID:        s.repository.newID("review"),
		StudentID: principal.UserID,
		ClassID:   principal.ClassID,
		// The shared content endpoint exposes published class content to every
		// student. Review plans stay private and are read through owner-aware
		// tutoring handlers instead.
		Status:    "private",
		CreatedAt: now,
		UpdatedAt: now,
		Steps:     make([]ReviewStep, len(drafts)),
	}
	for i, draft := range drafts {
		plan.Steps[i] = ReviewStep{
			ID:             s.repository.newID("step"),
			Order:          i + 1,
			Type:           strings.TrimSpace(draft.Type),
			Title:          strings.TrimSpace(draft.Title),
			Detail:         strings.TrimSpace(draft.Detail),
			KnowledgePoint: strings.TrimSpace(draft.KnowledgePoint),
		}
	}
	if err := s.repository.savePlan(plan); err != nil {
		return ReviewPathResult{}, err
	}
	s.log(ctx, principal, "tutoring.review_path.generated", map[string]any{
		"planId": plan.ID, "stepCount": len(plan.Steps),
	})
	resultPlan := cloneReviewPlan(plan)
	return ReviewPathResult{Plan: &resultPlan}, nil
}

func (s *Service) recentReviewMistakes(ctx context.Context, studentID, classID string, now time.Time) ([]ReviewMistake, error) {
	windowStart := now.Add(-learningStateWindow)
	var result []ReviewMistake
	for _, mistake := range s.repository.mistakes(studentID, classID) {
		if mistake.StudentID != studentID || mistake.ClassID != classID || mistake.CreatedAt.Before(windowStart) || mistake.CreatedAt.After(now) {
			continue
		}
		knowledgePoint := strings.TrimSpace(mistake.KnowledgePoint)
		if knowledgePoint == "" {
			continue
		}
		cause := strings.TrimSpace(mistake.ErrorCause)
		if err := security.CheckInput(ctx, knowledgePoint+"\n"+cause); err != nil {
			return nil, unsafeInput(err)
		}
		if runeLength(knowledgePoint) > 100 || runeLength(cause) > 300 {
			return nil, invalid("stored review evidence is too long")
		}
		result = append(result, ReviewMistake{KnowledgePoint: knowledgePoint, ErrorCause: cause})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].KnowledgePoint == result[j].KnowledgePoint {
			return result[i].ErrorCause < result[j].ErrorCause
		}
		return result[i].KnowledgePoint < result[j].KnowledgePoint
	})
	return result, nil
}

func selectReviewTarget(points []KnowledgePointState) string {
	if len(points) == 0 {
		return ""
	}
	priority := map[MasteryLevel]int{
		MasteryWeak:             0,
		MasteryDeveloping:       1,
		MasteryInsufficientData: 2,
		MasterySolid:            3,
	}
	sorted := append([]KnowledgePointState(nil), points...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left, right := priority[sorted[i].MasteryLevel], priority[sorted[j].MasteryLevel]
		if left == right {
			if sorted[i].SuccessRate == sorted[j].SuccessRate {
				return sorted[i].KnowledgePoint < sorted[j].KnowledgePoint
			}
			return sorted[i].SuccessRate < sorted[j].SuccessRate
		}
		return left < right
	})
	return sorted[0].KnowledgePoint
}

func validateReviewDrafts(ctx context.Context, drafts []ReviewStepDraft, points []KnowledgePointState, mistakes []ReviewMistake) error {
	if len(drafts) < 3 || len(drafts) > 5 {
		return errors.New("review path must contain three to five steps")
	}
	allowedKnowledgePoints := make(map[string]struct{})
	for _, point := range points {
		allowedKnowledgePoints[point.KnowledgePoint] = struct{}{}
	}
	for _, mistake := range mistakes {
		allowedKnowledgePoints[mistake.KnowledgePoint] = struct{}{}
	}
	allowedTypes := map[string]bool{"read": true, "example": true, "practice": true, "self_test": true}
	for _, draft := range drafts {
		stepType := strings.TrimSpace(draft.Type)
		title := strings.TrimSpace(draft.Title)
		detail := strings.TrimSpace(draft.Detail)
		knowledgePoint := strings.TrimSpace(draft.KnowledgePoint)
		if !allowedTypes[stepType] {
			return errors.New("review path contains an unknown step type")
		}
		if runeLength(title) < 1 || runeLength(title) > 120 || runeLength(detail) > 500 {
			return errors.New("review path contains an invalid field length")
		}
		if err := security.CheckInput(ctx, title+"\n"+detail); err != nil {
			return errors.New("review path contains unsafe model text")
		}
		if _, ok := allowedKnowledgePoints[knowledgePoint]; !ok {
			return errors.New("review path contains an unknown knowledge point")
		}
	}
	return nil
}

func (s *Service) LatestReviewPath(_ context.Context, principal auth.Principal, requestedStudentID string) (*ReviewPlan, error) {
	studentID, err := scopedStudent(principal, requestedStudentID)
	if err != nil {
		return nil, err
	}
	if !s.repository.skillEnabled(principal.ClassID, "review_path") {
		return nil, forbidden("review path is disabled for this class")
	}
	plan, ok := s.repository.getLatestPlan(principal.ClassID, studentID)
	if !ok {
		return nil, nil
	}
	return &plan, nil
}

func (s *Service) ReviewPath(_ context.Context, principal auth.Principal, planID string) (ReviewPlan, error) {
	if !s.repository.skillEnabled(principal.ClassID, "review_path") {
		return ReviewPlan{}, forbidden("review path is disabled for this class")
	}
	plan, ok := s.repository.getPlan(planID)
	if !ok || plan.ClassID != principal.ClassID {
		return ReviewPlan{}, notFound()
	}
	if principal.Role == auth.RoleStudent && plan.StudentID != principal.UserID {
		return ReviewPlan{}, notFound()
	}
	if principal.Role != auth.RoleStudent && principal.Role != auth.RoleTeacher {
		return ReviewPlan{}, forbidden("forbidden")
	}
	return plan, nil
}

func (s *Service) CompleteReviewStep(ctx context.Context, principal auth.Principal, planID, stepID string) (ReviewPlan, error) {
	if principal.Role != auth.RoleStudent {
		return ReviewPlan{}, forbidden("only the plan owner can complete a step")
	}
	if !s.repository.skillEnabled(principal.ClassID, "review_path") {
		return ReviewPlan{}, forbidden("review path is disabled for this class")
	}
	plan, err := s.repository.completePlanStep(planID, stepID, principal.UserID, principal.ClassID, s.now().UTC())
	if err != nil {
		return ReviewPlan{}, err
	}
	s.log(ctx, principal, "tutoring.review_path.step_completed", map[string]any{
		"planId": planID, "stepId": stepID,
	})
	return plan, nil
}

func scopedStudent(principal auth.Principal, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	switch principal.Role {
	case auth.RoleStudent:
		if requested != "" && requested != principal.UserID {
			return "", notFound()
		}
		return principal.UserID, nil
	case auth.RoleTeacher:
		if requested == "" {
			return "", invalid("studentId is required for a teacher request")
		}
		return requested, nil
	default:
		return "", forbidden("forbidden")
	}
}

func (s *Service) log(ctx context.Context, principal auth.Principal, action string, detail map[string]any) {
	if err := s.audit.Log(ctx, security.AuditEvent{
		ActorID: principal.UserID,
		ClassID: principal.ClassID,
		Action:  action,
		Detail:  detail,
	}); err != nil {
		log.Printf("tutoring audit persistence failed action=%s actor=%s: %v", action, principal.UserID, err)
	}
}

func runeLength(value string) int { return len([]rune(value)) }

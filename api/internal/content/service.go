package content

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/agent"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/auth"
)

const (
	maxEvidenceSources    = 32
	maxEvidenceCharacters = 64 * 1024
	maxSourceCharacters   = 8 * 1024
	maxPayloadBytes       = 256 * 1024
	maxUpdateRetries      = 8
)

var (
	idempotencyPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{7,127}$`)
	identifierPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	unsafeInputPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)ignore\s+(all\s+)?previous\s+instructions`),
		regexp.MustCompile(`(?i)system\s*:\s*`),
		regexp.MustCompile(`(?i)<\|?(system|assistant)\|?>`),
		regexp.MustCompile(`(?i)你现在是系统`),
	}
)

type ServiceOptions struct {
	Repository  Repository
	Evidence    EvidenceProvider
	Skills      SkillGate
	Producer    agent.ContentProductionAgent
	Reviewer    agent.ContentQualityAgent
	Clock       func() time.Time
	IDGenerator func(prefix string) string
}

type Service struct {
	repository Repository
	evidence   EvidenceProvider
	skills     SkillGate
	producer   agent.ContentProductionAgent
	reviewer   agent.ContentQualityAgent
	clock      func() time.Time
	id         func(prefix string) string
	sequence   atomic.Uint64
}

func NewService(options ServiceOptions) *Service {
	if options.Repository == nil {
		options.Repository = NewMemoryRepository()
	}
	if options.Evidence == nil {
		options.Evidence = unavailableEvidenceProvider{}
	}
	if options.Skills == nil {
		options.Skills = unavailableSkillGate{}
	}
	if options.Producer == nil {
		options.Producer = agent.NewDeterministicContentAgent()
	}
	if options.Reviewer == nil {
		options.Reviewer = agent.NewDeterministicQualityAgent()
	}
	if options.Clock == nil {
		options.Clock = time.Now
	}
	service := &Service{
		repository: options.Repository,
		evidence:   options.Evidence,
		skills:     options.Skills,
		producer:   options.Producer,
		reviewer:   options.Reviewer,
		clock:      options.Clock,
	}
	if options.IDGenerator != nil {
		service.id = options.IDGenerator
	} else {
		service.id = func(prefix string) string {
			next := service.sequence.Add(1)
			return fmt.Sprintf("%s-%d-%d", prefix, service.now().UnixNano(), next)
		}
	}
	return service
}

func NewDefaultService() *Service {
	return NewService(ServiceOptions{})
}

func (s *Service) GenerateCourseCards(
	ctx context.Context,
	principal auth.Principal,
	request CourseCardRequest,
) (GenerationResult, error) {
	production := agent.ProductionRequest{
		Skill:   agent.SkillCourseCards,
		Topic:   strings.TrimSpace(request.Topic),
		Count:   request.Count,
		Sources: nil,
	}
	return s.generate(ctx, principal, request.Meta, request.SourceRefs, production)
}

func (s *Service) GenerateExercises(
	ctx context.Context,
	principal auth.Principal,
	request ExerciseRequest,
) (GenerationResult, error) {
	production := agent.ProductionRequest{
		Skill:         agent.SkillExercises,
		Topic:         strings.TrimSpace(request.Topic),
		Count:         request.Count,
		ExerciseTypes: normalizedStrings(request.ExerciseTypes),
		Difficulty:    strings.ToLower(strings.TrimSpace(request.Difficulty)),
	}
	return s.generate(ctx, principal, request.Meta, request.SourceRefs, production)
}

func (s *Service) GenerateLessonPlan(
	ctx context.Context,
	principal auth.Principal,
	request LessonPlanRequest,
) (GenerationResult, error) {
	production := agent.ProductionRequest{
		Skill:           agent.SkillLessonPlan,
		Topic:           strings.TrimSpace(request.Topic),
		DurationMinutes: request.DurationMinutes,
		Objectives:      normalizedStrings(request.Objectives),
	}
	return s.generate(ctx, principal, request.Meta, request.SourceRefs, production)
}

func (s *Service) generate(
	ctx context.Context,
	principal auth.Principal,
	meta RequestMeta,
	refs []EvidenceRef,
	production agent.ProductionRequest,
) (GenerationResult, error) {
	started := s.now()
	action := "content.generate." + string(production.Skill)
	if err := requireTeacher(principal); err != nil {
		s.audit(ctx, principal, action, "", production.Skill, "rejected", AuditUsage{}, started, meta.RequestID)
		return GenerationResult{}, err
	}
	if err := validateIdempotencyKey(meta.IdempotencyKey); err != nil {
		s.audit(ctx, principal, action, "", production.Skill, "rejected", AuditUsage{}, started, meta.RequestID)
		return GenerationResult{}, err
	}
	if err := validateEvidenceRefs(refs); err != nil {
		s.audit(ctx, principal, action, "", production.Skill, "rejected", AuditUsage{}, started, meta.RequestID)
		return GenerationResult{}, err
	}
	if err := validateProductionRequest(production); err != nil {
		s.audit(ctx, principal, action, "", production.Skill, "rejected", AuditUsage{}, started, meta.RequestID)
		return GenerationResult{}, err
	}
	if err := s.requireEnabled(ctx, principal.ClassID, production.Skill); err != nil {
		s.audit(ctx, principal, action, "", production.Skill, "rejected", AuditUsage{}, started, meta.RequestID)
		return GenerationResult{}, err
	}

	sources, err := s.resolveEvidence(ctx, principal.ClassID, refs)
	if err != nil {
		s.audit(ctx, principal, action, "", production.Skill, "failed", AuditUsage{}, started, meta.RequestID)
		return GenerationResult{}, err
	}
	if production.Skill != agent.SkillLessonPlan && production.Count > len(sources) {
		err := &ValidationError{
			Field:   "sourceRefs",
			Message: "the number of distinct evidence chunks must cover every generated item",
		}
		s.audit(ctx, principal, action, "", production.Skill, "rejected", AuditUsage{SourceCount: len(sources)}, started, meta.RequestID)
		return GenerationResult{}, err
	}
	production.Sources = sources
	requestHash, err := hashJSON(production)
	if err != nil {
		s.audit(ctx, principal, action, "", production.Skill, "failed", AuditUsage{SourceCount: len(sources)}, started, meta.RequestID)
		return GenerationResult{}, fmt.Errorf("%w: encode request", ErrGenerationFailed)
	}
	scope := idempotencyScope(principal, production.Skill, meta.IdempotencyKey)
	existing, found, err := s.repository.LookupIdempotency(ctx, scope, requestHash)
	if err != nil {
		result := "failed"
		if errors.Is(err, ErrIdempotencyConflict) {
			result = "rejected"
		}
		s.audit(ctx, principal, action, "", production.Skill, result, AuditUsage{SourceCount: len(sources)}, started, meta.RequestID)
		return GenerationResult{}, err
	}
	if found {
		s.audit(ctx, principal, action, existing.ID, production.Skill, "replayed", usageFromProduction(existing.Usage), started, meta.RequestID)
		return GenerationResult{
			ContentID: existing.ID,
			Status:    existing.Status,
			Kind:      existing.Kind,
			Version:   existing.Version,
			Replayed:  true,
			Content:   existing,
		}, nil
	}
	output, err := s.producer.Produce(ctx, production)
	if err != nil {
		s.audit(ctx, principal, action, "", production.Skill, "failed", AuditUsage{SourceCount: len(sources)}, started, meta.RequestID)
		return GenerationResult{}, fmt.Errorf("%w: %v", ErrGenerationFailed, err)
	}
	if err := validateProductionOutput(output); err != nil {
		s.audit(ctx, principal, action, "", production.Skill, "failed", AuditUsage{SourceCount: len(sources)}, started, meta.RequestID)
		return GenerationResult{}, fmt.Errorf("%w: %v", ErrGenerationFailed, err)
	}
	report, err := s.review(ctx, production.Skill, output.Payload, sources)
	if err != nil {
		s.audit(ctx, principal, action, "", production.Skill, "failed", usageFromProduction(output.Usage), started, meta.RequestID)
		return GenerationResult{}, err
	}

	contentID := s.id("content")
	now := s.now()
	reviewRun := ReviewRun{
		ID:        s.id("review"),
		ContentID: contentID,
		ClassID:   principal.ClassID,
		ActorID:   principal.UserID,
		Kind:      production.Skill,
		Report:    report,
		Usage: ReviewUsage{
			Mode:         "deterministic_validation",
			SourceCount:  len(sources),
			CheckedItems: report.CheckedItems,
		},
		CreatedAt: now,
	}
	status := StatusPendingConfirmation
	if !report.Passed {
		status = StatusNeedsRevision
	}
	record := ContentRecord{
		ID:             contentID,
		ClassID:        principal.ClassID,
		CreatedBy:      principal.UserID,
		Kind:           production.Skill,
		Status:         status,
		Version:        1,
		Payload:        cloneMap(output.Payload),
		Citations:      citationsFromEvidence(sources),
		LastReview:     reviewRun,
		Usage:          output.Usage,
		CreatedAt:      now,
		UpdatedAt:      now,
		Evidence:       append([]agent.GroundingSource(nil), sources...),
		IdempotencyKey: meta.IdempotencyKey,
		RequestHash:    requestHash,
	}
	stored, replayed, err := s.repository.CreateOrGet(ctx, record, scope, requestHash)
	if err != nil {
		result := "failed"
		if errors.Is(err, ErrIdempotencyConflict) {
			result = "rejected"
		}
		s.audit(ctx, principal, action, "", production.Skill, result, usageFromProduction(output.Usage), started, meta.RequestID)
		return GenerationResult{}, err
	}
	auditResult := "succeeded"
	if replayed {
		auditResult = "replayed"
	}
	s.audit(ctx, principal, action, stored.ID, production.Skill, auditResult, usageFromProduction(stored.Usage), started, meta.RequestID)
	return GenerationResult{
		ContentID: stored.ID,
		Status:    stored.Status,
		Kind:      stored.Kind,
		Version:   stored.Version,
		Replayed:  replayed,
		Content:   stored,
	}, nil
}

// ReviewContent runs the fourth content-production skill. It can review an
// existing class-scoped draft or a supplied candidate against server-resolved
// evidence.
func (s *Service) ReviewContent(
	ctx context.Context,
	principal auth.Principal,
	request QualityReviewRequest,
) (ReviewRun, error) {
	started := s.now()
	action := "content.quality_review"
	if err := requireTeacher(principal); err != nil {
		s.audit(ctx, principal, action, "", agent.SkillQuality, "rejected", AuditUsage{}, started, request.RequestID)
		return ReviewRun{}, err
	}
	if strings.TrimSpace(request.ContentID) != "" {
		return s.reviewExisting(ctx, principal, request.ContentID, request.RequestID, started)
	}
	if !isGeneratableSkill(request.Kind) {
		err := &ValidationError{Field: "kind", Message: "must be course_cards, exercises, or lesson_plan"}
		s.audit(ctx, principal, action, "", agent.SkillQuality, "rejected", AuditUsage{}, started, request.RequestID)
		return ReviewRun{}, err
	}
	if err := s.requireEnabled(ctx, principal.ClassID, request.Kind); err != nil {
		s.audit(ctx, principal, action, "", agent.SkillQuality, "rejected", AuditUsage{}, started, request.RequestID)
		return ReviewRun{}, err
	}
	if err := validatePayload(request.Payload); err != nil {
		s.audit(ctx, principal, action, "", agent.SkillQuality, "rejected", AuditUsage{}, started, request.RequestID)
		return ReviewRun{}, err
	}
	if err := validateEvidenceRefs(request.SourceRefs); err != nil {
		s.audit(ctx, principal, action, "", agent.SkillQuality, "rejected", AuditUsage{}, started, request.RequestID)
		return ReviewRun{}, err
	}
	sources, err := s.resolveEvidence(ctx, principal.ClassID, request.SourceRefs)
	if err != nil {
		s.audit(ctx, principal, action, "", agent.SkillQuality, "failed", AuditUsage{}, started, request.RequestID)
		return ReviewRun{}, err
	}
	report, err := s.review(ctx, request.Kind, request.Payload, sources)
	if err != nil {
		s.audit(ctx, principal, action, "", agent.SkillQuality, "failed", AuditUsage{SourceCount: len(sources)}, started, request.RequestID)
		return ReviewRun{}, err
	}
	run := ReviewRun{
		ID:      s.id("review"),
		ClassID: principal.ClassID,
		ActorID: principal.UserID,
		Kind:    request.Kind,
		Report:  report,
		Usage: ReviewUsage{
			Mode:         "deterministic_validation",
			SourceCount:  len(sources),
			CheckedItems: report.CheckedItems,
		},
		CreatedAt: s.now(),
	}
	result := "passed"
	if !report.Passed {
		result = "needs_revision"
	}
	s.audit(ctx, principal, action, run.ID, agent.SkillQuality, result, AuditUsage{
		Mode:        run.Usage.Mode,
		SourceCount: run.Usage.SourceCount,
		OutputItems: run.Usage.CheckedItems,
	}, started, request.RequestID)
	return run, nil
}

func (s *Service) reviewExisting(
	ctx context.Context,
	principal auth.Principal,
	contentID string,
	requestID string,
	started time.Time,
) (ReviewRun, error) {
	action := "content.quality_review"
	for attempt := 0; attempt < maxUpdateRetries; attempt++ {
		record, err := s.scopedRecord(ctx, principal, strings.TrimSpace(contentID))
		if err != nil {
			s.audit(ctx, principal, action, contentID, agent.SkillQuality, "rejected", AuditUsage{}, started, requestID)
			return ReviewRun{}, err
		}
		if err := s.requireEnabled(ctx, principal.ClassID, record.Kind); err != nil {
			s.audit(ctx, principal, action, contentID, agent.SkillQuality, "rejected", AuditUsage{}, started, requestID)
			return ReviewRun{}, err
		}
		report, err := s.review(ctx, record.Kind, record.Payload, record.Evidence)
		if err != nil {
			s.audit(ctx, principal, action, contentID, agent.SkillQuality, "failed", AuditUsage{SourceCount: len(record.Evidence)}, started, requestID)
			return ReviewRun{}, err
		}
		run := ReviewRun{
			ID:        s.id("review"),
			ContentID: record.ID,
			ClassID:   record.ClassID,
			ActorID:   principal.UserID,
			Kind:      record.Kind,
			Report:    report,
			Usage: ReviewUsage{
				Mode:         "deterministic_validation",
				SourceCount:  len(record.Evidence),
				CheckedItems: report.CheckedItems,
			},
			CreatedAt: s.now(),
		}
		record.LastReview = run
		if record.Status == StatusPendingConfirmation || record.Status == StatusNeedsRevision {
			if report.Passed {
				record.Status = StatusPendingConfirmation
			} else {
				record.Status = StatusNeedsRevision
			}
		}
		record.UpdatedAt = s.now()
		_, err = s.repository.Update(ctx, record, record.Revision)
		if errors.Is(err, ErrConcurrentUpdate) {
			continue
		}
		if err != nil {
			s.audit(ctx, principal, action, contentID, agent.SkillQuality, "failed", AuditUsage{}, started, requestID)
			return ReviewRun{}, err
		}
		result := "passed"
		if !report.Passed {
			result = "needs_revision"
		}
		s.audit(ctx, principal, action, contentID, agent.SkillQuality, result, AuditUsage{
			Mode:        run.Usage.Mode,
			SourceCount: run.Usage.SourceCount,
			OutputItems: run.Usage.CheckedItems,
		}, started, requestID)
		return run, nil
	}
	s.audit(ctx, principal, action, contentID, agent.SkillQuality, "failed", AuditUsage{}, started, requestID)
	return ReviewRun{}, ErrConcurrentUpdate
}

func (s *Service) Confirm(
	ctx context.Context,
	principal auth.Principal,
	contentID string,
	request ConfirmRequest,
) (ContentRecord, error) {
	started := s.now()
	action := "content.confirm"
	if err := requireTeacher(principal); err != nil {
		s.audit(ctx, principal, action, contentID, "", "rejected", AuditUsage{}, started, request.RequestID)
		return ContentRecord{}, err
	}
	if request.Payload != nil {
		if err := validatePayload(request.Payload); err != nil {
			s.audit(ctx, principal, action, contentID, "", "rejected", AuditUsage{}, started, request.RequestID)
			return ContentRecord{}, err
		}
	}
	for attempt := 0; attempt < maxUpdateRetries; attempt++ {
		record, err := s.scopedRecord(ctx, principal, strings.TrimSpace(contentID))
		if err != nil {
			s.audit(ctx, principal, action, contentID, "", "rejected", AuditUsage{}, started, request.RequestID)
			return ContentRecord{}, err
		}
		if err := s.requireEnabled(ctx, principal.ClassID, record.Kind); err != nil {
			s.audit(ctx, principal, action, contentID, record.Kind, "rejected", AuditUsage{}, started, request.RequestID)
			return ContentRecord{}, err
		}
		if record.Status == StatusConfirmed || record.Status == StatusPublished {
			if request.Payload == nil || payloadEqual(request.Payload, record.Payload) {
				s.audit(ctx, principal, action, contentID, record.Kind, "idempotent", usageFromProduction(record.Usage), started, request.RequestID)
				return record, nil
			}
			s.audit(ctx, principal, action, contentID, record.Kind, "rejected", usageFromProduction(record.Usage), started, request.RequestID)
			return ContentRecord{}, ErrImmutableContent
		}
		if record.Status != StatusPendingConfirmation && record.Status != StatusNeedsRevision {
			s.audit(ctx, principal, action, contentID, record.Kind, "rejected", usageFromProduction(record.Usage), started, request.RequestID)
			return ContentRecord{}, ErrInvalidTransition
		}

		candidate := cloneRecord(record)
		if request.Payload != nil && !payloadEqual(request.Payload, candidate.Payload) {
			candidate.Payload = cloneMap(request.Payload)
			candidate.Version++
		}
		report, err := s.review(ctx, candidate.Kind, candidate.Payload, candidate.Evidence)
		if err != nil {
			s.audit(ctx, principal, action, contentID, candidate.Kind, "failed", usageFromProduction(candidate.Usage), started, request.RequestID)
			return ContentRecord{}, err
		}
		candidate.LastReview = ReviewRun{
			ID:        s.id("review"),
			ContentID: candidate.ID,
			ClassID:   candidate.ClassID,
			ActorID:   principal.UserID,
			Kind:      candidate.Kind,
			Report:    report,
			Usage: ReviewUsage{
				Mode:         "deterministic_validation",
				SourceCount:  len(candidate.Evidence),
				CheckedItems: report.CheckedItems,
			},
			CreatedAt: s.now(),
		}
		candidate.UpdatedAt = s.now()
		if !report.Passed {
			candidate.Status = StatusNeedsRevision
			updated, updateErr := s.repository.Update(ctx, candidate, record.Revision)
			if errors.Is(updateErr, ErrConcurrentUpdate) {
				continue
			}
			if updateErr != nil {
				s.audit(ctx, principal, action, contentID, candidate.Kind, "failed", usageFromProduction(candidate.Usage), started, request.RequestID)
				return ContentRecord{}, updateErr
			}
			s.audit(ctx, principal, action, contentID, candidate.Kind, "rejected", usageFromProduction(candidate.Usage), started, request.RequestID)
			return updated, ErrQualityReviewFailed
		}
		now := s.now()
		candidate.Status = StatusConfirmed
		candidate.ConfirmedBy = principal.UserID
		candidate.ConfirmedAt = &now
		candidate.RevisionNote = ""
		candidate.RevisionRequestedBy = ""
		candidate.RevisionRequestedAt = nil
		updated, updateErr := s.repository.Update(ctx, candidate, record.Revision)
		if errors.Is(updateErr, ErrConcurrentUpdate) {
			continue
		}
		if updateErr != nil {
			s.audit(ctx, principal, action, contentID, candidate.Kind, "failed", usageFromProduction(candidate.Usage), started, request.RequestID)
			return ContentRecord{}, updateErr
		}
		s.audit(ctx, principal, action, contentID, candidate.Kind, "succeeded", usageFromProduction(candidate.Usage), started, request.RequestID)
		return updated, nil
	}
	s.audit(ctx, principal, action, contentID, "", "failed", AuditUsage{}, started, request.RequestID)
	return ContentRecord{}, ErrConcurrentUpdate
}

func (s *Service) Reject(
	ctx context.Context,
	principal auth.Principal,
	contentID string,
	request RejectRequest,
) (ContentRecord, error) {
	started := s.now()
	action := "content.reject"
	reason := strings.TrimSpace(request.Reason)
	if err := requireTeacher(principal); err != nil {
		s.audit(ctx, principal, action, contentID, "", "rejected", AuditUsage{}, started, request.RequestID)
		return ContentRecord{}, err
	}
	if err := validateText("reason", reason, 1, 500); err != nil || containsUnsafeInstruction(reason) {
		s.audit(ctx, principal, action, contentID, "", "rejected", AuditUsage{}, started, request.RequestID)
		if err != nil {
			return ContentRecord{}, err
		}
		return ContentRecord{}, &ValidationError{Field: "reason", Message: "contains unsafe instructions"}
	}
	for attempt := 0; attempt < maxUpdateRetries; attempt++ {
		record, err := s.scopedRecord(ctx, principal, strings.TrimSpace(contentID))
		if err != nil {
			s.audit(ctx, principal, action, contentID, "", "rejected", AuditUsage{}, started, request.RequestID)
			return ContentRecord{}, err
		}
		if err := s.requireEnabled(ctx, principal.ClassID, record.Kind); err != nil {
			s.audit(ctx, principal, action, contentID, record.Kind, "rejected", AuditUsage{}, started, request.RequestID)
			return ContentRecord{}, err
		}
		if record.Status == StatusPublished {
			return ContentRecord{}, ErrImmutableContent
		}
		if record.Status == StatusNeedsRevision && record.RevisionNote == reason {
			return record, nil
		}
		if record.Status != StatusPendingConfirmation && record.Status != StatusConfirmed && record.Status != StatusNeedsRevision {
			return ContentRecord{}, ErrInvalidTransition
		}
		now := s.now()
		record.Status = StatusNeedsRevision
		record.RevisionNote = reason
		record.RevisionRequestedBy = principal.UserID
		record.RevisionRequestedAt = &now
		record.ConfirmedBy = ""
		record.ConfirmedAt = nil
		record.UpdatedAt = now
		updated, updateErr := s.repository.Update(ctx, record, record.Revision)
		if errors.Is(updateErr, ErrConcurrentUpdate) {
			continue
		}
		if updateErr != nil {
			s.audit(ctx, principal, action, contentID, record.Kind, "failed", usageFromProduction(record.Usage), started, request.RequestID)
			return ContentRecord{}, updateErr
		}
		s.audit(ctx, principal, action, contentID, record.Kind, "succeeded", usageFromProduction(record.Usage), started, request.RequestID)
		return updated, nil
	}
	return ContentRecord{}, ErrConcurrentUpdate
}

func (s *Service) Publish(
	ctx context.Context,
	principal auth.Principal,
	contentID string,
	request PublishRequest,
) (ContentRecord, error) {
	started := s.now()
	action := "content.publish"
	if err := requireTeacher(principal); err != nil {
		s.audit(ctx, principal, action, contentID, "", "rejected", AuditUsage{}, started, request.RequestID)
		return ContentRecord{}, err
	}
	if err := validateIdempotencyKey(request.IdempotencyKey); err != nil {
		s.audit(ctx, principal, action, contentID, "", "rejected", AuditUsage{}, started, request.RequestID)
		return ContentRecord{}, err
	}
	for attempt := 0; attempt < maxUpdateRetries; attempt++ {
		record, err := s.scopedRecord(ctx, principal, strings.TrimSpace(contentID))
		if err != nil {
			s.audit(ctx, principal, action, contentID, "", "rejected", AuditUsage{}, started, request.RequestID)
			return ContentRecord{}, err
		}
		if err := s.requireEnabled(ctx, principal.ClassID, record.Kind); err != nil {
			s.audit(ctx, principal, action, contentID, record.Kind, "rejected", AuditUsage{}, started, request.RequestID)
			return ContentRecord{}, err
		}
		if record.Status == StatusPublished {
			s.audit(ctx, principal, action, contentID, record.Kind, "idempotent", usageFromProduction(record.Usage), started, request.RequestID)
			return record, nil
		}
		if record.Status != StatusConfirmed || !record.LastReview.Report.Passed {
			s.audit(ctx, principal, action, contentID, record.Kind, "rejected", usageFromProduction(record.Usage), started, request.RequestID)
			return ContentRecord{}, ErrInvalidTransition
		}
		digest, err := hashJSON(struct {
			Kind      agent.ProductionSkill
			Version   int
			Payload   map[string]any
			Citations []agent.Citation
		}{
			Kind:      record.Kind,
			Version:   record.Version,
			Payload:   record.Payload,
			Citations: record.Citations,
		})
		if err != nil {
			s.audit(ctx, principal, action, contentID, record.Kind, "failed", usageFromProduction(record.Usage), started, request.RequestID)
			return ContentRecord{}, err
		}
		now := s.now()
		record.Status = StatusPublished
		record.PublishedBy = principal.UserID
		record.PublishedAt = &now
		record.FrozenDigest = digest
		record.FrozenPayload = cloneMap(record.Payload)
		record.PublishKey = request.IdempotencyKey
		record.UpdatedAt = now
		updated, updateErr := s.repository.Update(ctx, record, record.Revision)
		if errors.Is(updateErr, ErrConcurrentUpdate) {
			continue
		}
		if updateErr != nil {
			s.audit(ctx, principal, action, contentID, record.Kind, "failed", usageFromProduction(record.Usage), started, request.RequestID)
			return ContentRecord{}, updateErr
		}
		s.audit(ctx, principal, action, contentID, record.Kind, "succeeded", usageFromProduction(record.Usage), started, request.RequestID)
		return updated, nil
	}
	s.audit(ctx, principal, action, contentID, "", "failed", AuditUsage{}, started, request.RequestID)
	return ContentRecord{}, ErrConcurrentUpdate
}

func (s *Service) Get(
	ctx context.Context,
	principal auth.Principal,
	contentID string,
) (ContentRecord, error) {
	if err := requireAuthenticated(principal); err != nil {
		return ContentRecord{}, err
	}
	record, err := s.scopedRecord(ctx, principal, strings.TrimSpace(contentID))
	if err != nil {
		return ContentRecord{}, err
	}
	if principal.Role == auth.RoleTeacher {
		return record, nil
	}
	if principal.Role != auth.RoleStudent ||
		record.Status != StatusPublished ||
		(record.Kind != agent.SkillCourseCards && record.Kind != agent.SkillExercises) {
		return ContentRecord{}, auth.ErrNotFound
	}
	record.CreatedBy = ""
	record.ConfirmedBy = ""
	record.PublishedBy = ""
	record.LastReview = ReviewRun{}
	record.Usage = agent.ProductionUsage{}
	record.Revision = 0
	record.Evidence = nil
	record.FrozenPayload = nil
	if record.Kind == agent.SkillExercises {
		redactExerciseAnswers(record.Payload)
	}
	return record, nil
}

func (s *Service) AuditEvents(
	ctx context.Context,
	principal auth.Principal,
) ([]AuditEvent, error) {
	started := s.now()
	if err := requireTeacher(principal); err != nil {
		s.audit(ctx, principal, "content.audit.query", "", "", "rejected", AuditUsage{}, started, "")
		return nil, err
	}
	events, err := s.repository.ListAudit(ctx, principal.ClassID)
	if err != nil {
		s.audit(ctx, principal, "content.audit.query", "", "", "failed", AuditUsage{}, started, "")
		return nil, err
	}
	s.audit(ctx, principal, "content.audit.query", "", "", "succeeded", AuditUsage{}, started, "")
	return events, nil
}

func (s *Service) scopedRecord(
	ctx context.Context,
	principal auth.Principal,
	contentID string,
) (ContentRecord, error) {
	if contentID == "" {
		return ContentRecord{}, auth.ErrNotFound
	}
	record, ok, err := s.repository.Get(ctx, contentID)
	if err != nil {
		return ContentRecord{}, err
	}
	if !ok || record.ClassID != principal.ClassID {
		return ContentRecord{}, auth.ErrNotFound
	}
	return record, nil
}

func (s *Service) requireEnabled(
	ctx context.Context,
	classID string,
	skill agent.ProductionSkill,
) error {
	enabled, err := s.skills.Enabled(ctx, classID, skill)
	if err != nil {
		return fmt.Errorf("%w: skill gate", ErrSkillGateUnavailable)
	}
	if !enabled {
		return ErrSkillDisabled
	}
	return nil
}

func (s *Service) resolveEvidence(
	ctx context.Context,
	classID string,
	refs []EvidenceRef,
) ([]agent.GroundingSource, error) {
	sources, err := s.evidence.Resolve(ctx, classID, refs)
	if err != nil {
		if errors.Is(err, ErrEvidenceNotFound) || errors.Is(err, auth.ErrNotFound) {
			return nil, auth.ErrNotFound
		}
		return nil, fmt.Errorf("%w: resolve sources", ErrEvidenceUnavailable)
	}
	if err := validateEvidence(sources); err != nil {
		return nil, err
	}
	return sources, nil
}

func (s *Service) review(
	ctx context.Context,
	kind agent.ProductionSkill,
	payload map[string]any,
	sources []agent.GroundingSource,
) (agent.QualityReport, error) {
	report, err := s.reviewer.Review(ctx, agent.QualityReviewRequest{
		Skill:   kind,
		Payload: cloneMap(payload),
		Sources: append([]agent.GroundingSource(nil), sources...),
	})
	if err != nil {
		return agent.QualityReport{}, fmt.Errorf("%w: %v", ErrReviewFailed, err)
	}
	hasError := false
	for _, issue := range report.Issues {
		if issue.Severity == agent.SeverityError {
			hasError = true
			break
		}
	}
	if hasError {
		report.Passed = false
	}
	if report.Scope == "" {
		report.Scope = "schema_and_supplied_evidence"
	}
	return report, nil
}

func (s *Service) audit(
	ctx context.Context,
	principal auth.Principal,
	action string,
	objectID string,
	skill agent.ProductionSkill,
	result string,
	usage AuditUsage,
	started time.Time,
	requestID string,
) {
	duration := s.now().Sub(started).Milliseconds()
	if duration < 0 {
		duration = 0
	}
	if err := s.repository.AppendAudit(context.WithoutCancel(ctx), AuditEvent{
		ActorID:        principal.UserID,
		ClassID:        principal.ClassID,
		At:             s.now(),
		Action:         action,
		ObjectID:       objectID,
		Result:         result,
		Skill:          skill,
		Usage:          usage,
		DurationMillis: duration,
		RequestID:      requestID,
	}); err != nil {
		log.Printf("content audit persistence failed action=%s object=%s request_id=%s: %v", action, objectID, requestID, err)
	}
}

func (s *Service) now() time.Time {
	return s.clock().UTC()
}

func requireAuthenticated(principal auth.Principal) error {
	if strings.TrimSpace(principal.UserID) == "" ||
		strings.TrimSpace(principal.ClassID) == "" ||
		(principal.Role != auth.RoleTeacher && principal.Role != auth.RoleStudent) {
		return auth.ErrUnauthorized
	}
	return nil
}

func requireTeacher(principal auth.Principal) error {
	if err := requireAuthenticated(principal); err != nil {
		return err
	}
	if principal.Role != auth.RoleTeacher {
		return auth.ErrForbidden
	}
	return nil
}

func validateProductionRequest(request agent.ProductionRequest) error {
	if err := validateText("topic", request.Topic, 2, 200); err != nil {
		return err
	}
	if containsUnsafeInstruction(request.Topic) {
		return &ValidationError{Field: "topic", Message: "contains unsafe instructions"}
	}
	switch request.Skill {
	case agent.SkillCourseCards:
		if request.Count < 3 || request.Count > 12 {
			return &ValidationError{Field: "count", Message: "must be between 3 and 12"}
		}
	case agent.SkillExercises:
		if request.Count < 1 || request.Count > 20 {
			return &ValidationError{Field: "count", Message: "must be between 1 and 20"}
		}
		if len(request.ExerciseTypes) == 0 || len(request.ExerciseTypes) > 3 {
			return &ValidationError{Field: "exerciseTypes", Message: "must contain one to three supported types"}
		}
		for _, exerciseType := range request.ExerciseTypes {
			if exerciseType != "choice" && exerciseType != "fill" && exerciseType != "short_answer" {
				return &ValidationError{Field: "exerciseTypes", Message: "supported values are choice, fill, and short_answer"}
			}
		}
		if request.Difficulty != "easy" && request.Difficulty != "medium" && request.Difficulty != "hard" {
			return &ValidationError{Field: "difficulty", Message: "must be easy, medium, or hard"}
		}
	case agent.SkillLessonPlan:
		if request.DurationMinutes < 15 || request.DurationMinutes > 240 {
			return &ValidationError{Field: "durationMinutes", Message: "must be between 15 and 240"}
		}
		if len(request.Objectives) == 0 || len(request.Objectives) > 8 {
			return &ValidationError{Field: "objectives", Message: "must contain one to eight objectives"}
		}
		for i, objective := range request.Objectives {
			if err := validateText(fmt.Sprintf("objectives[%d]", i), objective, 2, 300); err != nil {
				return err
			}
			if containsUnsafeInstruction(objective) {
				return &ValidationError{Field: fmt.Sprintf("objectives[%d]", i), Message: "contains unsafe instructions"}
			}
		}
	default:
		return &ValidationError{Field: "kind", Message: "unsupported production skill"}
	}
	return nil
}

func validateEvidenceRefs(refs []EvidenceRef) error {
	if len(refs) > maxEvidenceSources {
		return &ValidationError{Field: "sourceRefs", Message: "too many source references"}
	}
	seen := make(map[string]struct{}, len(refs))
	for i, ref := range refs {
		ref.MaterialID = strings.TrimSpace(ref.MaterialID)
		ref.ChunkID = strings.TrimSpace(ref.ChunkID)
		if !identifierPattern.MatchString(ref.MaterialID) {
			return &ValidationError{Field: fmt.Sprintf("sourceRefs[%d].materialId", i), Message: "invalid identifier"}
		}
		if ref.ChunkID != "" && !identifierPattern.MatchString(ref.ChunkID) {
			return &ValidationError{Field: fmt.Sprintf("sourceRefs[%d].chunkId", i), Message: "invalid identifier"}
		}
		key := ref.MaterialID + "\x00" + ref.ChunkID
		if _, duplicate := seen[key]; duplicate {
			return &ValidationError{Field: "sourceRefs", Message: "duplicate source reference"}
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateEvidence(sources []agent.GroundingSource) error {
	if len(sources) == 0 {
		return auth.ErrNotFound
	}
	if len(sources) > maxEvidenceSources {
		return &ValidationError{Field: "sourceRefs", Message: "resolved evidence exceeds the source limit"}
	}
	total := 0
	seen := make(map[string]struct{}, len(sources))
	for i, source := range sources {
		if source.SourceID == "" || source.SourceVersion == "" || source.ChunkID == "" || strings.TrimSpace(source.Text) == "" {
			return &ValidationError{Field: fmt.Sprintf("sources[%d]", i), Message: "incomplete evidence snapshot"}
		}
		key := source.SourceID + "\x00" + source.SourceVersion + "\x00" + source.ChunkID
		if _, duplicate := seen[key]; duplicate {
			return &ValidationError{Field: "sources", Message: "duplicate evidence snapshot"}
		}
		seen[key] = struct{}{}
		characters := utf8.RuneCountInString(source.Text)
		if characters > maxSourceCharacters {
			return &ValidationError{Field: fmt.Sprintf("sources[%d].text", i), Message: "source chunk is too large"}
		}
		total += characters
		if total > maxEvidenceCharacters {
			return &ValidationError{Field: "sources", Message: "total evidence is too large"}
		}
		if source.ContentHash != agent.ContentHash(source.Text) {
			return &ValidationError{Field: fmt.Sprintf("sources[%d].contentHash", i), Message: "source hash mismatch"}
		}
		if containsUnsafeInstruction(source.Text) {
			return &ValidationError{Field: fmt.Sprintf("sources[%d].text", i), Message: "contains unsafe instructions"}
		}
	}
	return nil
}

func validateProductionOutput(output *agent.ProductionOutput) error {
	if output == nil || output.Payload == nil {
		return errors.New("agent returned an empty payload")
	}
	if output.Usage.Mode == "" ||
		output.Usage.Model == "" ||
		output.Usage.PromptVersion == "" ||
		output.Usage.SkillVersion == "" {
		return errors.New("agent usage metadata is incomplete")
	}
	return validatePayload(output.Payload)
}

func validatePayload(payload map[string]any) error {
	if payload == nil {
		return &ValidationError{Field: "payload", Message: "is required"}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return &ValidationError{Field: "payload", Message: "must be valid JSON data"}
	}
	if len(raw) > maxPayloadBytes {
		return &ValidationError{Field: "payload", Message: "exceeds 256 KiB"}
	}
	if err := checkValueSafety(payload); err != nil {
		return &ValidationError{Field: "payload", Message: "contains unsafe instructions"}
	}
	return nil
}

func checkValueSafety(value any) error {
	switch typed := value.(type) {
	case string:
		if containsUnsafeInstruction(typed) {
			return errors.New("unsafe instructions")
		}
		return nil
	case map[string]any:
		for _, item := range typed {
			if err := checkValueSafety(item); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range typed {
			if err := checkValueSafety(item); err != nil {
				return err
			}
		}
	case []map[string]any:
		for _, item := range typed {
			if err := checkValueSafety(item); err != nil {
				return err
			}
		}
	case []string:
		for _, item := range typed {
			if containsUnsafeInstruction(item) {
				return errors.New("unsafe instructions")
			}
		}
	}
	return nil
}

func containsUnsafeInstruction(text string) bool {
	for _, pattern := range unsafeInputPatterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

func validateText(field, value string, minRunes, maxRunes int) error {
	length := utf8.RuneCountInString(strings.TrimSpace(value))
	if length < minRunes || length > maxRunes {
		return &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("must contain between %d and %d characters", minRunes, maxRunes),
		}
	}
	return nil
}

func validateIdempotencyKey(key string) error {
	if !idempotencyPattern.MatchString(strings.TrimSpace(key)) {
		return &ValidationError{
			Field:   "idempotencyKey",
			Message: "must be 8-128 safe characters",
		}
	}
	return nil
}

func isGeneratableSkill(skill agent.ProductionSkill) bool {
	return skill == agent.SkillCourseCards ||
		skill == agent.SkillExercises ||
		skill == agent.SkillLessonPlan
}

func normalizedStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func idempotencyScope(
	principal auth.Principal,
	skill agent.ProductionSkill,
	key string,
) string {
	return principal.ClassID + "\x00" + principal.UserID + "\x00" + string(skill) + "\x00" + strings.TrimSpace(key)
}

func citationsFromEvidence(sources []agent.GroundingSource) []agent.Citation {
	out := make([]agent.Citation, 0, len(sources))
	for _, source := range sources {
		out = append(out, agent.Citation{
			SourceID:       source.SourceID,
			SourceVersion:  source.SourceVersion,
			ChunkID:        source.ChunkID,
			CharacterStart: source.CharacterStart,
			CharacterEnd:   source.CharacterEnd,
			ContentHash:    source.ContentHash,
			Snippet:        source.Text,
			Score:          1,
		})
	}
	return out
}

func usageFromProduction(usage agent.ProductionUsage) AuditUsage {
	return AuditUsage{
		Mode:            usage.Mode,
		SourceCount:     usage.SourceCount,
		OutputItems:     usage.OutputItems,
		InputCharacters: usage.InputCharacters,
	}
}

func hashJSON(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func payloadEqual(left, right map[string]any) bool {
	leftHash, leftErr := hashJSON(left)
	rightHash, rightErr := hashJSON(right)
	return leftErr == nil && rightErr == nil && leftHash == rightHash
}

func redactExerciseAnswers(payload map[string]any) {
	raw, ok := payload["questions"]
	if !ok {
		return
	}
	switch questions := raw.(type) {
	case []map[string]any:
		for _, question := range questions {
			delete(question, "answer")
			delete(question, "explanation")
		}
	case []any:
		for _, item := range questions {
			if question, ok := item.(map[string]any); ok {
				delete(question, "answer")
				delete(question, "explanation")
			}
		}
	}
}

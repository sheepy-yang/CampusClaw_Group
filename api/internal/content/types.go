// Package content implements Liu Kaiqi's grounded content-production skills
// and the shared teacher confirmation/publication lifecycle.
package content

import (
	"errors"
	"fmt"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/agent"
)

type Status string

const (
	StatusPendingConfirmation Status = "pending_confirmation"
	StatusNeedsRevision       Status = "needs_revision"
	StatusConfirmed           Status = "confirmed"
	StatusPublished           Status = "published"
)

var (
	ErrIdempotencyConflict  = errors.New("idempotency key was already used with a different request")
	ErrInvalidTransition    = errors.New("invalid content lifecycle transition")
	ErrImmutableContent     = errors.New("confirmed or published content cannot be changed")
	ErrQualityReviewFailed  = errors.New("content quality review failed")
	ErrEvidenceNotFound     = errors.New("grounding evidence not found")
	ErrEvidenceUnavailable  = errors.New("grounding evidence dependency unavailable")
	ErrSkillDisabled        = errors.New("content-production skill is disabled")
	ErrSkillGateUnavailable = errors.New("content-production skill gate unavailable")
	ErrConcurrentUpdate     = errors.New("content was updated concurrently")
	ErrGenerationFailed     = errors.New("content generation failed")
	ErrReviewFailed         = errors.New("content quality review dependency failed")
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

type EvidenceRef struct {
	MaterialID string `json:"materialId"`
	ChunkID    string `json:"chunkId,omitempty"`
}

type RequestMeta struct {
	IdempotencyKey string `json:"-"`
	RequestID      string `json:"-"`
}

type CourseCardRequest struct {
	Topic      string        `json:"topic"`
	Count      int           `json:"count"`
	SourceRefs []EvidenceRef `json:"sourceRefs,omitempty"`
	Meta       RequestMeta   `json:"-"`
}

type ExerciseRequest struct {
	Topic         string        `json:"topic"`
	Count         int           `json:"count"`
	ExerciseTypes []string      `json:"exerciseTypes"`
	Difficulty    string        `json:"difficulty"`
	SourceRefs    []EvidenceRef `json:"sourceRefs,omitempty"`
	Meta          RequestMeta   `json:"-"`
}

type LessonPlanRequest struct {
	Topic           string        `json:"topic"`
	DurationMinutes int           `json:"durationMinutes"`
	Objectives      []string      `json:"objectives"`
	SourceRefs      []EvidenceRef `json:"sourceRefs,omitempty"`
	Meta            RequestMeta   `json:"-"`
}

type QualityReviewRequest struct {
	ContentID  string                `json:"contentId,omitempty"`
	Kind       agent.ProductionSkill `json:"kind,omitempty"`
	Payload    map[string]any        `json:"payload,omitempty"`
	SourceRefs []EvidenceRef         `json:"sourceRefs,omitempty"`
	RequestID  string                `json:"-"`
}

type ConfirmRequest struct {
	Payload   map[string]any `json:"payload,omitempty"`
	RequestID string         `json:"-"`
}

type RejectRequest struct {
	Reason    string `json:"reason"`
	RequestID string `json:"-"`
}

type PublishRequest struct {
	IdempotencyKey string `json:"-"`
	RequestID      string `json:"-"`
}

type ReviewRun struct {
	ID        string                `json:"id"`
	ContentID string                `json:"contentId,omitempty"`
	ClassID   string                `json:"classId"`
	ActorID   string                `json:"actorId"`
	Kind      agent.ProductionSkill `json:"kind"`
	Report    agent.QualityReport   `json:"report"`
	Usage     ReviewUsage           `json:"usage"`
	CreatedAt time.Time             `json:"createdAt"`
}

type ReviewUsage struct {
	Mode         string `json:"mode"`
	SourceCount  int    `json:"sourceCount"`
	CheckedItems int    `json:"checkedItems"`
}

type ContentRecord struct {
	ID                  string                `json:"id"`
	ClassID             string                `json:"classId"`
	CreatedBy           string                `json:"createdBy"`
	Kind                agent.ProductionSkill `json:"kind"`
	Status              Status                `json:"status"`
	Version             int                   `json:"version"`
	Revision            int                   `json:"revision"`
	Payload             map[string]any        `json:"payload"`
	Citations           []agent.Citation      `json:"citations"`
	LastReview          ReviewRun             `json:"lastReview"`
	Usage               agent.ProductionUsage `json:"usage"`
	CreatedAt           time.Time             `json:"createdAt"`
	UpdatedAt           time.Time             `json:"updatedAt"`
	ConfirmedBy         string                `json:"confirmedBy,omitempty"`
	ConfirmedAt         *time.Time            `json:"confirmedAt,omitempty"`
	PublishedBy         string                `json:"publishedBy,omitempty"`
	PublishedAt         *time.Time            `json:"publishedAt,omitempty"`
	FrozenDigest        string                `json:"frozenDigest,omitempty"`
	RevisionNote        string                `json:"revisionNote,omitempty"`
	RevisionRequestedBy string                `json:"revisionRequestedBy,omitempty"`
	RevisionRequestedAt *time.Time            `json:"revisionRequestedAt,omitempty"`

	Evidence       []agent.GroundingSource `json:"-"`
	FrozenPayload  map[string]any          `json:"-"`
	IdempotencyKey string                  `json:"-"`
	RequestHash    string                  `json:"-"`
	PublishKey     string                  `json:"-"`
}

type GenerationResult struct {
	ContentID string                `json:"contentId"`
	Status    Status                `json:"status"`
	Kind      agent.ProductionSkill `json:"kind"`
	Version   int                   `json:"version"`
	Replayed  bool                  `json:"replayed"`
	Content   ContentRecord         `json:"content"`
}

type AuditUsage struct {
	Mode            string `json:"mode,omitempty"`
	SourceCount     int    `json:"sourceCount,omitempty"`
	OutputItems     int    `json:"outputItems,omitempty"`
	InputCharacters int    `json:"inputCharacters,omitempty"`
}

// AuditEvent contains the operational fields required by the core OpenSpec.
type AuditEvent struct {
	ActorID        string                `json:"actor"`
	ClassID        string                `json:"classId"`
	At             time.Time             `json:"timestamp"`
	Action         string                `json:"action"`
	ObjectID       string                `json:"object"`
	Result         string                `json:"result"`
	Skill          agent.ProductionSkill `json:"skill"`
	Usage          AuditUsage            `json:"usage"`
	DurationMillis int64                 `json:"durationMs"`
	RequestID      string                `json:"requestId"`
}

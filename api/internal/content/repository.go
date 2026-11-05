package content

import (
	"context"
	"sort"
	"sync"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/agent"
)

type Repository interface {
	LookupIdempotency(
		ctx context.Context,
		idempotencyScope string,
		requestHash string,
	) (ContentRecord, bool, error)
	CreateOrGet(
		ctx context.Context,
		record ContentRecord,
		idempotencyScope string,
		requestHash string,
	) (ContentRecord, bool, error)
	Get(ctx context.Context, id string) (ContentRecord, bool, error)
	Update(ctx context.Context, record ContentRecord, expectedRevision int) (ContentRecord, error)
	AppendAudit(ctx context.Context, event AuditEvent) error
	ListAudit(ctx context.Context, classID string) ([]AuditEvent, error)
}

type idempotencyEntry struct {
	RequestHash string
	ContentID   string
}

// MemoryRepository is an isolated MVP adapter. It deep-copies mutable payloads
// so callers cannot mutate persisted or frozen content through returned maps.
type MemoryRepository struct {
	mu          sync.RWMutex
	records     map[string]ContentRecord
	idempotency map[string]idempotencyEntry
	audit       []AuditEvent
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		records:     make(map[string]ContentRecord),
		idempotency: make(map[string]idempotencyEntry),
	}
}

func (r *MemoryRepository) LookupIdempotency(
	ctx context.Context,
	idempotencyScope string,
	requestHash string,
) (ContentRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return ContentRecord{}, false, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.idempotency[idempotencyScope]
	if !ok {
		return ContentRecord{}, false, nil
	}
	if entry.RequestHash != requestHash {
		return ContentRecord{}, false, ErrIdempotencyConflict
	}
	record, ok := r.records[entry.ContentID]
	if !ok {
		return ContentRecord{}, false, ErrConcurrentUpdate
	}
	return cloneRecord(record), true, nil
}

func (r *MemoryRepository) CreateOrGet(
	ctx context.Context,
	record ContentRecord,
	idempotencyScope string,
	requestHash string,
) (ContentRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return ContentRecord{}, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.idempotency[idempotencyScope]; ok {
		if existing.RequestHash != requestHash {
			return ContentRecord{}, false, ErrIdempotencyConflict
		}
		stored, exists := r.records[existing.ContentID]
		if !exists {
			return ContentRecord{}, false, ErrConcurrentUpdate
		}
		return cloneRecord(stored), true, nil
	}
	if _, exists := r.records[record.ID]; exists {
		return ContentRecord{}, false, ErrConcurrentUpdate
	}
	if record.Revision == 0 {
		record.Revision = 1
	}
	if record.Version == 0 {
		record.Version = 1
	}
	record.IdempotencyKey = idempotencyScope
	record.RequestHash = requestHash
	r.records[record.ID] = cloneRecord(record)
	r.idempotency[idempotencyScope] = idempotencyEntry{
		RequestHash: requestHash,
		ContentID:   record.ID,
	}
	return cloneRecord(record), false, nil
}

func (r *MemoryRepository) Get(ctx context.Context, id string) (ContentRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return ContentRecord{}, false, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	record, ok := r.records[id]
	return cloneRecord(record), ok, nil
}

func (r *MemoryRepository) Update(
	ctx context.Context,
	record ContentRecord,
	expectedRevision int,
) (ContentRecord, error) {
	if err := ctx.Err(); err != nil {
		return ContentRecord{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.records[record.ID]
	if !ok {
		return ContentRecord{}, ErrConcurrentUpdate
	}
	if current.Revision != expectedRevision {
		return ContentRecord{}, ErrConcurrentUpdate
	}
	if current.ClassID != record.ClassID ||
		current.CreatedBy != record.CreatedBy ||
		current.Kind != record.Kind ||
		!current.CreatedAt.Equal(record.CreatedAt) {
		return ContentRecord{}, ErrImmutableContent
	}
	record.Revision = expectedRevision + 1
	r.records[record.ID] = cloneRecord(record)
	return cloneRecord(record), nil
}

func (r *MemoryRepository) AppendAudit(ctx context.Context, event AuditEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.audit = append(r.audit, event)
	return nil
}

func (r *MemoryRepository) ListAudit(ctx context.Context, classID string) ([]AuditEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]AuditEvent, 0, len(r.audit))
	for _, event := range r.audit {
		if classID == "" || event.ClassID == classID {
			out = append(out, event)
		}
	}
	return out, nil
}

// List is intended for deterministic tests and future persistence adapters.
func (r *MemoryRepository) List(ctx context.Context, classID string) ([]ContentRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.records))
	for id, record := range r.records {
		if classID == "" || record.ClassID == classID {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	out := make([]ContentRecord, 0, len(ids))
	for _, id := range ids {
		out = append(out, cloneRecord(r.records[id]))
	}
	return out, nil
}

func cloneRecord(record ContentRecord) ContentRecord {
	record.Payload = cloneMap(record.Payload)
	record.FrozenPayload = cloneMap(record.FrozenPayload)
	record.Citations = append([]agent.Citation(nil), record.Citations...)
	record.Evidence = append([]agent.GroundingSource(nil), record.Evidence...)
	record.LastReview.Report.Issues = append(
		[]agent.QualityIssue(nil),
		record.LastReview.Report.Issues...,
	)
	return record
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = cloneValue(item)
	}
	return out
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = cloneValue(item)
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, len(typed))
		for i, item := range typed {
			out[i] = cloneMap(item)
		}
		return out
	case []string:
		return append([]string(nil), typed...)
	case []int:
		return append([]int(nil), typed...)
	case []float64:
		return append([]float64(nil), typed...)
	default:
		return typed
	}
}

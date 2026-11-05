package content

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/agent"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

type StoreRepository struct {
	memory *store.Memory
	mu     sync.Mutex
}

type persistedRecord struct {
	ContentRecord
	Evidence       []agent.GroundingSource `json:"evidence"`
	FrozenPayload  map[string]any          `json:"frozenPayload,omitempty"`
	IdempotencyKey string                  `json:"idempotencyKey"`
	RequestHash    string                  `json:"requestHash"`
	PublishKey     string                  `json:"publishKey,omitempty"`
}

func NewStoreRepository(memory *store.Memory) *StoreRepository {
	if memory == nil {
		memory = store.Default
	}
	return &StoreRepository{memory: memory}
}

func contentIdempotencyTask(scope string) string {
	return "content-idem-" + store.Digest(scope)[:32]
}

func (r *StoreRepository) LookupIdempotency(_ context.Context, scope, requestHash string) (ContentRecord, bool, error) {
	task, ok := r.memory.GetTask(contentIdempotencyTask(scope))
	if !ok {
		return ContentRecord{}, false, nil
	}
	if task.PayloadHash != requestHash {
		return ContentRecord{}, false, ErrIdempotencyConflict
	}
	record, ok, err := r.Get(context.Background(), task.ObjectID)
	return record, ok, err
}

func (r *StoreRepository) CreateOrGet(
	ctx context.Context,
	record ContentRecord,
	scope string,
	requestHash string,
) (ContentRecord, bool, error) {
	taskID := contentIdempotencyTask(scope)
	task, duplicate, err := r.memory.ClaimIdempotency(scope, "content", "generation", requestHash, taskID)
	if errors.Is(err, store.ErrConflict) {
		return ContentRecord{}, false, ErrIdempotencyConflict
	}
	if err != nil {
		return ContentRecord{}, false, err
	}
	if duplicate {
		existing, ok, err := r.Get(ctx, task.ObjectID)
		if err != nil {
			return ContentRecord{}, false, err
		}
		if !ok {
			return ContentRecord{}, false, ErrConcurrentUpdate
		}
		return existing, true, nil
	}
	if record.Revision == 0 {
		record.Revision = 1
	}
	if record.Version == 0 {
		record.Version = 1
	}
	record.IdempotencyKey = scope
	record.RequestHash = requestHash
	if err := r.save(record); err != nil {
		return ContentRecord{}, false, err
	}
	task.ClassID = record.ClassID
	task.UserID = record.CreatedBy
	task.ObjectID = record.ID
	task.Skill = string(record.Kind)
	task.Status = "succeeded"
	r.memory.SaveTask(task)
	if err := r.memory.EnsureTaskDurable(task.ID); err != nil {
		return ContentRecord{}, false, err
	}
	return cloneRecord(record), false, nil
}

func (r *StoreRepository) Get(_ context.Context, id string) (ContentRecord, bool, error) {
	raw, ok := r.memory.GetContent(id)
	if !ok || raw.Payload == nil {
		return ContentRecord{}, false, nil
	}
	value, ok := raw.Payload["contentProductionRecord"]
	if !ok {
		return ContentRecord{}, false, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ContentRecord{}, false, err
	}
	var persisted persistedRecord
	if err := json.Unmarshal(encoded, &persisted); err != nil {
		return ContentRecord{}, false, err
	}
	record := persisted.ContentRecord
	record.Evidence = persisted.Evidence
	record.FrozenPayload = persisted.FrozenPayload
	record.IdempotencyKey = persisted.IdempotencyKey
	record.RequestHash = persisted.RequestHash
	record.PublishKey = persisted.PublishKey
	return cloneRecord(record), true, nil
}

func (r *StoreRepository) Update(ctx context.Context, record ContentRecord, expectedRevision int) (ContentRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok, err := r.Get(ctx, record.ID)
	if err != nil {
		return ContentRecord{}, err
	}
	if !ok || current.Revision != expectedRevision {
		return ContentRecord{}, ErrConcurrentUpdate
	}
	if current.ClassID != record.ClassID || current.CreatedBy != record.CreatedBy || current.Kind != record.Kind || !current.CreatedAt.Equal(record.CreatedAt) {
		return ContentRecord{}, ErrImmutableContent
	}
	record.Revision = expectedRevision + 1
	if err := r.save(record); err != nil {
		return ContentRecord{}, err
	}
	return cloneRecord(record), nil
}

func (r *StoreRepository) save(record ContentRecord) error {
	persisted := persistedRecord{
		ContentRecord: record, Evidence: record.Evidence, FrozenPayload: record.FrozenPayload,
		IdempotencyKey: record.IdempotencyKey, RequestHash: record.RequestHash, PublishKey: record.PublishKey,
	}
	encoded, err := json.Marshal(persisted)
	if err != nil {
		return err
	}
	var value map[string]any
	if err := json.Unmarshal(encoded, &value); err != nil {
		return err
	}
	r.memory.SaveContent(store.ContentRecord{
		ID: record.ID, Skill: string(record.Kind), ClassID: record.ClassID, Status: string(record.Status),
		Payload:   map[string]any{"contentProductionRecord": value},
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	})
	return r.memory.EnsureContentDurable(record.ID)
}

func (r *StoreRepository) AppendAudit(_ context.Context, event AuditEvent) error {
	return r.memory.AppendAudit(store.AuditEvent{
		ID: store.NewID("audit"), ActorID: event.ActorID, ClassID: event.ClassID,
		Action: event.Action, Object: event.ObjectID, Result: event.Result, Skill: string(event.Skill),
		DurationMS: event.DurationMillis, RequestID: event.RequestID, CreatedAt: event.At,
		Detail: map[string]any{"usage": event.Usage},
	})
}

func (r *StoreRepository) ListAudit(_ context.Context, classID string) ([]AuditEvent, error) {
	raw := r.memory.ListAudit(classID)
	out := make([]AuditEvent, 0, len(raw))
	for _, event := range raw {
		if event.Skill == "" {
			continue
		}
		out = append(out, AuditEvent{
			ActorID: event.ActorID, ClassID: event.ClassID, At: event.CreatedAt,
			Action: event.Action, ObjectID: event.Object, Result: event.Result,
			Skill: agent.ProductionSkill(event.Skill), DurationMillis: event.DurationMS, RequestID: event.RequestID,
		})
	}
	return out, nil
}

var _ Repository = (*StoreRepository)(nil)

func NewSharedStoreEvidenceProvider(memory *store.Memory) *StoreEvidenceProvider {
	if memory == nil {
		memory = store.Default
	}
	return NewStoreEvidenceProvider(MaterialReaderFunc(func(classID string) []Material {
		stored := memory.MaterialsByClass(classID)
		materials := make([]Material, 0, len(stored))
		for _, item := range stored {
			chunks := make([]MaterialChunk, 0, len(item.Chunks))
			for _, chunk := range item.Chunks {
				chunks = append(chunks, MaterialChunk{
					ID: chunk.ID, Text: chunk.Text,
				})
			}
			materials = append(materials, Material{
				ID: item.ID, ClassID: item.ClassID, Content: item.Content, Chunks: chunks,
			})
		}
		return materials
	}))
}

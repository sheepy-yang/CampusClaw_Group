package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisPrefix = "campusclaw:v1:"

type RedisPersistence struct {
	client *redis.Client
}

type persistence interface {
	enabled() bool
	save(kind, id string, value any, indexes ...string) error
	load(kind, id string, dst any) error
	list(kind, index string, factory func() any) ([]any, error)
	markJobDone(key string) error
	jobDone(key string) (bool, error)
	claimIdempotency(composite, payloadHash, taskID string) (string, bool, error)
	releaseIdempotency(composite string)
}

type noopPersistence struct{}

func (noopPersistence) enabled() bool                                  { return false }
func (noopPersistence) save(string, string, any, ...string) error      { return nil }
func (noopPersistence) load(string, string, any) error                 { return ErrNotFound }
func (noopPersistence) list(string, string, func() any) ([]any, error) { return nil, nil }
func (noopPersistence) markJobDone(string) error                       { return nil }
func (noopPersistence) jobDone(string) (bool, error)                   { return false, nil }
func (noopPersistence) claimIdempotency(string, string, string) (string, bool, error) {
	return "", false, nil
}
func (noopPersistence) releaseIdempotency(string) {}

func (m *Memory) persistenceStore() persistence {
	if m == nil || m.persistence == nil {
		return noopPersistence{}
	}
	return m.persistence
}

func NewPersistenceFromEnv() persistence {
	if sqlPersistence := NewSQLPersistenceFromEnv(); sqlPersistence != nil {
		return sqlPersistence
	}
	rawURL := strings.TrimSpace(os.Getenv("REDIS_URL"))
	if rawURL == "" {
		if address := strings.TrimSpace(os.Getenv("REDIS_ADDR")); address != "" {
			rawURL = "redis://" + address + "/0"
		}
	}
	if rawURL == "" {
		return noopPersistence{}
	}
	options, err := redis.ParseURL(rawURL)
	if err != nil {
		return noopPersistence{}
	}
	return &RedisPersistence{client: redis.NewClient(options)}
}

func (p *RedisPersistence) enabled() bool { return p != nil }

func (p *RedisPersistence) save(kind, id string, value any, indexes ...string) error {
	if p == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	pipe := p.client.TxPipeline()
	pipe.Set(ctx, redisPrefix+kind+":"+id, encoded, 0)
	for _, index := range indexes {
		if index != "" {
			pipe.SAdd(ctx, redisPrefix+"index:"+kind+":"+index, id)
		}
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (p *RedisPersistence) load(kind, id string, dst any) error {
	if p == nil {
		return ErrNotFound
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	encoded, err := p.client.Get(ctx, redisPrefix+kind+":"+id).Bytes()
	if errors.Is(err, redis.Nil) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, dst)
}

func (p *RedisPersistence) list(kind, index string, factory func() any) ([]any, error) {
	if p == nil {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ids, err := p.client.SMembers(ctx, redisPrefix+"index:"+kind+":"+index).Result()
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(ids))
	for _, id := range ids {
		value := factory()
		if err := p.load(kind, id, value); err == nil {
			out = append(out, value)
		}
	}
	return out, nil
}

func (p *RedisPersistence) markJobDone(key string) error {
	if p == nil || key == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return p.client.Set(ctx, redisPrefix+"job-done:"+Digest(key), "1", 7*24*time.Hour).Err()
}

func (p *RedisPersistence) jobDone(key string) (bool, error) {
	if p == nil || key == "" {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	count, err := p.client.Exists(ctx, redisPrefix+"job-done:"+Digest(key)).Result()
	return count > 0, err
}

type idempotencyRecord struct {
	TaskID      string `json:"taskId"`
	PayloadHash string `json:"payloadHash"`
}

func (p *RedisPersistence) claimIdempotency(composite, payloadHash, taskID string) (string, bool, error) {
	if p == nil {
		return "", false, nil
	}
	key := redisPrefix + "idempotency:" + Digest(composite)
	record := idempotencyRecord{TaskID: taskID, PayloadHash: payloadHash}
	encoded, err := json.Marshal(record)
	if err != nil {
		return "", false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	created, err := p.client.SetNX(ctx, key, encoded, 7*24*time.Hour).Result()
	if err != nil {
		return "", false, err
	}
	if created {
		return taskID, false, nil
	}
	raw, err := p.client.Get(ctx, key).Bytes()
	if err != nil {
		return "", true, err
	}
	var existing idempotencyRecord
	if err := json.Unmarshal(raw, &existing); err != nil {
		return "", true, err
	}
	if existing.PayloadHash != payloadHash {
		return existing.TaskID, true, ErrConflict
	}
	return existing.TaskID, true, nil
}

func (p *RedisPersistence) releaseIdempotency(composite string) {
	if p == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.client.Del(ctx, redisPrefix+"idempotency:"+Digest(composite)).Err()
}

func (m *Memory) EnsureTaskDurable(id string) error {
	if !m.persistenceStore().enabled() {
		return nil
	}
	m.mu.RLock()
	task, ok := m.Tasks[id]
	m.mu.RUnlock()
	if !ok {
		return ErrNotFound
	}
	if err := m.persistenceStore().save("task", task.ID, task, task.ClassID, "user:"+task.UserID); err != nil {
		m.mu.Lock()
		delete(m.Tasks, id)
		m.mu.Unlock()
		return err
	}
	return nil
}

func (m *Memory) EnsureSubmissionDurable(id string) error {
	if !m.persistenceStore().enabled() {
		return nil
	}
	m.mu.RLock()
	submission, ok := m.Submissions[id]
	m.mu.RUnlock()
	if !ok {
		return ErrNotFound
	}
	if err := m.persistenceStore().save(
		"submission", submission.ID, submission, submission.ClassID,
		"assignment:"+submission.AssignmentID, "student:"+submission.StudentID,
	); err != nil {
		m.mu.Lock()
		delete(m.Submissions, id)
		m.mu.Unlock()
		return err
	}
	return nil
}

func (m *Memory) EnsureAssignmentDurable(id string) error {
	if !m.persistenceStore().enabled() {
		return nil
	}
	m.mu.RLock()
	assignment, ok := m.Assignments[id]
	m.mu.RUnlock()
	if !ok {
		return ErrNotFound
	}
	if err := m.persistenceStore().save("assignment", assignment.ID, assignment, assignment.ClassID); err != nil {
		m.mu.Lock()
		delete(m.Assignments, id)
		m.mu.Unlock()
		return err
	}
	return nil
}

func (m *Memory) EnsureMaterialDurable(id string) error {
	if !m.persistenceStore().enabled() {
		return nil
	}
	m.mu.RLock()
	material, ok := m.Materials[id]
	m.mu.RUnlock()
	if !ok {
		return ErrNotFound
	}
	if err := m.persistenceStore().save("material", material.ID, material, material.ClassID); err != nil {
		m.mu.Lock()
		delete(m.Materials, id)
		m.mu.Unlock()
		return err
	}
	return nil
}

func (m *Memory) EnsureConversationDurable(id string) error {
	if !m.persistenceStore().enabled() {
		return nil
	}
	m.mu.RLock()
	conversation, ok := m.Conversations[id]
	m.mu.RUnlock()
	if !ok {
		return ErrNotFound
	}
	if err := m.persistenceStore().save("conversation", conversation.ID, conversation, conversation.ClassID, "user:"+conversation.UserID); err != nil {
		m.mu.Lock()
		delete(m.Conversations, id)
		m.mu.Unlock()
		return err
	}
	return nil
}

func (m *Memory) EnsureAssistantDurable(classID string) error {
	if !m.persistenceStore().enabled() {
		return nil
	}
	m.mu.RLock()
	assistant, ok := m.Assistants[classID]
	m.mu.RUnlock()
	if !ok {
		return ErrNotFound
	}
	if err := m.persistenceStore().save("assistant", assistant.ClassID, assistant, assistant.ClassID); err != nil {
		m.mu.Lock()
		delete(m.Assistants, classID)
		m.mu.Unlock()
		return err
	}
	return nil
}

func (m *Memory) EnsureContentDurable(id string) error {
	if !m.persistenceStore().enabled() {
		return nil
	}
	m.mu.RLock()
	content, ok := m.Contents[id]
	m.mu.RUnlock()
	if !ok {
		return ErrNotFound
	}
	if err := m.persistenceStore().save("content", content.ID, content, content.ClassID, "skill:"+content.Skill); err != nil {
		m.mu.Lock()
		delete(m.Contents, id)
		m.mu.Unlock()
		return err
	}
	return nil
}

func (m *Memory) EnsureSimilarityDurable(attemptID string) error {
	if !m.persistenceStore().enabled() {
		return nil
	}
	m.mu.RLock()
	hits, ok := m.Similarity[attemptID]
	m.mu.RUnlock()
	if !ok {
		return ErrNotFound
	}
	if err := m.persistenceStore().save("similarity", attemptID, hits); err != nil {
		m.mu.Lock()
		delete(m.Similarity, attemptID)
		m.mu.Unlock()
		return err
	}
	return nil
}

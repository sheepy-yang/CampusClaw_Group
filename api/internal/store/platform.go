package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

func Digest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// ChunkMaterial creates immutable, traceable chunks using rune offsets.
func ChunkMaterial(materialID, version, title, content string) []Chunk {
	runes := []rune(strings.TrimSpace(content))
	if len(runes) == 0 {
		return nil
	}
	const (
		maxRunes = 320
		minRunes = 120
		overlap  = 40
	)
	var chunks []Chunk
	for start := 0; start < len(runes); {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		} else {
			for candidate := end; candidate >= start+minRunes; candidate-- {
				if isBoundary(runes[candidate-1]) {
					end = candidate
					break
				}
			}
		}
		text := strings.TrimSpace(string(runes[start:end]))
		if text != "" {
			index := len(chunks)
			chunks = append(chunks, Chunk{
				ID:              materialID + "-chunk-" + itoa(index+1),
				MaterialID:      materialID,
				MaterialVersion: version,
				Index:           index,
				CharacterStart:  start,
				CharacterEnd:    end,
				ContentHash:     Digest(text),
				Text:            text,
				Source:          title + "§" + itoa(index+1),
			})
		}
		if end == len(runes) {
			break
		}
		next := end - overlap
		if next <= start {
			next = end
		}
		start = next
	}
	return chunks
}

func isBoundary(r rune) bool {
	switch r {
	case '。', '！', '？', '；', '\n':
		return true
	default:
		return false
	}
}

func (m *Memory) SaveMaterial(material Material) {
	m.mu.Lock()
	m.Materials[material.ID] = material
	m.mu.Unlock()
	_ = m.persistenceStore().save("material", material.ID, material, material.ClassID)
}

func (m *Memory) GetMaterial(id string) (Material, bool) {
	m.mu.RLock()
	material, ok := m.Materials[id]
	m.mu.RUnlock()
	if ok {
		return material, true
	}
	if err := m.persistenceStore().load("material", id, &material); err != nil {
		return Material{}, false
	}
	m.mu.Lock()
	m.Materials[id] = material
	m.mu.Unlock()
	return material, true
}

func (m *Memory) SearchChunks(classID, query string, limit int) []Chunk {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	if limit < 1 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}
	queryTerms := terms(query)

	hits := make([]Chunk, 0)
	for _, material := range m.MaterialsByClass(classID) {
		for _, original := range material.Chunks {
			hit := original
			hit.KeywordScore = overlapScore(queryTerms, terms(hit.Text))
			hit.VectorScore = 0
			hit.FusionScore = hit.KeywordScore
			if strings.Contains(strings.ToLower(hit.Text), strings.ToLower(query)) {
				hit.FusionScore += 0.35
			}
			if hit.FusionScore >= 0.12 {
				hits = append(hits, hit)
			}
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].FusionScore == hits[j].FusionScore {
			return hits[i].ID < hits[j].ID
		}
		return hits[i].FusionScore > hits[j].FusionScore
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

func (m *Memory) ChunksByClass(classID string) []Chunk {
	out := make([]Chunk, 0)
	for _, material := range m.MaterialsByClass(classID) {
		out = append(out, material.Chunks...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func terms(text string) map[string]struct{} {
	runes := []rune(strings.ToLower(text))
	out := make(map[string]struct{})
	for _, r := range runes {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		out[string(r)] = struct{}{}
	}
	for i := 0; i+1 < len(runes); i++ {
		if unicode.IsSpace(runes[i]) || unicode.IsSpace(runes[i+1]) {
			continue
		}
		out[string(runes[i:i+2])] = struct{}{}
	}
	return out
}

func overlapScore(left, right map[string]struct{}) float64 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	intersection := 0
	for term := range left {
		if _, ok := right[term]; ok {
			intersection++
		}
	}
	return float64(intersection) / math.Sqrt(float64(len(left)*len(right)))
}

func vector(text string) []float64 {
	const dimensions = 64
	out := make([]float64, dimensions)
	for term := range terms(text) {
		sum := sha256.Sum256([]byte(term))
		index := int(sum[0]) % dimensions
		sign := 1.0
		if sum[1]&1 == 1 {
			sign = -1
		}
		out[index] += sign
	}
	return out
}

func cosine(left, right []float64) float64 {
	var dot, leftNorm, rightNorm float64
	for i := range left {
		dot += left[i] * right[i]
		leftNorm += left[i] * left[i]
		rightNorm += right[i] * right[i]
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	score := dot / math.Sqrt(leftNorm*rightNorm)
	if score < 0 {
		return 0
	}
	return score
}

func (m *Memory) GetAssistant(classID string) (AssistantConfig, bool) {
	m.mu.RLock()
	assistant, ok := m.Assistants[classID]
	m.mu.RUnlock()
	if ok {
		return assistant, true
	}
	if err := m.persistenceStore().load("assistant", classID, &assistant); err != nil {
		return AssistantConfig{}, false
	}
	m.mu.Lock()
	m.Assistants[classID] = assistant
	m.mu.Unlock()
	return assistant, true
}

func (m *Memory) SaveAssistant(assistant AssistantConfig) {
	m.mu.Lock()
	m.Assistants[assistant.ClassID] = assistant
	m.mu.Unlock()
	_ = m.persistenceStore().save("assistant", assistant.ClassID, assistant, assistant.ClassID)
}

func (m *Memory) SaveConversation(conversation Conversation) {
	m.mu.Lock()
	m.Conversations[conversation.ID] = conversation
	m.mu.Unlock()
	_ = m.persistenceStore().save("conversation", conversation.ID, conversation, conversation.ClassID, "user:"+conversation.UserID)
}

func (m *Memory) GetConversation(id string) (Conversation, bool) {
	m.mu.RLock()
	conversation, ok := m.Conversations[id]
	m.mu.RUnlock()
	if ok {
		return conversation, true
	}
	if err := m.persistenceStore().load("conversation", id, &conversation); err != nil {
		return Conversation{}, false
	}
	m.mu.Lock()
	m.Conversations[id] = conversation
	m.mu.Unlock()
	return conversation, true
}

func (m *Memory) ListConversations(classID, userID string) []Conversation {
	m.mu.RLock()
	out := make([]Conversation, 0)
	for _, conversation := range m.Conversations {
		if conversation.ClassID == classID && (userID == "" || conversation.UserID == userID) {
			out = append(out, conversation)
		}
	}
	m.mu.RUnlock()
	index := classID
	if userID != "" {
		index = "user:" + userID
	}
	if values, err := m.persistenceStore().list("conversation", index, func() any { return &Conversation{} }); err == nil {
		seen := map[string]bool{}
		for _, conversation := range out {
			seen[conversation.ID] = true
		}
		for _, value := range values {
			conversation := *(value.(*Conversation))
			if !seen[conversation.ID] && conversation.ClassID == classID && (userID == "" || conversation.UserID == userID) {
				out = append(out, conversation)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

func (m *Memory) SaveAssignment(assignment Assignment) {
	m.mu.Lock()
	m.Assignments[assignment.ID] = assignment
	m.mu.Unlock()
	_ = m.persistenceStore().save("assignment", assignment.ID, assignment, assignment.ClassID)
}

func (m *Memory) GetAssignment(id string) (Assignment, bool) {
	m.mu.RLock()
	assignment, ok := m.Assignments[id]
	m.mu.RUnlock()
	if ok {
		return assignment, true
	}
	if err := m.persistenceStore().load("assignment", id, &assignment); err != nil {
		return Assignment{}, false
	}
	m.mu.Lock()
	m.Assignments[id] = assignment
	m.mu.Unlock()
	return assignment, true
}

func (m *Memory) ListAssignments(classID string) []Assignment {
	m.mu.RLock()
	out := make([]Assignment, 0)
	for _, assignment := range m.Assignments {
		if assignment.ClassID == classID {
			out = append(out, assignment)
		}
	}
	m.mu.RUnlock()
	if values, err := m.persistenceStore().list("assignment", classID, func() any { return &Assignment{} }); err == nil {
		seen := map[string]bool{}
		for _, assignment := range out {
			seen[assignment.ID] = true
		}
		for _, value := range values {
			assignment := *(value.(*Assignment))
			if !seen[assignment.ID] {
				out = append(out, assignment)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (m *Memory) ClaimIdempotency(actorID, operation, key, payloadHash, taskID string) (TaskRecord, bool, error) {
	composite := actorID + "\x00" + operation + "\x00" + key
	if m.persistenceStore().enabled() {
		existingID, duplicate, err := m.persistenceStore().claimIdempotency(composite, payloadHash, taskID)
		if err != nil {
			return TaskRecord{}, duplicate, err
		}
		if duplicate {
			existing, ok := m.GetTask(existingID)
			if !ok {
				return TaskRecord{}, true, errors.New("idempotency task is unavailable")
			}
			return existing, true, nil
		}
		now := time.Now().UTC()
		task := TaskRecord{
			ID: taskID, UserID: actorID, ObjectID: operation, Status: "pending",
			IdempotencyKey: key, PayloadHash: payloadHash, CreatedAt: now, UpdatedAt: now,
		}
		if err := m.persistenceStore().save("task", task.ID, task, "user:"+actorID); err != nil {
			m.persistenceStore().releaseIdempotency(composite)
			return TaskRecord{}, false, err
		}
		m.mu.Lock()
		m.Idempotency[composite] = taskID
		m.Tasks[taskID] = task
		m.mu.Unlock()
		return task, false, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if existingID, ok := m.Idempotency[composite]; ok {
		existing, exists := m.Tasks[existingID]
		if !exists {
			delete(m.Idempotency, composite)
		} else if existing.PayloadHash != payloadHash {
			return TaskRecord{}, true, ErrConflict
		} else {
			return existing, true, nil
		}
	}
	now := time.Now().UTC()
	task := TaskRecord{
		ID: taskID, UserID: actorID, ObjectID: operation, Status: "pending",
		IdempotencyKey: key, PayloadHash: payloadHash, CreatedAt: now, UpdatedAt: now,
	}
	m.Idempotency[composite] = taskID
	m.Tasks[taskID] = task
	return task, false, nil
}

func (m *Memory) AppendMistake(mistake Mistake) error {
	m.mu.RLock()
	for _, existing := range m.Mistakes {
		if mistake.SubmissionID != "" && existing.SubmissionID == mistake.SubmissionID {
			m.mu.RUnlock()
			return nil
		}
	}
	m.mu.RUnlock()
	if err := m.persistenceStore().save("mistake", mistake.ID, mistake, mistake.ClassID, "student:"+mistake.StudentID, "submission:"+mistake.SubmissionID); err != nil {
		return err
	}
	m.mu.Lock()
	for _, existing := range m.Mistakes {
		if mistake.SubmissionID != "" && existing.SubmissionID == mistake.SubmissionID {
			m.mu.Unlock()
			return nil
		}
	}
	m.Mistakes = append(m.Mistakes, mistake)
	m.mu.Unlock()
	return nil
}

func (m *Memory) MistakeBySubmission(submissionID string) (Mistake, bool) {
	m.mu.RLock()
	for _, mistake := range m.Mistakes {
		if mistake.SubmissionID == submissionID {
			m.mu.RUnlock()
			return mistake, true
		}
	}
	m.mu.RUnlock()
	values, err := m.persistenceStore().list("mistake", "submission:"+submissionID, func() any { return &Mistake{} })
	if err != nil || len(values) == 0 {
		return Mistake{}, false
	}
	mistake := *(values[0].(*Mistake))
	m.mu.Lock()
	m.Mistakes = append(m.Mistakes, mistake)
	m.mu.Unlock()
	return mistake, true
}

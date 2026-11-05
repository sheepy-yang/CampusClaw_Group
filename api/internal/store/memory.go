// Package store 提供演示用内存数据层，便于无 Docker 依赖时跑通主路径。
package store

import (
	"errors"
	"os"
	"strings"
	"sync"
	"time"
)

type Material struct {
	ID             string    `json:"id"`
	ClassID        string    `json:"classId"`
	Title          string    `json:"title"`
	Content        string    `json:"content,omitempty"`
	Version        string    `json:"version"`
	ContentHash    string    `json:"contentHash"`
	IndexStatus    string    `json:"indexStatus"`
	EmbeddingModel string    `json:"embeddingModel,omitempty"`
	CreatedBy      string    `json:"createdBy"`
	CreatedAt      time.Time `json:"createdAt"`
	Chunks         []Chunk   `json:"chunks,omitempty"`
}

type Chunk struct {
	ID              string  `json:"id"`
	MaterialID      string  `json:"materialId"`
	MaterialVersion string  `json:"materialVersion"`
	Index           int     `json:"index"`
	CharacterStart  int     `json:"characterStart"`
	CharacterEnd    int     `json:"characterEnd"`
	ContentHash     string  `json:"contentHash"`
	Text            string  `json:"text"`
	Source          string  `json:"source"`
	KeywordScore    float64 `json:"keywordScore,omitempty"`
	VectorScore     float64 `json:"vectorScore,omitempty"`
	FusionScore     float64 `json:"fusionScore,omitempty"`
}

type AssistantConfig struct {
	ID            string    `json:"id"`
	ClassID       string    `json:"classId"`
	Name          string    `json:"name"`
	SystemPrompt  string    `json:"systemPrompt"`
	EnabledSkills []string  `json:"enabledSkills"`
	UpdatedBy     string    `json:"updatedBy"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type Conversation struct {
	ID          string                `json:"id"`
	ClassID     string                `json:"classId"`
	UserID      string                `json:"userId"`
	AssistantID string                `json:"assistantId"`
	Title       string                `json:"title"`
	Messages    []ConversationMessage `json:"messages"`
	CreatedAt   time.Time             `json:"createdAt"`
	UpdatedAt   time.Time             `json:"updatedAt"`
}

type ConversationMessage struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Citations []Chunk   `json:"citations,omitempty"`
	Tool      string    `json:"tool,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type Assignment struct {
	ID          string     `json:"id"`
	ClassID     string     `json:"classId"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Status      string     `json:"status"`
	MaxScore    float64    `json:"maxScore"`
	Items       []Question `json:"items"`
	CreatedBy   string     `json:"createdBy"`
	CreatedAt   time.Time  `json:"createdAt"`
	PublishedAt time.Time  `json:"publishedAt,omitempty"`
}

type Question struct {
	ID             string           `json:"id"`
	Stem           string           `json:"stem"`
	Type           string           `json:"type"`
	KnowledgePoint string           `json:"knowledgePoint"`
	Difficulty     string           `json:"difficulty"`
	Answer         string           `json:"answer,omitempty"`
	Explanation    string           `json:"explanation,omitempty"`
	Citations      []map[string]any `json:"citations,omitempty"`
	Status         string           `json:"status"`
	Adopted        bool             `json:"adopted"`
}

type ContentRecord struct {
	ID        string         `json:"id"`
	Skill     string         `json:"skill"`
	ClassID   string         `json:"classId"`
	Status    string         `json:"status"`
	Payload   map[string]any `json:"payload"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

type TaskRecord struct {
	ID             string     `json:"id"`
	Skill          string     `json:"skill"`
	ClassID        string     `json:"classId"`
	UserID         string     `json:"userId,omitempty"`
	ObjectID       string     `json:"objectId,omitempty"`
	Status         string     `json:"status"`
	Error          string     `json:"error,omitempty"`
	IdempotencyKey string     `json:"idempotencyKey,omitempty"`
	PayloadHash    string     `json:"payloadHash,omitempty"`
	Steps          []TaskStep `json:"steps"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type TaskStep struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type Submission struct {
	ID             string            `json:"id"`
	AssignmentID   string            `json:"assignmentId"`
	StudentID      string            `json:"studentId"`
	ClassID        string            `json:"classId"`
	AnswerText     string            `json:"answerText"`
	Answers        map[string]string `json:"answers,omitempty"`
	Transcription  string            `json:"transcription,omitempty"`
	Confirmed      bool              `json:"transcriptionConfirmed"`
	Steps          []map[string]any  `json:"steps,omitempty"`
	Score          *float64          `json:"score,omitempty"`
	Suggested      *float64          `json:"suggestedScore,omitempty"`
	MaxScore       float64           `json:"maxScore,omitempty"`
	FirstError     string            `json:"firstErrorStep,omitempty"`
	ErrorCause     string            `json:"errorCause,omitempty"`
	Confidence     float64           `json:"confidence"`
	NeedsReview    bool              `json:"needsReview"`
	TeacherNote    string            `json:"teacherNote,omitempty"`
	ScoreHistory   []map[string]any  `json:"scoreHistory,omitempty"`
	Status         string            `json:"status"`
	Snapshots      []AnswerSnapshot  `json:"snapshots,omitempty"`
	AIUsageNote    string            `json:"aiUsageNote,omitempty"`
	Citations      []string          `json:"citations,omitempty"`
	ImageDigest    string            `json:"imageDigest,omitempty"`
	ObjectiveOnly  bool              `json:"objectiveOnly,omitempty"`
	ModelFailed    bool              `json:"modelFailed,omitempty"`
	StudentVisible bool              `json:"studentVisibleDiag,omitempty"` // 诊断发布标记复用字段名时见 Content
	TaskID         string            `json:"taskId,omitempty"`
	IdempotencyKey string            `json:"idempotencyKey,omitempty"`
	SubmittedAt    time.Time         `json:"submittedAt,omitempty"`
}

type AnswerSnapshot struct {
	Version   int       `json:"version"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
	Trigger   string    `json:"trigger"` // interval | manual — 不采集逐键
}

// LearningOutcome describes the verified result of a learning event. An empty
// value is retained for backward-compatible data and is interpreted as unknown.
type LearningOutcome string

const (
	LearningOutcomeCorrect    LearningOutcome = "correct"
	LearningOutcomeIncorrect  LearningOutcome = "incorrect"
	LearningOutcomeUnderstood LearningOutcome = "understood"
	LearningOutcomeCompleted  LearningOutcome = "completed"
	LearningOutcomeUnknown    LearningOutcome = "unknown"
)

var ErrInvalidLearningOutcome = errors.New("invalid learning outcome")

// NormalizeLearningOutcome maps a legacy empty value to unknown and rejects
// unsupported values instead of treating them as successful evidence.
func NormalizeLearningOutcome(outcome LearningOutcome) (LearningOutcome, error) {
	if outcome == "" {
		return LearningOutcomeUnknown, nil
	}
	switch outcome {
	case LearningOutcomeCorrect,
		LearningOutcomeIncorrect,
		LearningOutcomeUnderstood,
		LearningOutcomeCompleted,
		LearningOutcomeUnknown:
		return outcome, nil
	default:
		return "", ErrInvalidLearningOutcome
	}
}

type LearningEvent struct {
	ID        string          `json:"id"`
	StudentID string          `json:"studentId"`
	ClassID   string          `json:"classId"`
	Kind      string          `json:"kind"`
	Summary   string          `json:"summary"`
	KP        string          `json:"knowledgePoint,omitempty"`
	Outcome   LearningOutcome `json:"outcome,omitempty"`
	At        time.Time       `json:"at"`
}

type Mistake struct {
	ID             string    `json:"id"`
	StudentID      string    `json:"studentId"`
	ClassID        string    `json:"classId"`
	AssignmentID   string    `json:"assignmentId,omitempty"`
	SubmissionID   string    `json:"submissionId,omitempty"`
	KnowledgePoint string    `json:"knowledgePoint"`
	QuestionStem   string    `json:"questionStem"`
	ErrorCause     string    `json:"errorCause"`
	Hint           string    `json:"hint,omitempty"`
	StandardAnswer string    `json:"-"`
	CreatedAt      time.Time `json:"createdAt"`
}

type PracticeSession struct {
	ID              string           `json:"id"`
	StudentID       string           `json:"studentId"`
	ClassID         string           `json:"classId"`
	KnowledgePoint  string           `json:"knowledgePoint"`
	Difficulty      string           `json:"difficulty"`
	QuestionCount   int              `json:"questionCount"`
	ConsecutiveOK   int              `json:"consecutiveCorrect"`
	HintsUsed       int              `json:"hintsUsed"`
	ErrorType       string           `json:"errorType"`
	MasteryEvidence []string         `json:"masteryEvidence"`
	CurrentQuestion map[string]any   `json:"currentQuestion,omitempty"`
	History         []map[string]any `json:"history"`
	Finished        bool             `json:"finished"`
	FinishReason    string           `json:"finishReason,omitempty"`
}

type TutorSession struct {
	ID           string   `json:"id"`
	StudentID    string   `json:"studentId"`
	ClassID      string   `json:"classId"`
	QuestionID   string   `json:"questionId"`
	Round        int      `json:"round"`
	HitStreak    int      `json:"hitStreak"`
	MaxRounds    int      `json:"maxRounds"`
	Released     bool     `json:"answerReleased"`
	FullSolution string   `json:"fullSolution,omitempty"`
	LastPrompt   string   `json:"lastPrompt"`
	History      []string `json:"history"`
}

type ClassroomSession struct {
	ID       string            `json:"id"`
	ClassID  string            `json:"classId"`
	Prompt   string            `json:"prompt"`
	Answers  []ClassroomAnswer `json:"answers"`
	Clusters []map[string]any  `json:"clusters,omitempty"`
}

type ClassroomAnswer struct {
	StudentID string `json:"studentId,omitempty"`
	Name      string `json:"name,omitempty"`
	Text      string `json:"text"`
}

type AuditEvent struct {
	ID         string         `json:"id"`
	ActorID    string         `json:"actorId"`
	ClassID    string         `json:"classId"`
	Action     string         `json:"action"`
	Object     string         `json:"object"`
	Result     string         `json:"result"`
	Skill      string         `json:"skill,omitempty"`
	Usage      int            `json:"usage,omitempty"`
	DurationMS int64          `json:"durationMs"`
	RequestID  string         `json:"requestId"`
	Detail     map[string]any `json:"detail,omitempty"`
	CreatedAt  time.Time      `json:"createdAt"`
}

type SimilarityHit struct {
	AttemptID  string  `json:"attemptId"`
	Fragment   string  `json:"fragment"`
	Source     string  `json:"source"`
	Similarity float64 `json:"similarity"`
	Location   string  `json:"location"`
}

type Attachment struct {
	ID          string    `json:"id"`
	Digest      string    `json:"digest"`
	ObjectKey   string    `json:"objectKey"`
	ClassID     string    `json:"classId"`
	OwnerID     string    `json:"ownerId"`
	ContentType string    `json:"contentType"`
	Size        int64     `json:"size"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Memory 全局演示存储。
type Memory struct {
	mu          sync.RWMutex
	persistence persistence

	Materials     map[string]Material
	Assistants    map[string]AssistantConfig
	Conversations map[string]Conversation
	Assignments   map[string]Assignment
	Contents      map[string]ContentRecord
	Tasks         map[string]TaskRecord
	Submissions   map[string]Submission
	Attachments   map[string]Attachment
	Mistakes      []Mistake
	Events        []LearningEvent
	Practice      map[string]PracticeSession
	Tutor         map[string]TutorSession
	Classroom     map[string]ClassroomSession
	Audit         []AuditEvent
	ClassSkills   map[string]map[string]bool // classID -> skill -> enabled
	Similarity    map[string][]SimilarityHit
	ProcessedJobs map[string]bool   // idempotency
	Idempotency   map[string]string // actor + operation + key -> task ID
}

var Default = New()

func New() *Memory {
	m := &Memory{
		persistence:   NewPersistenceFromEnv(),
		Materials:     map[string]Material{},
		Assistants:    map[string]AssistantConfig{},
		Conversations: map[string]Conversation{},
		Assignments:   map[string]Assignment{},
		Contents:      map[string]ContentRecord{},
		Tasks:         map[string]TaskRecord{},
		Submissions:   map[string]Submission{},
		Attachments:   map[string]Attachment{},
		Practice:      map[string]PracticeSession{},
		Tutor:         map[string]TutorSession{},
		Classroom:     map[string]ClassroomSession{},
		ClassSkills:   map[string]map[string]bool{},
		Similarity:    map[string][]SimilarityHit{},
		ProcessedJobs: map[string]bool{},
		Idempotency:   map[string]string{},
	}
	if demoDataEnabled() {
		m.seed()
	}
	return m
}

func demoDataEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV"))) {
	case "", "local", "development", "dev", "test", "demo":
		return true
	default:
		return false
	}
}

var allSkillIDs = []string{
	"proposition", "lesson_plan", "virtual_classroom",
	"multimodal_grade", "answer_replay", "class_review", "diagnosis",
	"review_path", "adaptive_practice", "preview_sheet",
	"socratic_tutor", "classroom_cluster",
}

func (m *Memory) seed() {
	classID := "class-demo"
	now := time.Now().UTC()
	m.Materials["mat-1"] = Material{
		ID: "mat-1", ClassID: classID, Title: "线性方程组讲义",
		Content: "线性方程组可用矩阵表示。高斯消元法通过初等行变换求阶梯型。秩等于增广矩阵秩时有解。",
		Version: "v1", ContentHash: "seed-linear-algebra", IndexStatus: "ready", EmbeddingModel: "seed",
		CreatedBy: "tea-1", CreatedAt: now,
		Chunks: []Chunk{
			{ID: "c1", MaterialID: "mat-1", MaterialVersion: "v1", Index: 0, CharacterStart: 0, CharacterEnd: 13, ContentHash: "seed-c1", Text: "线性方程组可用矩阵表示。", Source: "线性方程组讲义§1"},
			{ID: "c2", MaterialID: "mat-1", MaterialVersion: "v1", Index: 1, CharacterStart: 13, CharacterEnd: 33, ContentHash: "seed-c2", Text: "高斯消元法通过初等行变换求阶梯型。", Source: "线性方程组讲义§2"},
			{ID: "c3", MaterialID: "mat-1", MaterialVersion: "v1", Index: 2, CharacterStart: 33, CharacterEnd: 50, ContentHash: "seed-c3", Text: "秩等于增广矩阵秩时有解。", Source: "线性方程组讲义§3"},
			{ID: "c4", MaterialID: "mat-1", MaterialVersion: "v1", Index: 3, CharacterStart: 50, CharacterEnd: 59, ContentHash: "seed-c4", Text: "齐次方程总有零解。", Source: "线性方程组讲义§4"},
		},
	}
	m.Materials["mat-other"] = Material{
		ID: "mat-other", ClassID: "class-other", Title: "其他班讲义", Content: "仅其他班可见的离散数学材料。",
		Version: "v1", ContentHash: "seed-other", IndexStatus: "ready", EmbeddingModel: "seed",
		CreatedBy: "tea-2", CreatedAt: now,
		Chunks: []Chunk{{ID: "other-c1", MaterialID: "mat-other", MaterialVersion: "v1", Index: 0, CharacterStart: 0, CharacterEnd: 15, ContentHash: "seed-other-c1", Text: "仅其他班可见的离散数学材料。", Source: "其他班讲义§1"}},
	}
	m.Assistants[classID] = AssistantConfig{
		ID: "assistant-demo", ClassID: classID, Name: "班级学习助手",
		SystemPrompt:  "只依据本班课程资料回答；没有依据时明确说明。",
		EnabledSkills: []string{"rag_search", "mistake_hint"}, UpdatedBy: "tea-1", UpdatedAt: now,
	}
	skills := map[string]bool{}
	for _, s := range allSkillIDs {
		skills[s] = true
	}
	m.ClassSkills[classID] = skills
	otherSkills := map[string]bool{}
	for _, s := range allSkillIDs {
		otherSkills[s] = true
	}
	m.ClassSkills["class-other"] = otherSkills

	// 演示错题与学习事件（杨瑾文：诊断证据）
	m.Mistakes = []Mistake{
		{ID: "mistake-1", StudentID: "stu-1", ClassID: classID, KnowledgePoint: "高斯消元", QuestionStem: "将矩阵化简为阶梯型", ErrorCause: "行交换后未同步右端", Hint: "检查一次初等行变换是否作用于增广矩阵整行。", CreatedAt: now.Add(-48 * time.Hour)},
		{ID: "mistake-2", StudentID: "stu-1", ClassID: classID, KnowledgePoint: "矩阵秩", QuestionStem: "判断方程组有解条件", ErrorCause: "混淆系数矩阵与增广矩阵秩", Hint: "分别计算两个矩阵的秩并比较。", CreatedAt: now.Add(-24 * time.Hour)},
	}
	m.Events = []LearningEvent{
		{ID: "ev-1", StudentID: "stu-1", ClassID: classID, Kind: "assignment", Summary: "作业#1 行交换后未同步右端", KP: "高斯消元", At: now.Add(-72 * time.Hour)},
		{ID: "ev-2", StudentID: "stu-1", ClassID: classID, Kind: "practice", Summary: "练习#2 混淆两种秩", KP: "矩阵秩", At: now.Add(-48 * time.Hour)},
		{ID: "ev-3", StudentID: "stu-1", ClassID: classID, Kind: "tutoring", Summary: "辅导追问：增广矩阵概念", KP: "高斯消元", At: now.Add(-24 * time.Hour)},
		{ID: "ev-4", StudentID: "stu-1", ClassID: classID, Kind: "help", Summary: "课堂求助：回代计算", KP: "高斯消元", At: now.Add(-12 * time.Hour)},
	}
}

func (m *Memory) SkillEnabled(classID string, skill string) bool {
	return m.GetSkills(classID)[skill]
}

func (m *Memory) SetSkills(classID string, enabled map[string]bool) {
	m.mu.Lock()
	cp := map[string]bool{}
	for k, v := range enabled {
		cp[k] = v
	}
	m.ClassSkills[classID] = cp
	m.mu.Unlock()
	_ = m.persistenceStore().save("skills", classID, cp, classID)
}

func (m *Memory) GetSkills(classID string) map[string]bool {
	m.mu.RLock()
	out := map[string]bool{}
	src, ok := m.ClassSkills[classID]
	m.mu.RUnlock()
	if !ok {
		persisted := map[string]bool{}
		if err := m.persistenceStore().load("skills", classID, &persisted); err == nil {
			m.mu.Lock()
			m.ClassSkills[classID] = persisted
			m.mu.Unlock()
			src = persisted
			ok = true
		}
	}
	if !ok {
		for _, skill := range allSkillIDs {
			out[skill] = true
		}
		return out
	}
	for k, v := range src {
		out[k] = v
	}
	return out
}

func (m *Memory) SaveTask(t TaskRecord) {
	m.mu.Lock()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	t.UpdatedAt = time.Now().UTC()
	m.Tasks[t.ID] = t
	m.mu.Unlock()
	_ = m.persistenceStore().save("task", t.ID, t, t.ClassID, "user:"+t.UserID)
}

func (m *Memory) GetTask(id string) (TaskRecord, bool) {
	if m.persistenceStore().enabled() {
		var persisted TaskRecord
		if err := m.persistenceStore().load("task", id, &persisted); err == nil {
			m.mu.Lock()
			m.Tasks[id] = persisted
			m.mu.Unlock()
			return persisted, true
		}
	}
	m.mu.RLock()
	t, ok := m.Tasks[id]
	m.mu.RUnlock()
	if ok {
		return t, true
	}
	if err := m.persistenceStore().load("task", id, &t); err != nil {
		return TaskRecord{}, false
	}
	m.mu.Lock()
	m.Tasks[id] = t
	m.mu.Unlock()
	return t, true
}

func (m *Memory) SaveContent(c ContentRecord) {
	m.mu.Lock()
	m.Contents[c.ID] = c
	m.mu.Unlock()
	_ = m.persistenceStore().save("content", c.ID, c, c.ClassID, "skill:"+c.Skill)
}

func (m *Memory) GetContent(id string) (ContentRecord, bool) {
	if m.persistenceStore().enabled() {
		var persisted ContentRecord
		if err := m.persistenceStore().load("content", id, &persisted); err == nil {
			m.mu.Lock()
			m.Contents[id] = persisted
			m.mu.Unlock()
			return persisted, true
		}
	}
	m.mu.RLock()
	c, ok := m.Contents[id]
	m.mu.RUnlock()
	if ok {
		return c, true
	}
	if err := m.persistenceStore().load("content", id, &c); err != nil {
		return ContentRecord{}, false
	}
	m.mu.Lock()
	m.Contents[id] = c
	m.mu.Unlock()
	return c, true
}

func (m *Memory) AppendAudit(ev AuditEvent) error {
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now().UTC()
	}
	if err := m.persistenceStore().save("audit", ev.ID, ev, ev.ClassID); err != nil {
		return err
	}
	m.mu.Lock()
	m.Audit = append(m.Audit, ev)
	m.mu.Unlock()
	return nil
}

func (m *Memory) ListAudit(classID string) []AuditEvent {
	m.mu.RLock()
	var out []AuditEvent
	for _, e := range m.Audit {
		if classID == "" || e.ClassID == classID {
			out = append(out, e)
		}
	}
	m.mu.RUnlock()
	if classID != "" {
		if values, err := m.persistenceStore().list("audit", classID, func() any { return &AuditEvent{} }); err == nil {
			seen := map[string]bool{}
			for _, event := range out {
				seen[event.ID] = true
			}
			for _, value := range values {
				event := *(value.(*AuditEvent))
				if !seen[event.ID] {
					out = append(out, event)
				}
			}
		}
	}
	return out
}

func (m *Memory) MaterialsByClass(classID string) []Material {
	m.mu.RLock()
	var out []Material
	for _, mat := range m.Materials {
		if mat.ClassID == classID {
			out = append(out, mat)
		}
	}
	m.mu.RUnlock()
	if values, err := m.persistenceStore().list("material", classID, func() any { return &Material{} }); err == nil {
		seen := map[string]bool{}
		for _, material := range out {
			seen[material.ID] = true
		}
		for _, value := range values {
			material := *(value.(*Material))
			if !seen[material.ID] {
				out = append(out, material)
			}
		}
	}
	return out
}

func (m *Memory) MarkJobDone(key string) (bool, error) {
	m.mu.Lock()
	if m.ProcessedJobs[key] {
		m.mu.Unlock()
		return false, nil
	}
	m.mu.Unlock()
	if err := m.persistenceStore().markJobDone(key); err != nil {
		return false, err
	}
	m.mu.Lock()
	m.ProcessedJobs[key] = true
	m.mu.Unlock()
	return true, nil
}

func (m *Memory) JobDone(key string) (bool, error) {
	m.mu.RLock()
	done := m.ProcessedJobs[key]
	m.mu.RUnlock()
	if done {
		return true, nil
	}
	return m.persistenceStore().jobDone(key)
}

func (m *Memory) Lock()   { m.mu.Lock() }
func (m *Memory) Unlock() { m.mu.Unlock() }

func (m *Memory) GetSubmission(id string) (Submission, bool) {
	if m.persistenceStore().enabled() {
		var persisted Submission
		if err := m.persistenceStore().load("submission", id, &persisted); err == nil {
			m.mu.Lock()
			m.Submissions[id] = persisted
			m.mu.Unlock()
			return persisted, true
		}
	}
	m.mu.RLock()
	s, ok := m.Submissions[id]
	m.mu.RUnlock()
	if ok {
		return s, true
	}
	if err := m.persistenceStore().load("submission", id, &s); err != nil {
		return Submission{}, false
	}
	m.mu.Lock()
	m.Submissions[id] = s
	m.mu.Unlock()
	return s, true
}

func (m *Memory) SaveSubmission(s Submission) {
	m.mu.Lock()
	if s.SubmittedAt.IsZero() {
		s.SubmittedAt = time.Now().UTC()
	}
	m.Submissions[s.ID] = s
	m.mu.Unlock()
	_ = m.persistenceStore().save("submission", s.ID, s, s.ClassID, "assignment:"+s.AssignmentID, "student:"+s.StudentID)
}

func (m *Memory) SetSimilarity(attemptID string, hits []SimilarityHit) error {
	if err := m.persistenceStore().save("similarity", attemptID, hits); err != nil {
		return err
	}
	m.mu.Lock()
	m.Similarity[attemptID] = append([]SimilarityHit(nil), hits...)
	m.mu.Unlock()
	return nil
}

func (m *Memory) GetSimilarity(attemptID string) []SimilarityHit {
	m.mu.RLock()
	hits, ok := m.Similarity[attemptID]
	m.mu.RUnlock()
	if ok {
		return append([]SimilarityHit(nil), hits...)
	}
	var persisted []SimilarityHit
	if err := m.persistenceStore().load("similarity", attemptID, &persisted); err != nil {
		return nil
	}
	m.mu.Lock()
	m.Similarity[attemptID] = append([]SimilarityHit(nil), persisted...)
	m.mu.Unlock()
	return append([]SimilarityHit(nil), persisted...)
}

func (m *Memory) ListContents(classID string, skill string) []ContentRecord {
	m.mu.RLock()
	var out []ContentRecord
	for _, c := range m.Contents {
		if (classID == "" || c.ClassID == classID) && (skill == "" || c.Skill == skill) {
			out = append(out, c)
		}
	}
	m.mu.RUnlock()
	if classID != "" {
		if values, err := m.persistenceStore().list("content", classID, func() any { return &ContentRecord{} }); err == nil {
			seen := map[string]bool{}
			for _, content := range out {
				seen[content.ID] = true
			}
			for _, value := range values {
				content := *(value.(*ContentRecord))
				if !seen[content.ID] && (skill == "" || content.Skill == skill) {
					out = append(out, content)
				}
			}
		}
	}
	return out
}

func (m *Memory) ListSubmissions(classID, assignmentID string) []Submission {
	m.mu.RLock()
	var out []Submission
	for _, s := range m.Submissions {
		if classID != "" && s.ClassID != classID {
			continue
		}
		if assignmentID != "" && s.AssignmentID != assignmentID {
			continue
		}
		out = append(out, s)
	}
	m.mu.RUnlock()
	index := classID
	if assignmentID != "" {
		index = "assignment:" + assignmentID
	}
	if index != "" {
		if values, err := m.persistenceStore().list("submission", index, func() any { return &Submission{} }); err == nil {
			seen := map[string]bool{}
			for _, submission := range out {
				seen[submission.ID] = true
			}
			for _, value := range values {
				submission := *(value.(*Submission))
				if !seen[submission.ID] && (classID == "" || submission.ClassID == classID) && (assignmentID == "" || submission.AssignmentID == assignmentID) {
					out = append(out, submission)
				}
			}
		}
	}
	return out
}

func (m *Memory) EventsFor(studentID, classID string) []LearningEvent {
	m.mu.RLock()
	var out []LearningEvent
	for _, e := range m.Events {
		if (studentID == "" || e.StudentID == studentID) && (classID == "" || e.ClassID == classID) {
			out = append(out, e)
		}
	}
	m.mu.RUnlock()
	index := classID
	if studentID != "" {
		index = "student:" + studentID
	}
	if index != "" {
		if values, err := m.persistenceStore().list("event", index, func() any { return &LearningEvent{} }); err == nil {
			seen := make(map[string]bool, len(out))
			for _, event := range out {
				seen[event.ID] = true
			}
			for _, value := range values {
				event := *(value.(*LearningEvent))
				if !seen[event.ID] && (classID == "" || event.ClassID == classID) && (studentID == "" || event.StudentID == studentID) {
					out = append(out, event)
					seen[event.ID] = true
				}
			}
		}
	}
	return out
}

func (m *Memory) AppendEvent(e LearningEvent) error {
	m.mu.RLock()
	for _, existing := range m.Events {
		if e.ID != "" && existing.ID == e.ID {
			m.mu.RUnlock()
			return nil
		}
	}
	m.mu.RUnlock()
	if err := m.persistenceStore().save("event", e.ID, e, e.ClassID, "student:"+e.StudentID); err != nil {
		return err
	}
	m.mu.Lock()
	for _, existing := range m.Events {
		if e.ID != "" && existing.ID == e.ID {
			m.mu.Unlock()
			return nil
		}
	}
	m.Events = append(m.Events, e)
	m.mu.Unlock()
	return nil
}

func (m *Memory) MistakesFor(studentID, classID string) []Mistake {
	m.mu.RLock()
	var out []Mistake
	for _, mk := range m.Mistakes {
		if (studentID == "" || mk.StudentID == studentID) && (classID == "" || mk.ClassID == classID) {
			out = append(out, mk)
		}
	}
	m.mu.RUnlock()
	index := classID
	if studentID != "" {
		index = "student:" + studentID
	}
	if index != "" {
		if values, err := m.persistenceStore().list("mistake", index, func() any { return &Mistake{} }); err == nil {
			seen := map[string]bool{}
			for _, mistake := range out {
				seen[mistake.ID] = true
			}
			for _, value := range values {
				mistake := *(value.(*Mistake))
				if !seen[mistake.ID] && (classID == "" || mistake.ClassID == classID) && (studentID == "" || mistake.StudentID == studentID) {
					out = append(out, mistake)
				}
			}
		}
	}
	return out
}

var idSeq int

func NewID(prefix string) string {
	Default.mu.Lock()
	idSeq++
	n := idSeq
	Default.mu.Unlock()
	return prefix + "-" + time.Now().Format("150405") + "-" + itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

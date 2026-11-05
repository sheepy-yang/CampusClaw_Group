// Package store 提供演示用内存数据层，便于无 Docker 依赖时跑通主路径。
package store

import (
	"errors"
	"sync"
	"time"
)

type Material struct {
	ID      string  `json:"id"`
	ClassID string  `json:"classId"`
	Title   string  `json:"title"`
	Content string  `json:"content"`
	Chunks  []Chunk `json:"chunks"`
}

type Chunk struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Source string `json:"source"`
}

type Assignment struct {
	ID      string     `json:"id"`
	ClassID string     `json:"classId"`
	Title   string     `json:"title"`
	Status  string     `json:"status"`
	Items   []Question `json:"items"`
}

type Question struct {
	ID          string           `json:"id"`
	Stem        string           `json:"stem"`
	Type        string           `json:"type"`
	Difficulty  string           `json:"difficulty"`
	Answer      string           `json:"answer,omitempty"`
	Explanation string           `json:"explanation,omitempty"`
	Citations   []map[string]any `json:"citations,omitempty"`
	Status      string           `json:"status"`
	Adopted     bool             `json:"adopted"`
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
	ID      string     `json:"id"`
	Skill   string     `json:"skill"`
	ClassID string     `json:"classId"`
	Status  string     `json:"status"`
	Steps   []TaskStep `json:"steps"`
}

type TaskStep struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type Submission struct {
	ID             string           `json:"id"`
	AssignmentID   string           `json:"assignmentId"`
	StudentID      string           `json:"studentId"`
	ClassID        string           `json:"classId"`
	AnswerText     string           `json:"answerText"`
	Transcription  string           `json:"transcription,omitempty"`
	Confirmed      bool             `json:"transcriptionConfirmed"`
	Steps          []map[string]any `json:"steps,omitempty"`
	Score          *float64         `json:"score,omitempty"`
	Suggested      *float64         `json:"suggestedScore,omitempty"`
	MaxScore       float64          `json:"maxScore,omitempty"`
	FirstError     string           `json:"firstErrorStep,omitempty"`
	ErrorCause     string           `json:"errorCause,omitempty"`
	Confidence     float64          `json:"confidence"`
	NeedsReview    bool             `json:"needsReview"`
	TeacherNote    string           `json:"teacherNote,omitempty"`
	ScoreHistory   []map[string]any `json:"scoreHistory,omitempty"`
	Status         string           `json:"status"`
	Snapshots      []AnswerSnapshot `json:"snapshots,omitempty"`
	AIUsageNote    string           `json:"aiUsageNote,omitempty"`
	Citations      []string         `json:"citations,omitempty"`
	ImageDigest    string           `json:"imageDigest,omitempty"`
	ObjectiveOnly  bool             `json:"objectiveOnly,omitempty"`
	ModelFailed    bool             `json:"modelFailed,omitempty"`
	StudentVisible bool             `json:"studentVisibleDiag,omitempty"` // 诊断发布标记复用字段名时见 Content
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
	StudentID      string    `json:"studentId"`
	ClassID        string    `json:"classId"`
	KnowledgePoint string    `json:"knowledgePoint"`
	QuestionStem   string    `json:"questionStem"`
	ErrorCause     string    `json:"errorCause"`
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
	ID        string         `json:"id"`
	ActorID   string         `json:"actorId"`
	ClassID   string         `json:"classId"`
	Action    string         `json:"action"`
	Detail    map[string]any `json:"detail"`
	CreatedAt time.Time      `json:"createdAt"`
}

type SimilarityHit struct {
	AttemptID  string  `json:"attemptId"`
	Fragment   string  `json:"fragment"`
	Source     string  `json:"source"`
	Similarity float64 `json:"similarity"`
	Location   string  `json:"location"`
}

// Memory 全局演示存储。
type Memory struct {
	mu sync.RWMutex

	Materials     map[string]Material
	Assignments   map[string]Assignment
	Contents      map[string]ContentRecord
	Tasks         map[string]TaskRecord
	Submissions   map[string]Submission
	Mistakes      []Mistake
	Events        []LearningEvent
	Practice      map[string]PracticeSession
	Tutor         map[string]TutorSession
	Classroom     map[string]ClassroomSession
	Audit         []AuditEvent
	ClassSkills   map[string]map[string]bool // classID -> skill -> enabled
	Similarity    map[string][]SimilarityHit
	ProcessedJobs map[string]bool // idempotency
}

var Default = New()

func New() *Memory {
	m := &Memory{
		Materials:     map[string]Material{},
		Assignments:   map[string]Assignment{},
		Contents:      map[string]ContentRecord{},
		Tasks:         map[string]TaskRecord{},
		Submissions:   map[string]Submission{},
		Practice:      map[string]PracticeSession{},
		Tutor:         map[string]TutorSession{},
		Classroom:     map[string]ClassroomSession{},
		ClassSkills:   map[string]map[string]bool{},
		Similarity:    map[string][]SimilarityHit{},
		ProcessedJobs: map[string]bool{},
	}
	m.seed()
	return m
}

var allSkillIDs = []string{
	"proposition", "lesson_plan", "virtual_classroom",
	"multimodal_grade", "answer_replay", "class_review", "diagnosis",
	"review_path", "adaptive_practice", "preview_sheet",
	"socratic_tutor", "classroom_cluster",
}

func (m *Memory) seed() {
	classID := "class-demo"
	m.Materials["mat-1"] = Material{
		ID: "mat-1", ClassID: classID, Title: "线性方程组讲义",
		Content: "线性方程组可用矩阵表示。高斯消元法通过初等行变换求阶梯型。秩等于增广矩阵秩时有解。",
		Chunks: []Chunk{
			{ID: "c1", Text: "线性方程组可用矩阵表示。", Source: "讲义§1"},
			{ID: "c2", Text: "高斯消元法通过初等行变换求阶梯型。", Source: "讲义§2"},
			{ID: "c3", Text: "秩等于增广矩阵秩时有解。", Source: "讲义§3"},
			{ID: "c4", Text: "齐次方程总有零解。", Source: "讲义§4"},
		},
	}
	skills := map[string]bool{}
	for _, s := range allSkillIDs {
		skills[s] = true
	}
	m.ClassSkills[classID] = skills

	// 演示错题与学习事件（杨瑾文：诊断证据）
	now := time.Now()
	m.Mistakes = []Mistake{
		{StudentID: "stu-1", ClassID: classID, KnowledgePoint: "高斯消元", QuestionStem: "将矩阵化简为阶梯型", ErrorCause: "行交换后未同步右端", CreatedAt: now.Add(-48 * time.Hour)},
		{StudentID: "stu-1", ClassID: classID, KnowledgePoint: "矩阵秩", QuestionStem: "判断方程组有解条件", ErrorCause: "混淆系数矩阵与增广矩阵秩", CreatedAt: now.Add(-24 * time.Hour)},
	}
	m.Events = []LearningEvent{
		{ID: "ev-1", StudentID: "stu-1", ClassID: classID, Kind: "assignment", Summary: "作业#1 行交换后未同步右端", KP: "高斯消元", At: now.Add(-72 * time.Hour)},
		{ID: "ev-2", StudentID: "stu-1", ClassID: classID, Kind: "practice", Summary: "练习#2 混淆两种秩", KP: "矩阵秩", At: now.Add(-48 * time.Hour)},
		{ID: "ev-3", StudentID: "stu-1", ClassID: classID, Kind: "tutoring", Summary: "辅导追问：增广矩阵概念", KP: "高斯消元", At: now.Add(-24 * time.Hour)},
		{ID: "ev-4", StudentID: "stu-1", ClassID: classID, Kind: "help", Summary: "课堂求助：回代计算", KP: "高斯消元", At: now.Add(-12 * time.Hour)},
	}
}

func (m *Memory) SkillEnabled(classID string, skill string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.ClassSkills[classID]
	if !ok {
		return true
	}
	return s[skill]
}

func (m *Memory) SetSkills(classID string, enabled map[string]bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := map[string]bool{}
	for k, v := range enabled {
		cp[k] = v
	}
	m.ClassSkills[classID] = cp
}

func (m *Memory) GetSkills(classID string) map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := map[string]bool{}
	src, ok := m.ClassSkills[classID]
	if !ok {
		for _, s := range allSkillIDs {
			out[s] = true
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
	defer m.mu.Unlock()
	m.Tasks[t.ID] = t
}

func (m *Memory) GetTask(id string) (TaskRecord, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.Tasks[id]
	return t, ok
}

func (m *Memory) SaveContent(c ContentRecord) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Contents[c.ID] = c
}

func (m *Memory) GetContent(id string) (ContentRecord, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.Contents[id]
	return c, ok
}

func (m *Memory) AppendAudit(ev AuditEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Audit = append(m.Audit, ev)
}

func (m *Memory) ListAudit(classID string) []AuditEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []AuditEvent
	for _, e := range m.Audit {
		if classID == "" || e.ClassID == classID {
			out = append(out, e)
		}
	}
	return out
}

func (m *Memory) MaterialsByClass(classID string) []Material {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []Material
	for _, mat := range m.Materials {
		if mat.ClassID == classID {
			out = append(out, mat)
		}
	}
	return out
}

func (m *Memory) MarkJobDone(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ProcessedJobs[key] {
		return false
	}
	m.ProcessedJobs[key] = true
	return true
}

func (m *Memory) Lock()   { m.mu.Lock() }
func (m *Memory) Unlock() { m.mu.Unlock() }

func (m *Memory) GetSubmission(id string) (Submission, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.Submissions[id]
	return s, ok
}

func (m *Memory) SaveSubmission(s Submission) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Submissions[s.ID] = s
}

func (m *Memory) SetSimilarity(attemptID string, hits []SimilarityHit) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Similarity[attemptID] = hits
}

func (m *Memory) GetSimilarity(attemptID string) []SimilarityHit {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]SimilarityHit{}, m.Similarity[attemptID]...)
}

func (m *Memory) ListContents(classID string, skill string) []ContentRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []ContentRecord
	for _, c := range m.Contents {
		if (classID == "" || c.ClassID == classID) && (skill == "" || c.Skill == skill) {
			out = append(out, c)
		}
	}
	return out
}

func (m *Memory) ListSubmissions(classID, assignmentID string) []Submission {
	m.mu.RLock()
	defer m.mu.RUnlock()
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
	return out
}

func (m *Memory) EventsFor(studentID, classID string) []LearningEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []LearningEvent
	for _, e := range m.Events {
		if (studentID == "" || e.StudentID == studentID) && (classID == "" || e.ClassID == classID) {
			out = append(out, e)
		}
	}
	return out
}

func (m *Memory) AppendEvent(e LearningEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Events = append(m.Events, e)
}

func (m *Memory) MistakesFor(studentID, classID string) []Mistake {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []Mistake
	for _, mk := range m.Mistakes {
		if (studentID == "" || mk.StudentID == studentID) && (classID == "" || mk.ClassID == classID) {
			out = append(out, mk)
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

package tutoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/domain"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/security"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

var errVersionConflict = errors.New("resource version conflict")

const tutoringRecordKey = "tutoringRecord"

type tutoringPersistentRecord struct {
	ReviewPlan          *ReviewPlan                   `json:"reviewPlan,omitempty"`
	TutorSession        *TutorSession                 `json:"tutorSession,omitempty"`
	TutorQuestion       *persistedTutorQuestion       `json:"tutorQuestion,omitempty"`
	TutorVersion        int                           `json:"tutorVersion,omitempty"`
	PracticeSession     *PracticeSession              `json:"practiceSession,omitempty"`
	CurrentQuestion     *PracticeQuestion             `json:"currentQuestion,omitempty"`
	PracticeSubmissions map[string]PracticeSubmission `json:"practiceSubmissions,omitempty"`
	PracticeAnswers     map[string]string             `json:"practiceAnswers,omitempty"`
	Interaction         *ClassroomInteraction         `json:"interaction,omitempty"`
	ClassroomAnswers    map[string]ClassroomAnswer    `json:"classroomAnswers,omitempty"`
}

type persistedTutorQuestion struct {
	ID               string     `json:"id"`
	StudentID        string     `json:"studentId,omitempty"`
	ClassID          string     `json:"classId,omitempty"`
	Stem             string     `json:"stem"`
	FullSolution     string     `json:"fullSolution"`
	KeyConceptGroups [][]string `json:"keyConceptGroups"`
	NegativeTerms    []string   `json:"negativeTerms,omitempty"`
	Demo             bool       `json:"demo"`
}

// Repository adds the state needed by the four tutoring skills while reusing
// the project's existing Memory store for learning evidence, content, skill
// switches, and audit-visible learning events.
type Repository struct {
	memory *store.Memory

	mu             sync.RWMutex
	plans          map[string]ReviewPlan
	latestPlan     map[string]string
	tutors         map[string]TutorSession
	practices      map[string]PracticeSession
	interactions   map[string]ClassroomInteraction
	tutorQuestions map[string]TutorQuestion
	practiceBank   map[string]PracticeQuestion
	sequence       atomic.Uint64
}

func NewRepository(memory *store.Memory) *Repository {
	if memory == nil {
		memory = store.Default
	}
	r := &Repository{
		memory:         memory,
		plans:          make(map[string]ReviewPlan),
		latestPlan:     make(map[string]string),
		tutors:         make(map[string]TutorSession),
		practices:      make(map[string]PracticeSession),
		interactions:   make(map[string]ClassroomInteraction),
		tutorQuestions: make(map[string]TutorQuestion),
		practiceBank:   make(map[string]PracticeQuestion),
	}
	if tutoringDemoDataEnabled() {
		r.seedQuestions()
	}
	return r
}

func tutoringDemoDataEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV"))) {
	case "", "local", "development", "dev", "test", "demo":
		return true
	default:
		return false
	}
}

func (r *Repository) Memory() *store.Memory { return r.memory }

func (r *Repository) newID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, r.sequence.Add(1))
}

func (r *Repository) skillEnabled(classID, skill string) bool {
	return r.memory.SkillEnabled(classID, skill)
}

func (r *Repository) learningEvents(studentID, classID string) []store.LearningEvent {
	return r.memory.EventsFor(studentID, classID)
}

func (r *Repository) mistakes(studentID, classID string) []store.Mistake {
	return r.memory.MistakesFor(studentID, classID)
}

func (r *Repository) savePlan(plan ReviewPlan) error {
	if err := r.savePlanContent(plan); err != nil {
		return persistenceUnavailable(err)
	}
	r.mu.Lock()
	r.plans[plan.ID] = cloneReviewPlan(plan)
	r.latestPlan[scopeKey(plan.ClassID, plan.StudentID)] = plan.ID
	r.mu.Unlock()
	return nil
}

func (r *Repository) savePlanContent(plan ReviewPlan) error {
	steps := make([]map[string]any, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		steps = append(steps, map[string]any{
			"id":             step.ID,
			"order":          step.Order,
			"type":           step.Type,
			"title":          step.Title,
			"detail":         step.Detail,
			"knowledgePoint": step.KnowledgePoint,
			"completed":      step.Completed,
			"completedAt":    step.CompletedAt,
		})
	}
	return r.savePersistentContent(store.ContentRecord{
		ID:      plan.ID,
		Skill:   string(domain.SkillReviewPath),
		ClassID: plan.ClassID,
		Status:  plan.Status,
		Payload: map[string]any{
			"studentId":       plan.StudentID,
			"steps":           steps,
			"selfTestCounts":  false,
			tutoringRecordKey: persistentMap(tutoringPersistentRecord{ReviewPlan: &plan}),
		},
		CreatedAt: plan.CreatedAt,
		UpdatedAt: plan.UpdatedAt,
	})
}

func (r *Repository) getPlan(id string) (ReviewPlan, bool) {
	r.mu.RLock()
	plan, ok := r.plans[id]
	r.mu.RUnlock()
	if !ok {
		if record, found := r.persistentRecord(id); found && record.ReviewPlan != nil {
			plan = cloneReviewPlan(*record.ReviewPlan)
			r.mu.Lock()
			r.plans[id] = plan
			r.latestPlan[scopeKey(plan.ClassID, plan.StudentID)] = id
			r.mu.Unlock()
			ok = true
		}
	}
	return cloneReviewPlan(plan), ok
}

func (r *Repository) getLatestPlan(classID, studentID string) (ReviewPlan, bool) {
	r.mu.RLock()
	id := r.latestPlan[scopeKey(classID, studentID)]
	plan, ok := r.plans[id]
	r.mu.RUnlock()
	if !ok {
		for _, content := range r.memory.ListContents(classID, string(domain.SkillReviewPath)) {
			record, valid := decodePersistentRecord(content)
			if !valid || record.ReviewPlan == nil || record.ReviewPlan.StudentID != studentID {
				continue
			}
			candidate := cloneReviewPlan(*record.ReviewPlan)
			if !ok || candidate.UpdatedAt.After(plan.UpdatedAt) {
				plan, id, ok = candidate, candidate.ID, true
			}
		}
		if ok {
			r.mu.Lock()
			r.plans[id] = plan
			r.latestPlan[scopeKey(classID, studentID)] = id
			r.mu.Unlock()
		}
	}
	return cloneReviewPlan(plan), ok
}

func (r *Repository) completePlanStep(planID, stepID, studentID, classID string, now time.Time) (ReviewPlan, error) {
	if _, ok := r.getPlan(planID); !ok {
		return ReviewPlan{}, notFound()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	plan, ok := r.plans[planID]
	if !ok || plan.StudentID != studentID || plan.ClassID != classID {
		return ReviewPlan{}, notFound()
	}
	stepIndex := -1
	for i := range plan.Steps {
		if plan.Steps[i].ID == stepID {
			stepIndex = i
			break
		}
	}
	if stepIndex < 0 {
		return ReviewPlan{}, notFound()
	}
	if plan.Steps[stepIndex].Completed {
		return cloneReviewPlan(plan), nil
	}

	original := cloneReviewPlan(plan)
	completedAt := now.UTC()
	plan.Steps[stepIndex].Completed = true
	plan.Steps[stepIndex].CompletedAt = &completedAt
	plan.UpdatedAt = completedAt
	result := cloneReviewPlan(plan)
	step := plan.Steps[stepIndex]

	if err := r.savePlanContent(plan); err != nil {
		return ReviewPlan{}, persistenceUnavailable(err)
	}
	if err := r.memory.AppendEvent(store.LearningEvent{
		ID:        r.newID("event"),
		StudentID: studentID,
		ClassID:   classID,
		Kind:      "review_task",
		Summary:   "completed review step",
		KP:        step.KnowledgePoint,
		Outcome:   store.LearningOutcomeCompleted,
		At:        completedAt,
	}); err != nil {
		_ = r.savePlanContent(original)
		return ReviewPlan{}, persistenceUnavailable(err)
	}
	r.plans[planID] = plan
	return result, nil
}

func (r *Repository) addTutorQuestion(question TutorQuestion) error {
	if err := validateTutorQuestion(question); err != nil {
		return err
	}
	r.mu.Lock()
	r.tutorQuestions[question.ID] = cloneTutorQuestion(question)
	r.mu.Unlock()
	return nil
}

func (r *Repository) AddTutorQuestion(question TutorQuestion) error {
	return r.addTutorQuestion(question)
}

func (r *Repository) tutorQuestion(id string) (TutorQuestion, bool) {
	r.mu.RLock()
	question, ok := r.tutorQuestions[id]
	r.mu.RUnlock()
	return cloneTutorQuestion(question), ok
}

func (r *Repository) latestMistakeTutorQuestion(studentID, classID string) (TutorQuestion, bool) {
	var selected store.Mistake
	found := false
	for _, mistake := range r.mistakes(studentID, classID) {
		if strings.TrimSpace(mistake.StandardAnswer) == "" {
			continue
		}
		if !found || mistake.CreatedAt.After(selected.CreatedAt) {
			selected = mistake
			found = true
		}
	}
	if !found {
		return TutorQuestion{}, false
	}
	solution := "参考答案：" + strings.TrimSpace(selected.StandardAnswer)
	if hint := strings.TrimSpace(selected.Hint); hint != "" {
		solution += "。思路提示：" + hint
	}
	question := TutorQuestion{
		ID: selected.ID, StudentID: studentID, ClassID: classID,
		Stem: selected.QuestionStem, FullSolution: solution,
		KeyConceptGroups: [][]string{{strings.TrimSpace(selected.StandardAnswer)}},
		NegativeTerms:    []string{"不知道", "不会", "不确定"},
	}
	if err := r.addTutorQuestion(question); err != nil {
		return TutorQuestion{}, false
	}
	return question, true
}

func (r *Repository) saveTutor(session TutorSession, expectedVersion int) error {
	if expectedVersion > 0 {
		if _, exists := r.tutor(session.ID); !exists {
			return errVersionConflict
		}
	}
	r.mu.Lock()
	current, exists := r.tutors[session.ID]
	if expectedVersion == 0 {
		if exists {
			r.mu.Unlock()
			return errVersionConflict
		}
		session.Version = 1
		if err := r.saveTutorContent(session); err != nil {
			r.mu.Unlock()
			return persistenceUnavailable(err)
		}
		r.tutors[session.ID] = cloneTutorSession(session)
		r.mu.Unlock()
		return nil
	}
	if !exists || current.Version != expectedVersion {
		r.mu.Unlock()
		return errVersionConflict
	}
	session.Version = expectedVersion + 1
	if err := r.saveTutorContent(session); err != nil {
		r.mu.Unlock()
		return persistenceUnavailable(err)
	}
	r.tutors[session.ID] = cloneTutorSession(session)
	r.mu.Unlock()
	return nil
}

func (r *Repository) tutor(id string) (TutorSession, bool) {
	r.mu.RLock()
	session, ok := r.tutors[id]
	r.mu.RUnlock()
	if !ok {
		if record, found := r.persistentRecord(id); found && record.TutorSession != nil {
			session = cloneTutorSession(*record.TutorSession)
			session.Version = record.TutorVersion
			r.mu.Lock()
			r.tutors[id] = session
			if record.TutorQuestion != nil {
				question := record.TutorQuestion.question()
				r.tutorQuestions[question.ID] = question
			}
			r.mu.Unlock()
			ok = true
		}
	}
	return cloneTutorSession(session), ok
}

func (r *Repository) saveTutorContent(session TutorSession) error {
	copySession := cloneTutorSession(session)
	question, hasQuestion := r.tutorQuestions[session.QuestionID]
	var persistedQuestion *persistedTutorQuestion
	if hasQuestion {
		cloned := cloneTutorQuestion(question)
		persisted := newPersistedTutorQuestion(cloned)
		persistedQuestion = &persisted
	}
	status := "active"
	if session.Released {
		status = "released"
	}
	return r.savePersistentContent(store.ContentRecord{
		ID: session.ID, Skill: string(domain.SkillSocraticTutor), ClassID: session.ClassID, Status: status,
		Payload: map[string]any{
			"studentId": session.StudentID,
			tutoringRecordKey: persistentMap(tutoringPersistentRecord{
				TutorSession: &copySession, TutorQuestion: persistedQuestion, TutorVersion: session.Version,
			}),
		},
		CreatedAt: session.CreatedAt, UpdatedAt: session.UpdatedAt,
	})
}

func (r *Repository) addPracticeQuestion(question PracticeQuestion) error {
	if err := validatePracticeQuestion(question); err != nil {
		return err
	}
	r.mu.Lock()
	r.practiceBank[question.ID] = question
	r.mu.Unlock()
	return nil
}

func (r *Repository) AddPracticeQuestion(question PracticeQuestion) error {
	return r.addPracticeQuestion(question)
}

func (r *Repository) hasPracticeQuestions(knowledgePoint string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, question := range r.practiceBank {
		if question.KnowledgePoint == knowledgePoint {
			return true
		}
	}
	return false
}

func (r *Repository) importPublishedPracticeQuestions(classID, knowledgePoint string) {
	for _, assignment := range r.memory.ListAssignments(classID) {
		if assignment.Status != "published" {
			continue
		}
		for _, item := range assignment.Items {
			itemKnowledgePoint := strings.TrimSpace(item.KnowledgePoint)
			if itemKnowledgePoint == "" {
				itemKnowledgePoint = assignment.Title
			}
			if itemKnowledgePoint != knowledgePoint || strings.TrimSpace(item.Answer) == "" {
				continue
			}
			difficulty := strings.ToLower(strings.TrimSpace(item.Difficulty))
			switch difficulty {
			case "easy", "medium", "hard":
			default:
				difficulty = "medium"
			}
			alternatives := make([]string, 0)
			for _, alternative := range strings.Split(item.Answer, "|") {
				if value := strings.TrimSpace(alternative); value != "" && value != strings.TrimSpace(item.Answer) {
					alternatives = append(alternatives, value)
				}
			}
			_ = r.addPracticeQuestion(PracticeQuestion{
				PublicQuestion: PublicQuestion{
					ID: assignment.ID + ":" + item.ID, KnowledgePoint: itemKnowledgePoint,
					Difficulty: difficulty, Stem: item.Stem,
					Hint: "回到题干条件和已学定义，逐步核对后再作答。", Verified: true,
				},
				ClassID: classID,
				Answer:  item.Answer, AcceptableAnswers: alternatives,
			})
		}
	}
}

func (r *Repository) selectPracticeQuestion(classID, knowledgePoint, difficulty string, offset int) (PracticeQuestion, bool) {
	r.mu.RLock()
	var candidates []PracticeQuestion
	for _, question := range r.practiceBank {
		if (question.ClassID == "" || question.ClassID == classID) &&
			question.KnowledgePoint == knowledgePoint && question.Difficulty == difficulty {
			candidates = append(candidates, question)
		}
	}
	r.mu.RUnlock()
	if len(candidates) == 0 {
		return PracticeQuestion{}, false
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	return candidates[offset%len(candidates)], true
}

func (r *Repository) savePractice(session PracticeSession) error {
	r.mu.Lock()
	if _, exists := r.practices[session.ID]; exists {
		r.mu.Unlock()
		return errVersionConflict
	}
	if err := r.savePracticeContent(session); err != nil {
		r.mu.Unlock()
		return persistenceUnavailable(err)
	}
	r.practices[session.ID] = clonePracticeSession(session)
	r.mu.Unlock()
	return nil
}

func (r *Repository) practice(id string) (PracticeSession, bool) {
	r.mu.RLock()
	session, ok := r.practices[id]
	r.mu.RUnlock()
	if !ok {
		if record, found := r.persistentRecord(id); found && record.PracticeSession != nil {
			session = clonePracticeSession(*record.PracticeSession)
			if record.CurrentQuestion != nil {
				session.CurrentQuestion = *record.CurrentQuestion
			}
			session.Submissions = record.PracticeSubmissions
			if session.Submissions == nil {
				session.Submissions = make(map[string]PracticeSubmission)
			}
			for id, answer := range record.PracticeAnswers {
				submission := session.Submissions[id]
				submission.AnswerText = answer
				session.Submissions[id] = submission
			}
			r.mu.Lock()
			r.practices[id] = clonePracticeSession(session)
			r.mu.Unlock()
			ok = true
		}
	}
	return clonePracticeSession(session), ok
}

func (r *Repository) updatePractice(id string, update func(*PracticeSession) error) (PracticeSession, error) {
	if _, ok := r.practice(id); !ok {
		return PracticeSession{}, notFound()
	}
	r.mu.Lock()
	session, ok := r.practices[id]
	if !ok {
		r.mu.Unlock()
		return PracticeSession{}, notFound()
	}
	if err := update(&session); err != nil {
		r.mu.Unlock()
		return PracticeSession{}, err
	}
	result := clonePracticeSession(session)
	if err := r.savePracticeContent(result); err != nil {
		r.mu.Unlock()
		return PracticeSession{}, persistenceUnavailable(err)
	}
	r.practices[id] = clonePracticeSession(session)
	r.mu.Unlock()
	return result, nil
}

func (r *Repository) savePracticeContent(session PracticeSession) error {
	copySession := clonePracticeSession(session)
	currentQuestion := session.CurrentQuestion
	answers := make(map[string]string, len(session.Submissions))
	for id, submission := range session.Submissions {
		answers[id] = submission.AnswerText
	}
	status := "active"
	if session.Finished {
		status = "finished"
	}
	return r.savePersistentContent(store.ContentRecord{
		ID: session.ID, Skill: string(domain.SkillAdaptivePractice), ClassID: session.ClassID, Status: status,
		Payload: map[string]any{
			"studentId": session.StudentID,
			tutoringRecordKey: persistentMap(tutoringPersistentRecord{
				PracticeSession: &copySession, CurrentQuestion: &currentQuestion,
				PracticeSubmissions: session.Submissions, PracticeAnswers: answers,
			}),
		},
		CreatedAt: session.CreatedAt, UpdatedAt: session.UpdatedAt,
	})
}

func (r *Repository) saveInteraction(interaction ClassroomInteraction) error {
	r.mu.Lock()
	if _, exists := r.interactions[interaction.ID]; exists {
		r.mu.Unlock()
		return errVersionConflict
	}
	if err := r.saveInteractionContent(interaction); err != nil {
		r.mu.Unlock()
		return persistenceUnavailable(err)
	}
	r.interactions[interaction.ID] = cloneInteraction(interaction)
	r.mu.Unlock()
	return nil
}

func (r *Repository) interaction(id string) (ClassroomInteraction, bool) {
	r.mu.RLock()
	interaction, ok := r.interactions[id]
	r.mu.RUnlock()
	if !ok {
		if record, found := r.persistentRecord(id); found && record.Interaction != nil {
			interaction = cloneInteraction(*record.Interaction)
			interaction.Answers = record.ClassroomAnswers
			restoreClassroomOwners(&interaction)
			r.mu.Lock()
			r.interactions[id] = cloneInteraction(interaction)
			r.mu.Unlock()
			ok = true
		}
	}
	return cloneInteraction(interaction), ok
}

func (r *Repository) activeInteraction(classID string) (ClassroomInteraction, bool) {
	for _, content := range r.memory.ListContents(classID, string(domain.SkillClassroomCluster)) {
		record, valid := decodePersistentRecord(content)
		if !valid || record.Interaction == nil {
			continue
		}
		interaction := cloneInteraction(*record.Interaction)
		interaction.Answers = record.ClassroomAnswers
		restoreClassroomOwners(&interaction)
		r.mu.Lock()
		if _, exists := r.interactions[interaction.ID]; !exists {
			r.interactions[interaction.ID] = interaction
		}
		r.mu.Unlock()
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var active []ClassroomInteraction
	for _, interaction := range r.interactions {
		if interaction.ClassID == classID && interaction.Status == "open" {
			active = append(active, cloneInteraction(interaction))
		}
	}
	if len(active) == 0 {
		return ClassroomInteraction{}, false
	}
	sort.Slice(active, func(i, j int) bool {
		if active[i].CreatedAt.Equal(active[j].CreatedAt) {
			return active[i].ID < active[j].ID
		}
		return active[i].CreatedAt.After(active[j].CreatedAt)
	})
	return active[0], true
}

func (r *Repository) updateInteraction(id string, update func(*ClassroomInteraction) error) (ClassroomInteraction, error) {
	if _, ok := r.interaction(id); !ok {
		return ClassroomInteraction{}, notFound()
	}
	r.mu.Lock()
	interaction, ok := r.interactions[id]
	if !ok {
		r.mu.Unlock()
		return ClassroomInteraction{}, notFound()
	}
	if err := update(&interaction); err != nil {
		r.mu.Unlock()
		return ClassroomInteraction{}, err
	}
	result := cloneInteraction(interaction)
	if err := r.saveInteractionContent(result); err != nil {
		r.mu.Unlock()
		return ClassroomInteraction{}, persistenceUnavailable(err)
	}
	r.interactions[id] = cloneInteraction(interaction)
	r.mu.Unlock()
	return result, nil
}

func (r *Repository) saveInteractionContent(interaction ClassroomInteraction) error {
	copyInteraction := cloneInteraction(interaction)
	return r.savePersistentContent(store.ContentRecord{
		ID: interaction.ID, Skill: string(domain.SkillClassroomCluster),
		ClassID: interaction.ClassID, Status: interaction.Status,
		Payload: map[string]any{
			"teacherId": interaction.TeacherID,
			tutoringRecordKey: persistentMap(tutoringPersistentRecord{
				Interaction: &copyInteraction, ClassroomAnswers: interaction.Answers,
			}),
		},
		CreatedAt: interaction.CreatedAt, UpdatedAt: interaction.UpdatedAt,
	})
}

func (r *Repository) savePersistentContent(content store.ContentRecord) error {
	r.memory.SaveContent(content)
	return r.memory.EnsureContentDurable(content.ID)
}

func (r *Repository) persistentRecord(id string) (tutoringPersistentRecord, bool) {
	content, ok := r.memory.GetContent(strings.TrimSpace(id))
	if !ok {
		return tutoringPersistentRecord{}, false
	}
	return decodePersistentRecord(content)
}

func decodePersistentRecord(content store.ContentRecord) (tutoringPersistentRecord, bool) {
	value, ok := content.Payload[tutoringRecordKey]
	if !ok {
		return tutoringPersistentRecord{}, false
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return tutoringPersistentRecord{}, false
	}
	var record tutoringPersistentRecord
	if err := json.Unmarshal(encoded, &record); err != nil {
		return tutoringPersistentRecord{}, false
	}
	return record, true
}

func persistentMap(record tutoringPersistentRecord) map[string]any {
	encoded, err := json.Marshal(record)
	if err != nil {
		return map[string]any{}
	}
	var value map[string]any
	if err := json.Unmarshal(encoded, &value); err != nil {
		return map[string]any{}
	}
	return value
}

func restoreClassroomOwners(interaction *ClassroomInteraction) {
	if interaction.Answers == nil {
		interaction.Answers = make(map[string]ClassroomAnswer)
		return
	}
	for studentID, answer := range interaction.Answers {
		answer.StudentID = studentID
		interaction.Answers[studentID] = answer
	}
}

func (r *Repository) seedQuestions() {
	if err := r.addTutorQuestion(TutorQuestion{
		ID:           "q-wrong-1",
		Stem:         "高斯消元中交换增广矩阵两行时，右端常数项应如何处理？",
		FullSolution: "交换增广矩阵的两行时，系数和右端常数项必须一起交换，才能保持方程组等价。",
		KeyConceptGroups: [][]string{
			{"右端", "常数项"},
			{"同步", "一起", "同时", "交换"},
		},
		NegativeTerms: []string{"不需要", "不用", "无需", "不必", "不能"},
		Demo:          true,
	}); err != nil {
		panic("invalid built-in tutoring question: " + err.Error())
	}

	questions := []PracticeQuestion{
		{PublicQuestion: PublicQuestion{ID: "matrix-e-1", KnowledgePoint: "矩阵基本运算", Difficulty: "easy", Stem: "交换矩阵的两行会改变矩阵的行数吗？", Hint: "行交换只改变顺序。", Verified: true}, Answer: "不会", AcceptableAnswers: []string{"不改变", "不会改变"}},
		{PublicQuestion: PublicQuestion{ID: "gauss-e-1", KnowledgePoint: "高斯消元", Difficulty: "easy", Stem: "高斯消元允许交换两行吗？", Hint: "回忆三种初等行变换。", Verified: true}, Answer: "允许", AcceptableAnswers: []string{"可以", "是"}},
		{PublicQuestion: PublicQuestion{ID: "gauss-e-2", KnowledgePoint: "高斯消元", Difficulty: "easy", Stem: "行变换应作用于系数矩阵还是整个增广矩阵？", Hint: "方程两边必须保持等价。", Verified: true}, Answer: "整个增广矩阵", AcceptableAnswers: []string{"增广矩阵"}},
		{PublicQuestion: PublicQuestion{ID: "gauss-m-1", KnowledgePoint: "高斯消元", Difficulty: "medium", Stem: "交换增广矩阵两行时，右端常数项如何处理？", Hint: "把每一行看作一个完整方程。", Verified: true}, Answer: "一起交换", AcceptableAnswers: []string{"同步交换", "同时交换"}, Prerequisite: "矩阵基本运算"},
		{PublicQuestion: PublicQuestion{ID: "gauss-m-2", KnowledgePoint: "高斯消元", Difficulty: "medium", Stem: "消元的直接目标是什么？", Hint: "观察主元下方元素。", Verified: true}, Answer: "将主元下方元素化为零", AcceptableAnswers: []string{"主元下方化零", "构造阶梯形"}, Prerequisite: "矩阵基本运算"},
		{PublicQuestion: PublicQuestion{ID: "calculation-m-1", KnowledgePoint: "消元计算", Difficulty: "medium", Stem: "将数值 6 除以 3，结果是多少？", Hint: "检查除法计算。", Verified: true}, Answer: "2"},
		{PublicQuestion: PublicQuestion{ID: "gauss-h-1", KnowledgePoint: "高斯消元", Difficulty: "hard", Stem: "为什么初等行变换保持方程组解集不变？", Hint: "每种行变换都可逆。", Verified: true}, Answer: "初等行变换可逆", AcceptableAnswers: []string{"因为行变换可逆"}},
		{PublicQuestion: PublicQuestion{ID: "gauss-h-2", KnowledgePoint: "高斯消元", Difficulty: "hard", Stem: "出现全零系数行但右端非零意味着什么？", Hint: "该行表示 0 等于非零常数。", Verified: true}, Answer: "方程组无解", AcceptableAnswers: []string{"无解"}},
		{PublicQuestion: PublicQuestion{ID: "gauss-h-3", KnowledgePoint: "高斯消元", Difficulty: "hard", Stem: "阶梯形中非零行数等于什么？", Hint: "联系矩阵秩的定义。", Verified: true}, Answer: "矩阵的秩", AcceptableAnswers: []string{"秩"}},
	}
	for _, question := range questions {
		if err := r.addPracticeQuestion(question); err != nil {
			panic("invalid built-in practice question: " + err.Error())
		}
	}
}

func validateTutorQuestion(question TutorQuestion) error {
	if question.ID == "" || question.Stem == "" || question.FullSolution == "" || len(question.KeyConceptGroups) == 0 {
		return invalid("tutoring question is incomplete")
	}
	if runeLength(question.ID) > 128 || runeLength(question.Stem) > 1000 || runeLength(question.FullSolution) > 3000 {
		return invalid("tutoring question has an invalid field length")
	}
	if err := security.CheckInput(context.Background(), question.Stem+"\n"+question.FullSolution); err != nil {
		return unsafeInput(err)
	}
	for _, group := range question.KeyConceptGroups {
		if len(group) == 0 {
			return invalid("tutoring question has an empty concept group")
		}
		for _, concept := range group {
			if strings.TrimSpace(concept) == "" || runeLength(concept) > 100 {
				return invalid("tutoring question has an invalid concept")
			}
		}
	}
	for _, negative := range question.NegativeTerms {
		if strings.TrimSpace(negative) == "" || runeLength(negative) > 100 {
			return invalid("tutoring question has an invalid negative term")
		}
	}
	return nil
}

func validatePracticeQuestion(question PracticeQuestion) error {
	if question.ID == "" || question.KnowledgePoint == "" || question.Stem == "" || question.Answer == "" || !question.Verified {
		return invalid("practice question is incomplete or unverified")
	}
	if runeLength(question.ID) > 128 || runeLength(question.KnowledgePoint) > 100 ||
		runeLength(question.Stem) > 1000 || runeLength(question.Answer) > 500 || runeLength(question.Hint) > 500 {
		return invalid("practice question has an invalid field length")
	}
	if err := security.CheckInput(context.Background(), question.KnowledgePoint+"\n"+question.Stem+"\n"+question.Hint); err != nil {
		return unsafeInput(err)
	}
	if runeLength(question.Prerequisite) > 100 {
		return invalid("practice question prerequisite is too long")
	}
	for _, acceptable := range question.AcceptableAnswers {
		if strings.TrimSpace(acceptable) == "" || runeLength(acceptable) > 500 {
			return invalid("practice question has an invalid acceptable answer")
		}
	}
	switch question.Difficulty {
	case "easy", "medium", "hard":
		return nil
	default:
		return invalid("practice question has an invalid difficulty")
	}
}

func scopeKey(classID, studentID string) string { return classID + "\x00" + studentID }

func cloneReviewPlan(plan ReviewPlan) ReviewPlan {
	plan.Steps = append([]ReviewStep(nil), plan.Steps...)
	return plan
}

func cloneTutorQuestion(question TutorQuestion) TutorQuestion {
	question.NegativeTerms = append([]string(nil), question.NegativeTerms...)
	groups := question.KeyConceptGroups
	question.KeyConceptGroups = make([][]string, len(groups))
	for i := range groups {
		question.KeyConceptGroups[i] = append([]string(nil), groups[i]...)
	}
	return question
}

func newPersistedTutorQuestion(question TutorQuestion) persistedTutorQuestion {
	return persistedTutorQuestion{
		ID: question.ID, StudentID: question.StudentID, ClassID: question.ClassID,
		Stem: question.Stem, FullSolution: question.FullSolution,
		KeyConceptGroups: question.KeyConceptGroups, NegativeTerms: question.NegativeTerms,
		Demo: question.Demo,
	}
}

func (question persistedTutorQuestion) question() TutorQuestion {
	return cloneTutorQuestion(TutorQuestion{
		ID: question.ID, StudentID: question.StudentID, ClassID: question.ClassID,
		Stem: question.Stem, FullSolution: question.FullSolution,
		KeyConceptGroups: question.KeyConceptGroups, NegativeTerms: question.NegativeTerms,
		Demo: question.Demo,
	})
}

func cloneTutorSession(session TutorSession) TutorSession {
	session.History = append([]TutorTurn(nil), session.History...)
	return session
}

func clonePracticeSession(session PracticeSession) PracticeSession {
	session.Attempts = append([]PracticeAttempt(nil), session.Attempts...)
	session.MasteryEvidence = append([]string(nil), session.MasteryEvidence...)
	submissions := session.Submissions
	session.Submissions = make(map[string]PracticeSubmission, len(submissions))
	for key, value := range submissions {
		session.Submissions[key] = value
	}
	if session.Question != nil {
		question := *session.Question
		session.Question = &question
	}
	return session
}

func cloneInteraction(interaction ClassroomInteraction) ClassroomInteraction {
	answers := interaction.Answers
	interaction.Answers = make(map[string]ClassroomAnswer, len(answers))
	for key, value := range answers {
		interaction.Answers[key] = value
	}
	if interaction.Analysis != nil {
		analysis := *interaction.Analysis
		analysis.Clusters = append([]ClassroomCluster(nil), analysis.Clusters...)
		for i := range analysis.Clusters {
			analysis.Clusters[i].MemberIDs = append([]string(nil), analysis.Clusters[i].MemberIDs...)
		}
		interaction.Analysis = &analysis
	}
	return interaction
}

package tutoring

import (
	"context"
	"time"
)

const learningStateWindow = 14 * 24 * time.Hour

type MasteryLevel string

const (
	MasteryWeak             MasteryLevel = "weak"
	MasteryDeveloping       MasteryLevel = "developing"
	MasterySolid            MasteryLevel = "solid"
	MasteryInsufficientData MasteryLevel = "insufficient_data"
)

type KnowledgePointState struct {
	KnowledgePoint  string       `json:"knowledgePoint"`
	EventCount      int          `json:"eventCount"`
	EvidenceCount   int          `json:"evidenceCount"`
	CorrectCount    int          `json:"correctCount"`
	IncorrectCount  int          `json:"incorrectCount"`
	UnderstoodCount int          `json:"understoodCount"`
	SuccessRate     float64      `json:"successRate"`
	MasteryLevel    MasteryLevel `json:"masteryLevel"`
}

type LearningState struct {
	StudentID       string                `json:"studentId"`
	ClassID         string                `json:"classId"`
	WindowDays      int                   `json:"windowDays"`
	KnowledgePoints []KnowledgePointState `json:"knowledgePoints"`
}

type ReviewStepDraft struct {
	Type           string `json:"type"`
	Title          string `json:"title"`
	Detail         string `json:"detail,omitempty"`
	KnowledgePoint string `json:"knowledgePoint"`
}

type ReviewStep struct {
	ID             string     `json:"id"`
	Order          int        `json:"order"`
	Type           string     `json:"type"`
	Title          string     `json:"title"`
	Detail         string     `json:"detail,omitempty"`
	KnowledgePoint string     `json:"knowledgePoint"`
	Completed      bool       `json:"completed"`
	CompletedAt    *time.Time `json:"completedAt,omitempty"`
}

type ReviewPlan struct {
	ID        string       `json:"id"`
	StudentID string       `json:"studentId"`
	ClassID   string       `json:"classId"`
	Status    string       `json:"status"`
	Steps     []ReviewStep `json:"steps"`
	CreatedAt time.Time    `json:"createdAt"`
	UpdatedAt time.Time    `json:"updatedAt"`
}

type ReviewPathResult struct {
	Skipped bool        `json:"skipped"`
	Reason  string      `json:"reason,omitempty"`
	Plan    *ReviewPlan `json:"plan,omitempty"`
}

type ReviewModelInput struct {
	KnowledgePoints []KnowledgePointState `json:"knowledgePoints"`
	Mistakes        []ReviewMistake       `json:"mistakes"`
	Target          string                `json:"targetKnowledgePoint"`
}

type ReviewMistake struct {
	KnowledgePoint string `json:"knowledgePoint"`
	ErrorCause     string `json:"errorCause"`
}

type ReviewPathModel interface {
	GenerateReviewPath(context.Context, ReviewModelInput) ([]ReviewStepDraft, error)
}

type TutorQuestion struct {
	ID               string     `json:"id"`
	StudentID        string     `json:"-"`
	ClassID          string     `json:"-"`
	Stem             string     `json:"stem"`
	FullSolution     string     `json:"-"`
	KeyConceptGroups [][]string `json:"-"`
	NegativeTerms    []string   `json:"-"`
	Demo             bool       `json:"demo"`
}

type TutorTurn struct {
	Round     int       `json:"round"`
	Reply     string    `json:"reply"`
	Hit       bool      `json:"hit"`
	Prompt    string    `json:"prompt,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type TutorSession struct {
	ID            string      `json:"id"`
	StudentID     string      `json:"studentId"`
	ClassID       string      `json:"classId"`
	QuestionID    string      `json:"questionId"`
	QuestionStem  string      `json:"questionStem"`
	Demo          bool        `json:"demo"`
	Round         int         `json:"round"`
	HitStreak     int         `json:"hitStreak"`
	MaxRounds     int         `json:"maxRounds"`
	Released      bool        `json:"answerReleased"`
	ReleaseReason string      `json:"releaseReason,omitempty"`
	FullSolution  string      `json:"fullSolution,omitempty"`
	LastPrompt    string      `json:"prompt,omitempty"`
	History       []TutorTurn `json:"history"`
	Version       int         `json:"-"`
	CreatedAt     time.Time   `json:"createdAt"`
	UpdatedAt     time.Time   `json:"updatedAt"`
}

type TutorModelInput struct {
	QuestionStem string `json:"questionStem"`
	Round        int    `json:"round"`
	HitStreak    int    `json:"hitStreak"`
	StudentReply string `json:"studentReply,omitempty"`
}

type TutorModel interface {
	NextTutorQuestion(context.Context, TutorModelInput) (string, error)
}

type PublicQuestion struct {
	ID             string `json:"id"`
	KnowledgePoint string `json:"knowledgePoint"`
	Difficulty     string `json:"difficulty"`
	Stem           string `json:"stem"`
	Hint           string `json:"-"`
	Verified       bool   `json:"verified"`
}

type PracticeQuestion struct {
	PublicQuestion
	ClassID           string `json:"classId,omitempty"`
	Answer            string
	AcceptableAnswers []string
	Prerequisite      string
}

type PracticeSubmission struct {
	ID         string         `json:"submissionId"`
	AnswerText string         `json:"-"`
	Result     PracticeResult `json:"result"`
}

type PracticeResult struct {
	SubmissionID string          `json:"submissionId"`
	Outcome      string          `json:"outcome"`
	Transition   string          `json:"transition"`
	Finished     bool            `json:"finished"`
	FinishReason string          `json:"finishReason,omitempty"`
	Question     *PublicQuestion `json:"question,omitempty"`
	Summary      *PracticeReport `json:"summary,omitempty"`
}

type PracticeReport struct {
	QuestionCount       int      `json:"questionCount"`
	CorrectCount        int      `json:"correctCount"`
	MasteryEvidence     []string `json:"masteryEvidence"`
	CountsTowardGrade   bool     `json:"countsTowardGrade"`
	FinalKnowledgePoint string   `json:"finalKnowledgePoint"`
}

type PracticeAttempt struct {
	QuestionID     string `json:"questionId"`
	KnowledgePoint string `json:"knowledgePoint"`
	Difficulty     string `json:"difficulty"`
	Outcome        string `json:"outcome"`
	Transition     string `json:"transition"`
}

type PracticeSession struct {
	ID                 string                        `json:"id"`
	StudentID          string                        `json:"studentId"`
	ClassID            string                        `json:"classId"`
	KnowledgePoint     string                        `json:"knowledgePoint"`
	Difficulty         string                        `json:"difficulty"`
	ConsecutiveCorrect int                           `json:"consecutiveCorrect"`
	HardCorrect        int                           `json:"hardCorrect"`
	QuestionCount      int                           `json:"questionCount"`
	CorrectCount       int                           `json:"correctCount"`
	HintsUsed          int                           `json:"hintsUsed"`
	CurrentQuestion    PracticeQuestion              `json:"-"`
	Question           *PublicQuestion               `json:"question,omitempty"`
	Attempts           []PracticeAttempt             `json:"attempts"`
	Submissions        map[string]PracticeSubmission `json:"-"`
	MasteryEvidence    []string                      `json:"masteryEvidence"`
	Finished           bool                          `json:"finished"`
	FinishReason       string                        `json:"finishReason,omitempty"`
	CreatedAt          time.Time                     `json:"createdAt"`
	UpdatedAt          time.Time                     `json:"updatedAt"`
}

type ClassroomAnswer struct {
	AnonymousID string    `json:"anonymousId"`
	StudentID   string    `json:"-"`
	Text        string    `json:"text,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type ClusterDraft struct {
	ID        string   `json:"id"`
	MemberIDs []string `json:"memberIds"`
	Texts     []string `json:"texts"`
}

type ClusterSummaryDraft struct {
	GroupID   string   `json:"groupId"`
	MemberIDs []string `json:"memberIds"`
	Label     string   `json:"label"`
	FollowUp  string   `json:"followUp"`
}

type ClassroomCluster struct {
	ID        string   `json:"id"`
	MemberIDs []string `json:"memberIds"`
	Size      int      `json:"size"`
	Label     string   `json:"label"`
	FollowUp  string   `json:"followUp"`
}

type ClassroomAnalysis struct {
	Skipped       bool               `json:"skipped"`
	Reason        string             `json:"reason,omitempty"`
	Mode          string             `json:"mode,omitempty"`
	AnswerCount   int                `json:"answerCount"`
	Anonymous     bool               `json:"anonymous"`
	CountsAsGrade bool               `json:"countsAsGrade"`
	Clusters      []ClassroomCluster `json:"clusters,omitempty"`
}

type ClassroomInteraction struct {
	ID        string                     `json:"id"`
	ClassID   string                     `json:"classId"`
	TeacherID string                     `json:"teacherId"`
	Prompt    string                     `json:"prompt"`
	Status    string                     `json:"status"`
	Answers   map[string]ClassroomAnswer `json:"-"`
	Analysis  *ClassroomAnalysis         `json:"analysis,omitempty"`
	CreatedAt time.Time                  `json:"createdAt"`
	ClosedAt  *time.Time                 `json:"closedAt,omitempty"`
	UpdatedAt time.Time                  `json:"updatedAt"`
}

type ClassroomClusterModel interface {
	SummarizeClusters(context.Context, []ClusterDraft) ([]ClusterSummaryDraft, error)
}

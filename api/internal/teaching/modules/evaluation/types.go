package evaluation

import "time"

// 杨瑾文模块：作业评价与班级分析 —— 状态与结构约定。

const (
	SubQueued              = "queued"
	SubOCRProcessing       = "ocr_processing"
	SubAwaitConfirm        = "awaiting_transcription_confirm"
	SubGrading             = "grading"
	SubNeedsTeacherReview  = "needs_teacher_review"
	SubGradedSuggested     = "graded_suggested"
	SubTeacherFinal        = "teacher_final"
	SubFailedKept          = "failed_submission_kept" // 模型失败不丢提交
	SubDrafting            = "drafting"

	TriggerInterval = "interval"
	TriggerManual   = "manual"

	MinClassReviewSamples = 5
	MinDiagnosisSamples   = 3
	LowConfidenceThreshold = 0.8
)

// GradeStep 主观题步骤划分结果。
type GradeStep struct {
	Index   int    `json:"index"`
	Text    string `json:"text"`
	OK      bool   `json:"ok"`
	Comment string `json:"comment,omitempty"`
}

// ScoreChange 教师改分审计。
type ScoreChange struct {
	Before    *float64  `json:"before"`
	After     float64   `json:"after"`
	Note      string    `json:"note"`
	TeacherID string    `json:"teacherId"`
	At        time.Time `json:"at"`
}

// LearningEvent 诊断用时序证据。
type LearningEvent struct {
	ID        string    `json:"id"`
	StudentID string    `json:"studentId"`
	ClassID   string    `json:"classId"`
	Kind      string    `json:"kind"` // assignment | practice | help | tutoring
	Summary   string    `json:"summary"`
	KP        string    `json:"knowledgePoint,omitempty"`
	At        time.Time `json:"at"`
}

// SimilarityReport 原创性审查证据包（不含违规结论）。
type SimilarityReport struct {
	AttemptID string           `json:"attemptId"`
	Evidence  []SimilarityItem `json:"evidence"`
	Note      string           `json:"note"`
	Verdict   any              `json:"verdict"` // 恒为 null：不自动判定
}

type SimilarityItem struct {
	Fragment   string  `json:"fragment"`
	Source     string  `json:"source"`
	Similarity float64 `json:"similarity"`
	Location   string  `json:"location"`
}

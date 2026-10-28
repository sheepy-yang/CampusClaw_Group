// Package evaluation 杨瑾文模块骨架：多模态批改 / 讲评 / 诊断 / 作答回放与原创性审查。
package evaluation

import "context"

// MultimodalGradeService 多模态主观题批改（含手写转写确认、首错定位）。
type MultimodalGradeService struct{}

func (MultimodalGradeService) EnqueueGrade(_ context.Context, submissionID string) error {
	_ = submissionID
	panic("TODO: evaluation.MultimodalGradeService.EnqueueGrade")
}

func (MultimodalGradeService) ConfirmTranscription(_ context.Context, submissionID string, text string) error {
	_ = submissionID
	_ = text
	panic("TODO: evaluation.MultimodalGradeService.ConfirmTranscription")
}

func (MultimodalGradeService) TeacherOverrideScore(_ context.Context, resultID string, score float64, note string) error {
	_ = resultID
	_ = score
	_ = note
	panic("TODO: evaluation.MultimodalGradeService.TeacherOverrideScore")
}

// ClassReviewService 班级讲评（人数门槛 <5 不调模型；送模前匿名化）。
type ClassReviewService struct{}

func (ClassReviewService) Generate(_ context.Context, assignmentID string) error {
	_ = assignmentID
	panic("TODO: evaluation.ClassReviewService.Generate")
}

// DiagnosisService 学习诊断（样本 <3 只显示样本不足）。
type DiagnosisService struct{}

func (DiagnosisService) Generate(_ context.Context, studentID string) error {
	_ = studentID
	panic("TODO: evaluation.DiagnosisService.Generate")
}

// AnswerReplayService 作答过程回放与原创性审查（只展示证据，不自动判定违规）。
type AnswerReplayService struct{}

func (AnswerReplayService) SaveSnapshot(_ context.Context, attemptID string, content string) error {
	_ = attemptID
	_ = content
	panic("TODO: evaluation.AnswerReplayService.SaveSnapshot")
}

func (AnswerReplayService) Timeline(_ context.Context, attemptID string) (any, error) {
	_ = attemptID
	panic("TODO: evaluation.AnswerReplayService.Timeline")
}

func (AnswerReplayService) SimilarityEvidence(_ context.Context, attemptID string) (any, error) {
	_ = attemptID
	panic("TODO: evaluation.AnswerReplayService.SimilarityEvidence")
}

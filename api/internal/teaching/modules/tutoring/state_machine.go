package tutoring

import "github.com/sheepy-yang/CampusClaw_Group/api/internal/apperrors"

// PracticeState 自适应练习会话状态摘要。
type PracticeState struct {
	MasteryEvidence    []string
	ErrorType          string
	ConsecutiveCorrect int
	HintsUsed          int
	CurrentDifficulty  string
	KnowledgePoint     string
}

// TransitionReason 可解释的状态转移原因。
type TransitionReason string

const (
	ReasonLevelUp      TransitionReason = "level_up"       // 正确且解释充分
	ReasonPrerequisite TransitionReason = "prerequisite"   // 概念性错误 → 前置知识
	ReasonSameLevelVar TransitionReason = "same_level_var" // 计算失误 → 同层变式
	ReasonFinished     TransitionReason = "finished"       // 掌握 / 题量上限 / 主动结束
)

// NextTransition declares the state-machine boundary without inventing business rules.
func NextTransition(_ PracticeState, _ string) (PracticeState, TransitionReason, error) {
	return PracticeState{}, "", apperrors.ErrNotImplemented
}

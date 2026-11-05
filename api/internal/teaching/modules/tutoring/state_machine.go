package tutoring

// PracticeState 自适应练习会话状态摘要。
type PracticeState struct {
	MasteryEvidence    []string
	ErrorType          string
	ConsecutiveCorrect int
	HintsUsed          int
	CurrentDifficulty  string
	KnowledgePoint     string
	QuestionCount      int
}

// TransitionReason 可解释的状态转移原因。
type TransitionReason string

const (
	ReasonLevelUp      TransitionReason = "level_up"       // 正确且解释充分
	ReasonPrerequisite TransitionReason = "prerequisite"   // 概念性错误 → 前置知识
	ReasonSameLevelVar TransitionReason = "same_level_var" // 计算失误 → 同层变式
	ReasonFinished     TransitionReason = "finished"       // 掌握 / 题量上限 / 主动结束
)

var difficultyOrder = []string{"easy", "medium", "hard"}

// NextTransition 根据作答结果计算下一状态。
// outcome: correct | conceptual_error | calculation_error | quit
func NextTransition(state PracticeState, outcome string) (PracticeState, TransitionReason) {
	next := state
	next.QuestionCount = state.QuestionCount + 1

	if outcome == "quit" || next.QuestionCount >= 10 {
		next.ErrorType = outcome
		return next, ReasonFinished
	}

	switch outcome {
	case "correct":
		next.ConsecutiveCorrect = state.ConsecutiveCorrect + 1
		next.ErrorType = ""
		next.MasteryEvidence = append(append([]string{}, state.MasteryEvidence...), "correct@"+state.CurrentDifficulty)
		if next.ConsecutiveCorrect >= 2 {
			next.CurrentDifficulty = bumpDifficulty(state.CurrentDifficulty)
			if state.CurrentDifficulty == "hard" && next.ConsecutiveCorrect >= 3 {
				return next, ReasonFinished
			}
			next.ConsecutiveCorrect = 0
			return next, ReasonLevelUp
		}
		return next, ReasonSameLevelVar
	case "conceptual_error":
		next.ConsecutiveCorrect = 0
		next.ErrorType = "conceptual_error"
		next.CurrentDifficulty = "easy"
		next.KnowledgePoint = "前置:" + state.KnowledgePoint
		return next, ReasonPrerequisite
	case "calculation_error":
		next.ConsecutiveCorrect = 0
		next.ErrorType = "calculation_error"
		return next, ReasonSameLevelVar
	default:
		next.ErrorType = outcome
		return next, ReasonSameLevelVar
	}
}

func bumpDifficulty(cur string) string {
	for i, d := range difficultyOrder {
		if d == cur && i+1 < len(difficultyOrder) {
			return difficultyOrder[i+1]
		}
	}
	return cur
}

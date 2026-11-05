package tutoring

import "testing"

func TestNextTransitionBranches(t *testing.T) {
	st := PracticeState{CurrentDifficulty: "medium", KnowledgePoint: "高斯消元"}

	next, reason := NextTransition(st, "correct")
	if reason != ReasonSameLevelVar && reason != ReasonLevelUp {
		t.Fatalf("correct => %s", reason)
	}
	next.ConsecutiveCorrect = 1
	next, reason = NextTransition(next, "correct")
	if reason != ReasonLevelUp {
		t.Fatalf("two correct should level up, got %s diff=%s", reason, next.CurrentDifficulty)
	}
	if next.CurrentDifficulty != "hard" {
		t.Fatalf("expected hard, got %s", next.CurrentDifficulty)
	}

	next, reason = NextTransition(PracticeState{CurrentDifficulty: "medium", KnowledgePoint: "高斯消元"}, "conceptual_error")
	if reason != ReasonPrerequisite || next.CurrentDifficulty != "easy" {
		t.Fatalf("conceptual => %#v %s", next, reason)
	}

	next, reason = NextTransition(PracticeState{CurrentDifficulty: "medium"}, "calculation_error")
	if reason != ReasonSameLevelVar {
		t.Fatalf("calc => %s", reason)
	}

	next, reason = NextTransition(PracticeState{QuestionCount: 9}, "correct")
	if reason != ReasonFinished {
		t.Fatalf("q>=10 should finish, got %s count=%d", reason, next.QuestionCount)
	}
}

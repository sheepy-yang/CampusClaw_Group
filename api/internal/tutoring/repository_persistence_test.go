package tutoring

import (
	"testing"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

func TestRepositoryReloadsTutoringSessionsFromDurableContent(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	memory := store.New()
	repository := NewRepository(memory)
	now := time.Now().UTC()

	plan := ReviewPlan{
		ID: "plan-persist", StudentID: "stu-1", ClassID: "class-demo", Status: "active",
		Steps:     []ReviewStep{{ID: "step-1", Order: 1, Type: "read", Title: "阅读", KnowledgePoint: "高斯消元"}},
		CreatedAt: now, UpdatedAt: now,
	}
	repository.savePlan(plan)

	question := TutorQuestion{
		ID: "question-persist", StudentID: "stu-1", ClassID: "class-demo",
		Stem: "题目", FullSolution: "完整解析", KeyConceptGroups: [][]string{{"关键概念"}},
	}
	if err := repository.addTutorQuestion(question); err != nil {
		t.Fatal(err)
	}
	tutor := TutorSession{
		ID: "tutor-persist", StudentID: "stu-1", ClassID: "class-demo",
		QuestionID: question.ID, QuestionStem: "题目", MaxRounds: 5,
		LastPrompt: "下一步？", CreatedAt: now, UpdatedAt: now,
	}
	if err := repository.saveTutor(tutor, 0); err != nil {
		t.Fatal(err)
	}

	practice := PracticeSession{
		ID: "practice-persist", StudentID: "stu-1", ClassID: "class-demo",
		KnowledgePoint: "高斯消元", Difficulty: "medium",
		CurrentQuestion: PracticeQuestion{
			PublicQuestion: PublicQuestion{ID: "q1", KnowledgePoint: "高斯消元", Difficulty: "medium", Stem: "题目", Verified: true},
			Answer:         "内部答案",
		},
		Submissions: map[string]PracticeSubmission{
			"submission-1": {ID: "submission-1", AnswerText: "学生答案"},
		},
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repository.savePractice(practice); err != nil {
		t.Fatal(err)
	}

	interaction := ClassroomInteraction{
		ID: "interaction-persist", ClassID: "class-demo", TeacherID: "tea-1",
		Prompt: "问题", Status: "open",
		Answers: map[string]ClassroomAnswer{
			"stu-1": {AnonymousID: "answer-1", StudentID: "stu-1", Text: "回答", UpdatedAt: now},
		},
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repository.saveInteraction(interaction); err != nil {
		t.Fatal(err)
	}

	reloaded := NewRepository(memory)
	if got, ok := reloaded.getLatestPlan("class-demo", "stu-1"); !ok || got.ID != plan.ID {
		t.Fatalf("reloaded plan=%#v ok=%v", got, ok)
	}
	if got, ok := reloaded.tutor(tutor.ID); !ok || got.Version != 1 || got.LastPrompt != tutor.LastPrompt {
		t.Fatalf("reloaded tutor=%#v ok=%v", got, ok)
	}
	if got, ok := reloaded.tutorQuestion(question.ID); !ok || got.FullSolution != question.FullSolution {
		t.Fatalf("reloaded tutor question=%#v ok=%v", got, ok)
	}
	if got, ok := reloaded.practice(practice.ID); !ok ||
		got.CurrentQuestion.Answer != "内部答案" ||
		got.Submissions["submission-1"].AnswerText != "学生答案" {
		t.Fatalf("reloaded practice=%#v ok=%v", got, ok)
	}
	if got, ok := reloaded.activeInteraction("class-demo"); !ok ||
		got.Answers["stu-1"].Text != "回答" {
		t.Fatalf("reloaded interaction=%#v ok=%v", got, ok)
	}
}

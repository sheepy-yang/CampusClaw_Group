package teaching_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/queue"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/teaching"
)

func TestVirtualClassroomUsesConfirmedLessonDurableTaskAndIdempotency(t *testing.T) {
	original := store.Default
	store.Default = store.New()
	t.Cleanup(func() { store.Default = original })

	now := time.Now().UTC()
	store.Default.SaveContent(store.ContentRecord{
		ID: "lesson-confirmed", Skill: string(teaching.SkillLessonPlan), ClassID: "class-demo",
		Status: string(teaching.StatusConfirmed), Payload: map[string]any{
			"title": "高斯消元", "objectives": []string{"解释等价变换"},
		},
		CreatedAt: now, UpdatedAt: now,
	})
	broker := queue.NewMemoryQueue(4)
	pipeline := teaching.NewPipeline(teaching.PipelineDependencies{Queue: broker})
	input := map[string]any{
		"classId": "class-demo", "userId": "tea-1", "lessonPlanId": "lesson-confirmed",
		"objective": "检查等价变换条件", "rounds": 3, "idempotencyKey": "virtual:test-001",
	}
	task, err := pipeline.Start(context.Background(), teaching.SkillVirtualClassroom, input)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != teaching.TaskQueued || task.ObjectID == "" {
		t.Fatalf("task = %#v", task)
	}
	replayed, err := pipeline.Start(context.Background(), teaching.SkillVirtualClassroom, input)
	if err != nil || replayed.ID != task.ID || replayed.ObjectID != task.ObjectID {
		t.Fatalf("idempotent replay=%#v err=%v", replayed, err)
	}
	conflictInput := map[string]any{}
	for key, value := range input {
		conflictInput[key] = value
	}
	conflictInput["rounds"] = 4
	if _, err := pipeline.Start(context.Background(), teaching.SkillVirtualClassroom, conflictInput); !errors.Is(err, teaching.ErrPipelineConflict) {
		t.Fatalf("conflicting replay err=%v", err)
	}

	delivery, err := broker.Dequeue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.NewWorker().Handle(context.Background(), delivery.Job); err != nil {
		t.Fatal(err)
	}
	report, ok := store.Default.GetContent(task.ObjectID)
	if !ok || report.Status != "pending" {
		t.Fatalf("report = %#v, exists=%v", report, ok)
	}
	drill, ok := report.Payload["drillReport"].(map[string]any)
	if !ok || drill["label"] != "教学演练（非真实学生评价）" {
		t.Fatalf("drill report = %#v", report.Payload["drillReport"])
	}
	if dialogue, ok := drill["dialogue"].([]map[string]any); !ok || len(dialogue) == 0 {
		t.Fatalf("dialogue = %#v", drill["dialogue"])
	}
}

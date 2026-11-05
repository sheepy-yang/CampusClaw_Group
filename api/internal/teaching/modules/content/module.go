// Package content 刘恺骐模块：智能命题 / 备课 / 预习单 / 虚拟课堂演练。
package content

import (
	"context"
	"fmt"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/agent"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/domain"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/queue"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

type Services struct {
	Orch     agent.Orchestrator
	Verifier agent.VerifierAgent
	Enqueue  queue.Enqueuer
}

func New(orch agent.Orchestrator, verifier agent.VerifierAgent, enq queue.Enqueuer) *Services {
	return &Services{Orch: orch, Verifier: verifier, Enqueue: enq}
}

type GenerateResult struct {
	TaskID    string              `json:"taskId"`
	ContentID string              `json:"contentId"`
	Status    domain.ContentStatus `json:"status"`
	Payload   map[string]any      `json:"payload"`
	Issues    []string            `json:"issues,omitempty"`
}

func (s *Services) GenerateProposition(ctx context.Context, classID string, input map[string]any) (*GenerateResult, error) {
	input["skill"] = "proposition"
	res, err := s.Orch.Run(ctx, nil, input)
	if err != nil {
		return nil, err
	}
	ok, issues, err := s.Verifier.Verify(ctx, res, agent.VerifyRules{RequireCitations: true, SchemaName: "proposition"})
	if err != nil {
		return nil, err
	}
	status := domain.StatusPending
	if !ok {
		res.Payload["needsReview"] = true
		res.Payload["verifyIssues"] = issues
	}
	return s.persist(classID, domain.SkillProposition, status, res.Payload)
}

func (s *Services) GenerateLessonPlan(ctx context.Context, classID string, input map[string]any) (*GenerateResult, error) {
	input["skill"] = "lesson_plan"
	res, err := s.Orch.Run(ctx, nil, input)
	if err != nil {
		return nil, err
	}
	ok, issues, _ := s.Verifier.Verify(ctx, res, agent.VerifyRules{RequireCitations: true, SchemaName: "lesson_plan"})
	if !ok {
		res.Payload["needsReview"] = true
		res.Payload["verifyIssues"] = issues
	}
	return s.persist(classID, domain.SkillLessonPlan, domain.StatusPending, res.Payload)
}

func (s *Services) GeneratePreviewSheet(ctx context.Context, classID string, input map[string]any) (*GenerateResult, error) {
	input["skill"] = "预习"
	res, err := s.Orch.Run(ctx, nil, input)
	if err != nil {
		return nil, err
	}
	ok, issues, _ := s.Verifier.Verify(ctx, res, agent.VerifyRules{RequireCitations: false, SchemaName: "preview_sheet"})
	if !ok {
		return &GenerateResult{Status: domain.StatusRejected, Issues: issues, Payload: res.Payload}, fmt.Errorf("preview sheet rejected: %v", issues)
	}
	return s.persist(classID, domain.SkillPreviewSheet, domain.StatusPending, res.Payload)
}

func (s *Services) StartVirtualClassroom(ctx context.Context, classID, lessonPlanID string, rounds int) (*GenerateResult, error) {
	contentID := store.NewID("content")
	taskID := store.NewID("task")
	payload := map[string]any{
		"lessonPlanId": lessonPlanID,
		"rounds":       rounds,
		"label":        "教学演练（非真实学生评价）",
	}
	now := time.Now()
	store.Default.SaveContent(store.ContentRecord{
		ID: contentID, Skill: string(domain.SkillVirtualClassroom), ClassID: classID,
		Status: string(domain.StatusGenerating), Payload: payload, CreatedAt: now, UpdatedAt: now,
	})
	store.Default.SaveTask(store.TaskRecord{
		ID: taskID, Skill: string(domain.SkillVirtualClassroom), ClassID: classID, Status: "queued",
		Steps: []store.TaskStep{{Name: "enqueue", Status: "done"}},
	})
	_ = s.Enqueue.Enqueue(ctx, queue.Job{
		ID: store.NewID("job"), Type: queue.JobVirtualClassroom,
		IdempotencyKey: "vc-" + contentID,
		Payload: map[string]any{
			"taskId": taskID, "contentId": contentID, "lessonPlanId": lessonPlanID,
			"dialogue": []map[string]any{
				{"role": "薄弱", "utterance": "为什么右边也要变？", "grounded": true},
				{"role": "教师", "utterance": "因为方程左右同时变换才等价。", "grounded": true},
			},
		},
	})
	return &GenerateResult{TaskID: taskID, ContentID: contentID, Status: domain.StatusGenerating, Payload: payload}, nil
}

func (s *Services) persist(classID string, skill domain.SkillID, status domain.ContentStatus, payload map[string]any) (*GenerateResult, error) {
	contentID := store.NewID("content")
	taskID := store.NewID("task")
	now := time.Now()
	store.Default.SaveContent(store.ContentRecord{
		ID: contentID, Skill: string(skill), ClassID: classID, Status: string(status),
		Payload: payload, CreatedAt: now, UpdatedAt: now,
	})
	store.Default.SaveTask(store.TaskRecord{
		ID: taskID, Skill: string(skill), ClassID: classID, Status: "pending",
		Steps: []store.TaskStep{{Name: "generate", Status: "done"}, {Name: "verify", Status: "done"}},
	})
	return &GenerateResult{TaskID: taskID, ContentID: contentID, Status: status, Payload: payload}, nil
}

type PropositionService struct{ *Services }
type LessonPlanService struct{ *Services }
type PreviewSheetService struct{ *Services }
type VirtualClassroomService struct{ *Services }

func (p PropositionService) Generate(ctx context.Context, input map[string]any) error {
	_, err := p.GenerateProposition(ctx, "class-demo", input)
	return err
}
func (p LessonPlanService) Generate(ctx context.Context, input map[string]any) error {
	_, err := p.GenerateLessonPlan(ctx, "class-demo", input)
	return err
}
func (p PreviewSheetService) Generate(ctx context.Context, input map[string]any) error {
	_, err := p.GeneratePreviewSheet(ctx, "class-demo", input)
	return err
}
func (p VirtualClassroomService) StartDrill(ctx context.Context, lessonPlanID string, rounds int) error {
	_, err := p.StartVirtualClassroom(ctx, "class-demo", lessonPlanID, rounds)
	return err
}

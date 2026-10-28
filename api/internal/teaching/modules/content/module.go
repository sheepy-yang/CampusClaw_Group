// Package content 刘恺骐模块骨架：智能命题 / 备课 / 预习单 / 虚拟课堂演练。
package content

import "context"

// PropositionService 智能命题。
type PropositionService struct{}

func (PropositionService) Generate(_ context.Context, _ map[string]any) error {
	panic("TODO: content.PropositionService.Generate")
}

// LessonPlanService 智能备课。
type LessonPlanService struct{}

func (LessonPlanService) Generate(_ context.Context, _ map[string]any) error {
	panic("TODO: content.LessonPlanService.Generate")
}

// PreviewSheetService 智能预习单。
type PreviewSheetService struct{}

func (PreviewSheetService) Generate(_ context.Context, _ map[string]any) error {
	panic("TODO: content.PreviewSheetService.Generate")
}

// VirtualClassroomService 虚拟学生课堂演练。
type VirtualClassroomService struct{}

func (VirtualClassroomService) StartDrill(_ context.Context, lessonPlanID string, rounds int) error {
	_ = lessonPlanID
	_ = rounds
	panic("TODO: content.VirtualClassroomService.StartDrill")
}

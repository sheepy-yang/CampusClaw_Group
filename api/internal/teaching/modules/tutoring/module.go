// Package tutoring 刘沛鑫模块骨架：复习路径 / 追问辅导 / 课堂聚类 / 自适应闯关。
package tutoring

import (
	"context"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/apperrors"
)

// ReviewPathService 个性化复习路径（3～5 步；无有效记录不调模型）。
type ReviewPathService struct{}

func (ReviewPathService) Generate(_ context.Context, studentID string) error {
	_ = studentID
	return apperrors.ErrNotImplemented
}

// SocraticTutorService 追问式辅导（答案释放条件满足前不展示完整解析）。
type SocraticTutorService struct{}

func (SocraticTutorService) NextQuestion(_ context.Context, sessionID string, reply string) error {
	_ = sessionID
	_ = reply
	return apperrors.ErrNotImplemented
}

// ClassroomClusterService 课堂回答聚类（<5 人不调模型；最多 4 类）。
type ClassroomClusterService struct{}

func (ClassroomClusterService) Cluster(_ context.Context, sessionID string) error {
	_ = sessionID
	return apperrors.ErrNotImplemented
}

// AdaptivePracticeService 自适应练习闯关状态机。
type AdaptivePracticeService struct{}

func (AdaptivePracticeService) Start(_ context.Context, studentID string, knowledgePoint string) error {
	_ = studentID
	_ = knowledgePoint
	return apperrors.ErrNotImplemented
}

func (AdaptivePracticeService) SubmitAnswer(_ context.Context, sessionID string, answer string) error {
	_ = sessionID
	_ = answer
	return apperrors.ErrNotImplemented
}

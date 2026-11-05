package teaching

import "github.com/sheepy-yang/CampusClaw_Group/api/internal/domain"

// 再导出，保持原 teaching 包对外 API。
type SkillID = domain.SkillID
type ContentStatus = domain.ContentStatus
type QuestionStatus = domain.QuestionStatus

const (
	SkillProposition      = domain.SkillProposition
	SkillLessonPlan       = domain.SkillLessonPlan
	SkillVirtualClassroom = domain.SkillVirtualClassroom
	SkillMultimodalGrade  = domain.SkillMultimodalGrade
	SkillAnswerReplay     = domain.SkillAnswerReplay
	SkillClassReview      = domain.SkillClassReview
	SkillDiagnosis        = domain.SkillDiagnosis
	SkillReviewPath       = domain.SkillReviewPath
	SkillAdaptivePractice = domain.SkillAdaptivePractice
	SkillPreviewSheet     = domain.SkillPreviewSheet
	SkillSocraticTutor    = domain.SkillSocraticTutor
	SkillClassroomCluster = domain.SkillClassroomCluster

	StatusGenerating = domain.StatusGenerating
	StatusPending    = domain.StatusPending
	StatusConfirmed  = domain.StatusConfirmed
	StatusRejected   = domain.StatusRejected
	StatusPublished  = domain.StatusPublished

	QDraft     = domain.QDraft
	QValidated = domain.QValidated
	QReviewing = domain.QReviewing
	QPublished = domain.QPublished
	QArchived  = domain.QArchived
)

func AllSkills() []SkillID { return domain.AllSkills() }

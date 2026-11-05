package domain

// SkillID 十二项可独立启用/停用/验收的技能模块。
type SkillID string

const (
	SkillProposition      SkillID = "proposition"
	SkillLessonPlan       SkillID = "lesson_plan"
	SkillVirtualClassroom SkillID = "virtual_classroom"
	SkillMultimodalGrade  SkillID = "multimodal_grade"
	SkillAnswerReplay     SkillID = "answer_replay"
	SkillClassReview      SkillID = "class_review"
	SkillDiagnosis        SkillID = "diagnosis"
	SkillReviewPath       SkillID = "review_path"
	SkillAdaptivePractice SkillID = "adaptive_practice"
	SkillPreviewSheet     SkillID = "preview_sheet"
	SkillSocraticTutor    SkillID = "socratic_tutor"
	SkillClassroomCluster SkillID = "classroom_cluster"
)

func AllSkills() []SkillID {
	return []SkillID{
		SkillProposition, SkillLessonPlan, SkillVirtualClassroom,
		SkillMultimodalGrade, SkillAnswerReplay, SkillClassReview, SkillDiagnosis,
		SkillReviewPath, SkillAdaptivePractice, SkillPreviewSheet,
		SkillSocraticTutor, SkillClassroomCluster,
	}
}

// ContentStatus 教案 / 预习单 / 讲评 / 诊断等内容生命周期。
type ContentStatus string

const (
	StatusGenerating ContentStatus = "generating"
	StatusPending    ContentStatus = "pending"
	StatusConfirmed  ContentStatus = "confirmed"
	StatusRejected   ContentStatus = "rejected"
	StatusPublished  ContentStatus = "published"
)

// QuestionStatus 试题生命周期。
type QuestionStatus string

const (
	QDraft     QuestionStatus = "draft"
	QValidated QuestionStatus = "validated"
	QReviewing QuestionStatus = "reviewing"
	QPublished QuestionStatus = "published"
	QArchived  QuestionStatus = "archived"
)

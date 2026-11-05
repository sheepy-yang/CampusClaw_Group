package domain

// SkillID 十二项可独立启用/停用/验收的技能模块。
type SkillID string

const (
	// 刘恺骐：智能内容生产与统一编排
	SkillProposition      SkillID = "proposition"
	SkillLessonPlan       SkillID = "lesson_plan"
	SkillPreviewSheet     SkillID = "preview_sheet"
	SkillVirtualClassroom SkillID = "virtual_classroom"

	// 杨瑾文：作业评价与班级分析
	SkillMultimodalGrade SkillID = "multimodal_grade"
	SkillClassReview     SkillID = "class_review"
	SkillDiagnosis       SkillID = "diagnosis"
	SkillAnswerReplay    SkillID = "answer_replay"

	// 刘沛鑫：个性化辅导与课堂互动
	SkillReviewPath       SkillID = "review_path"
	SkillSocraticTutor    SkillID = "socratic_tutor"
	SkillClassroomCluster SkillID = "classroom_cluster"
	SkillAdaptivePractice SkillID = "adaptive_practice"
)

// Owner 技能负责人（与 README 分工一致）。
type Owner string

const (
	OwnerLiuKaiqi   Owner = "刘恺骐"
	OwnerYangJinwen Owner = "杨瑾文"
	OwnerLiuPeixin  Owner = "刘沛鑫"
)

func (s SkillID) Owner() Owner {
	switch s {
	case SkillProposition, SkillLessonPlan, SkillPreviewSheet, SkillVirtualClassroom:
		return OwnerLiuKaiqi
	case SkillMultimodalGrade, SkillClassReview, SkillDiagnosis, SkillAnswerReplay:
		return OwnerYangJinwen
	case SkillReviewPath, SkillSocraticTutor, SkillClassroomCluster, SkillAdaptivePractice:
		return OwnerLiuPeixin
	default:
		return ""
	}
}

func AllSkills() []SkillID {
	return []SkillID{
		SkillProposition, SkillLessonPlan, SkillPreviewSheet, SkillVirtualClassroom,
		SkillMultimodalGrade, SkillClassReview, SkillDiagnosis, SkillAnswerReplay,
		SkillReviewPath, SkillSocraticTutor, SkillClassroomCluster, SkillAdaptivePractice,
	}
}

func SkillsByOwner(owner Owner) []SkillID {
	var out []SkillID
	for _, s := range AllSkills() {
		if s.Owner() == owner {
			out = append(out, s)
		}
	}
	return out
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

package teaching

// SkillID 十二项可独立启用/停用/验收的技能模块。
type SkillID string

const (
	SkillProposition       SkillID = "proposition"         // 智能命题
	SkillLessonPlan        SkillID = "lesson_plan"         // 智能备课
	SkillVirtualClassroom  SkillID = "virtual_classroom"   // 虚拟学生课堂演练
	SkillMultimodalGrade   SkillID = "multimodal_grade"    // 多模态主观题批改
	SkillAnswerReplay      SkillID = "answer_replay"       // 作答过程回放与原创性审查
	SkillClassReview       SkillID = "class_review"        // 班级讲评
	SkillDiagnosis         SkillID = "diagnosis"           // 学习诊断
	SkillReviewPath        SkillID = "review_path"         // 个性化复习路径
	SkillAdaptivePractice  SkillID = "adaptive_practice"   // 自适应练习闯关
	SkillPreviewSheet      SkillID = "preview_sheet"       // 智能预习单
	SkillSocraticTutor     SkillID = "socratic_tutor"      // 追问式辅导
	SkillClassroomCluster  SkillID = "classroom_cluster"   // 课堂回答聚类
)

// AllSkills 返回全部技能 ID，供班级开关与验收枚举使用。
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
	StatusGenerating ContentStatus = "generating" // 生成中
	StatusPending    ContentStatus = "pending"    // 待确认
	StatusConfirmed  ContentStatus = "confirmed"  // 已确认
	StatusRejected   ContentStatus = "rejected"   // 已驳回
	StatusPublished  ContentStatus = "published"  // 已发布
)

// QuestionStatus 试题生命周期。
type QuestionStatus string

const (
	QDraft      QuestionStatus = "draft"
	QValidated  QuestionStatus = "validated"
	QReviewing  QuestionStatus = "reviewing"
	QPublished  QuestionStatus = "published"
	QArchived   QuestionStatus = "archived"
)

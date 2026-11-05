export type AppRole = "teacher" | "student";
export type SkillOwner = "刘恺骐" | "杨瑾文" | "刘沛鑫" | "公共";

export interface RouteManifestItem {
  role: AppRole;
  path: string;
  label: string;
  capability: string;
  owner: SkillOwner;
}

// The manifest is the product-level route inventory. Individual feature
// changes replace placeholders without changing navigation ad hoc.
export const routeManifest: readonly RouteManifestItem[] = [
  // 刘恺骐：智能内容生产与统一编排
  { role: "teacher", path: "proposition", label: "智能命题", capability: "proposition", owner: "刘恺骐" },
  { role: "teacher", path: "lesson-plan", label: "智能备课", capability: "lesson_plan", owner: "刘恺骐" },
  { role: "teacher", path: "preview-sheet", label: "智能预习单", capability: "preview_sheet", owner: "刘恺骐" },
  { role: "teacher", path: "virtual-classroom", label: "虚拟课堂演练", capability: "virtual_classroom", owner: "刘恺骐" },

  // 杨瑾文：作业评价与班级分析
  { role: "teacher", path: "grade", label: "多模态批改 / 改分", capability: "multimodal_grade", owner: "杨瑾文" },
  { role: "teacher", path: "review", label: "班级讲评", capability: "class_review", owner: "杨瑾文" },
  { role: "teacher", path: "diagnosis", label: "学习诊断", capability: "diagnosis", owner: "杨瑾文" },
  { role: "teacher", path: "replay", label: "作答回放与原创性审查", capability: "answer_replay", owner: "杨瑾文" },

  // 刘沛鑫：个性化辅导与课堂互动
  { role: "teacher", path: "cluster", label: "课堂回答聚类", capability: "classroom_cluster", owner: "刘沛鑫" },
  { role: "student", path: "review-path", label: "个性化复习路径", capability: "review_path", owner: "刘沛鑫" },
  { role: "student", path: "adaptive", label: "自适应练习闯关", capability: "adaptive_practice", owner: "刘沛鑫" },
  { role: "student", path: "tutor", label: "追问式辅导", capability: "socratic_tutor", owner: "刘沛鑫" },
  { role: "student", path: "classroom", label: "课堂短文本互动", capability: "classroom_cluster", owner: "刘沛鑫" },

  // 公共平台
  { role: "teacher", path: "skills", label: "班级技能开关", capability: "skill_settings", owner: "公共" },
  { role: "teacher", path: "tasks", label: "任务与审计", capability: "task_audit", owner: "公共" },
  { role: "student", path: "assignments", label: "已发布试卷 / 作业", capability: "assignments", owner: "公共" },
  { role: "student", path: "preview", label: "预习单", capability: "preview_sheet", owner: "刘恺骐" },
  { role: "student", path: "attempt", label: "作答与版本回看", capability: "answer_replay", owner: "杨瑾文" },
];

export type AppRole = "teacher" | "student";

export interface RouteManifestItem {
  role: AppRole;
  path: string;
  label: string;
  capability: string;
}

// The manifest is the product-level route inventory. Individual feature
// changes replace placeholders without changing navigation ad hoc.
export const routeManifest: readonly RouteManifestItem[] = [
  { role: "teacher", path: "proposition", label: "智能命题", capability: "proposition" },
  { role: "teacher", path: "lesson-plan", label: "智能备课", capability: "lesson_plan" },
  { role: "teacher", path: "preview-sheet", label: "智能预习单", capability: "preview_sheet" },
  { role: "teacher", path: "virtual-classroom", label: "虚拟课堂演练", capability: "virtual_classroom" },
  { role: "teacher", path: "grade", label: "多模态批改 / 改分", capability: "multimodal_grade" },
  { role: "teacher", path: "review", label: "班级讲评", capability: "class_review" },
  { role: "teacher", path: "diagnosis", label: "学习诊断", capability: "diagnosis" },
  { role: "teacher", path: "replay", label: "作答回放与原创性审查", capability: "answer_replay" },
  { role: "teacher", path: "cluster", label: "课堂回答聚类", capability: "classroom_cluster" },
  { role: "teacher", path: "skills", label: "班级技能开关", capability: "skill_settings" },
  { role: "teacher", path: "tasks", label: "任务与审计", capability: "task_audit" },
  { role: "student", path: "assignments", label: "已发布试卷 / 作业", capability: "assignments" },
  { role: "student", path: "preview", label: "预习单", capability: "preview_sheet" },
  { role: "student", path: "review-path", label: "个性化复习路径", capability: "review_path" },
  { role: "student", path: "adaptive", label: "自适应练习闯关", capability: "adaptive_practice" },
  { role: "student", path: "tutor", label: "追问式辅导", capability: "socratic_tutor" },
  { role: "student", path: "classroom", label: "课堂短文本互动", capability: "classroom_cluster" },
  { role: "student", path: "attempt", label: "作答与版本回看", capability: "answer_replay" },
];

import { Link, Route, Routes } from "react-router-dom";
import { useEffect, useState } from "react";
import { api } from "../../api";
import { ConfirmPublish, SkillPanel } from "../../components/SkillPanel";
import { DiagnosisPage, GradePage, ReplayPage, ReviewPage } from "./Evaluation";

function SkillsSwitch() {
  const [skills, setSkills] = useState<Record<string, boolean>>({});
  useEffect(() => {
    api<Record<string, boolean>>("/api/v1/classes/class-demo/skills").then(setSkills).catch(console.error);
  }, []);
  return (
    <section className="panel grid">
      <h2>班级技能开关</h2>
      <div className="grid two">
        {Object.entries(skills).map(([k, v]) => (
          <label key={k} className="row">
            <input
              type="checkbox"
              checked={v}
              onChange={async (e) => {
                const next = { ...skills, [k]: e.target.checked };
                setSkills(next);
                await api("/api/v1/classes/class-demo/skills", { method: "PUT", body: JSON.stringify(next) });
              }}
            />
            {k}
          </label>
        ))}
      </div>
    </section>
  );
}

function Proposition() {
  const [contentId, setContentId] = useState("");
  return (
    <SkillPanel
      title="智能命题"
      description="生成与验证双通道；引用须来自讲义；教师逐题采纳后才能发布。"
      onRun={async () => {
        const res = await api<{ contentId: string }>("/api/v1/skills/proposition", {
          method: "POST",
          body: JSON.stringify({ materialId: "mat-1", count: 2 }),
        });
        setContentId(res.contentId);
        return res;
      }}
    >
      <ConfirmPublish contentId={contentId} />
    </SkillPanel>
  );
}

function LessonPlan() {
  const [contentId, setContentId] = useState("");
  return (
    <SkillPanel
      title="智能备课"
      description="事实性内容须有讲义依据；未经确认的教案不能发布。"
      onRun={async () => {
        const res = await api<{ contentId: string }>("/api/v1/skills/lesson-plan", {
          method: "POST",
          body: JSON.stringify({ objectives: ["高斯消元"] }),
        });
        setContentId(res.contentId);
        return res;
      }}
    >
      <ConfirmPublish contentId={contentId} />
    </SkillPanel>
  );
}

function VirtualClassroom() {
  return (
    <SkillPanel
      title="虚拟学生课堂演练"
      description="不同基础虚拟学生模拟提问；报告标注为教学演练，不评价真实学生。"
      actionLabel="开始演练"
      onRun={() =>
        api("/api/v1/skills/virtual-classroom", {
          method: "POST",
          body: JSON.stringify({ lessonPlanId: "lp-demo", rounds: 3 }),
        })
      }
    />
  );
}


function Cluster() {
  return (
    <SkillPanel
      title="课堂回答聚类"
      description="&lt;5 人不调模型；最多 4 类；结果不计成绩。"
      onRun={() =>
        api("/api/v1/skills/classroom-cluster", {
          method: "POST",
          body: JSON.stringify({ sessionId: "class-session-1" }),
        })
      }
    />
  );
}

function PreviewSheet() {
  const [contentId, setContentId] = useState("");
  return (
    <SkillPanel
      title="智能预习单"
      description="有效知识卡 &lt;3 不保存；教师确认后推送。"
      onRun={async () => {
        const res = await api<{ contentId: string }>("/api/v1/skills/preview-sheet", {
          method: "POST",
          body: JSON.stringify({ materialId: "mat-1" }),
        });
        setContentId(res.contentId);
        return res;
      }}
    >
      <ConfirmPublish contentId={contentId} />
    </SkillPanel>
  );
}

function Audit() {
  return (
    <SkillPanel
      title="审计记录"
      description="接口与技能调用可追溯。"
      actionLabel="刷新"
      onRun={() => api("/api/v1/audit")}
    />
  );
}

const teacherLinks = [
  ["proposition", "智能命题"],
  ["lesson-plan", "智能备课"],
  ["preview", "智能预习单"],
  ["virtual-classroom", "虚拟课堂演练"],
  ["grade", "多模态批改"],
  ["review", "班级讲评"],
  ["diagnosis", "学习诊断"],
  ["replay", "作答回放审查"],
  ["cluster", "课堂回答聚类"],
  ["skills", "技能开关"],
  ["audit", "审计"],
] as const;

export function TeacherHome() {
  return (
    <div className="workspace">
      <aside>
        <h1 className="workspace-title">教师工作台</h1>
        <p className="workspace-sub">内容生产 · 评价分析 · 班级洞察</p>
        <nav className="workspace-nav">
          {teacherLinks.map(([to, label]) => (
            <Link key={to} to={to}>{label}</Link>
          ))}
        </nav>
      </aside>
      <div className="workspace-main">
        <Routes>
          <Route path="proposition" element={<Proposition />} />
          <Route path="lesson-plan" element={<LessonPlan />} />
          <Route path="preview" element={<PreviewSheet />} />
          <Route path="virtual-classroom" element={<VirtualClassroom />} />
          <Route path="grade" element={<GradePage />} />
          <Route path="review" element={<ReviewPage />} />
          <Route path="diagnosis" element={<DiagnosisPage />} />
          <Route path="replay" element={<ReplayPage />} />
          <Route path="cluster" element={<Cluster />} />
          <Route path="skills" element={<SkillsSwitch />} />
          <Route path="audit" element={<Audit />} />
          <Route path="*" element={<p className="empty-hint">从左侧选择一项技能开始。</p>} />
        </Routes>
      </div>
    </div>
  );
}

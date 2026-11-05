import { Link, Route, Routes } from "react-router-dom";
import { useEffect, useState } from "react";
import { api, classApiPath, newIdempotencyKey } from "../../api";
import { ConfirmPublish, SkillPanel } from "../../components/SkillPanel";
import { MCPToolsPage } from "../../components/MCPTools";
import { DiagnosisPage, GradePage, ReplayPage, ReviewPage } from "./Evaluation";
import { AssistantConfigPage, AssignmentBuilderPage, MaterialsPage } from "./Platform";

function SkillsSwitch() {
  const [skills, setSkills] = useState<Record<string, boolean>>({});
  useEffect(() => {
    api<Record<string, boolean>>(classApiPath("/skills")).then(setSkills).catch(console.error);
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
                await api(classApiPath("/skills"), { method: "PUT", body: JSON.stringify(next) });
              }}
            />
            {k}
          </label>
        ))}
      </div>
    </section>
  );
}

type MaterialOption = { id: string; title: string };

function useMaterialSelection() {
  const [materials, setMaterials] = useState<MaterialOption[]>([]);
  const [materialId, setMaterialId] = useState("");
  useEffect(() => {
    api<MaterialOption[]>(classApiPath("/materials"))
      .then((items) => {
        setMaterials(items);
        setMaterialId((current) => current || items[0]?.id || "");
      })
      .catch(console.error);
  }, []);
  return { materials, materialId, setMaterialId };
}

function MaterialSelect({
  materials,
  materialId,
  setMaterialId,
}: ReturnType<typeof useMaterialSelection>) {
  return (
    <label className="muted">依据讲义
      <select value={materialId} onChange={(event) => setMaterialId(event.target.value)}>
        {materials.map((material) => <option key={material.id} value={material.id}>{material.title}</option>)}
      </select>
    </label>
  );
}

function Proposition() {
  const [contentId, setContentId] = useState("");
  const material = useMaterialSelection();
  return (
    <SkillPanel
      title="智能命题"
      description="生成与验证双通道；引用须来自讲义；教师逐题采纳后才能发布。"
      onRun={async () => {
        if (!material.materialId) throw new Error("请先为当前班级入库并选择讲义");
        const res = await api<{ contentId: string }>("/api/v1/skills/proposition", {
          method: "POST",
          body: JSON.stringify({
            topic: "高斯消元",
            materialId: material.materialId,
            count: 2,
            exerciseTypes: ["choice", "short_answer"],
            difficulty: "medium",
            idempotencyKey: newIdempotencyKey("proposition"),
          }),
        });
        setContentId(res.contentId);
        return res;
      }}
    >
      <MaterialSelect {...material} />
      <ConfirmPublish contentId={contentId} />
    </SkillPanel>
  );
}

function LessonPlan() {
  const [contentId, setContentId] = useState("");
  const material = useMaterialSelection();
  return (
    <SkillPanel
      title="智能备课"
      description="事实性内容须有讲义依据；未经确认的教案不能发布。"
      onRun={async () => {
        if (!material.materialId) throw new Error("请先为当前班级入库并选择讲义");
        const res = await api<{ contentId: string }>("/api/v1/skills/lesson-plan", {
          method: "POST",
          body: JSON.stringify({
            topic: "高斯消元",
            materialId: material.materialId,
            durationMinutes: 45,
            objectives: ["理解高斯消元的等价变换条件"],
            idempotencyKey: newIdempotencyKey("lesson-plan"),
          }),
        });
        setContentId(res.contentId);
        localStorage.setItem("campusclaw_lesson_plan_id", res.contentId);
        return res;
      }}
    >
      <MaterialSelect {...material} />
      <ConfirmPublish contentId={contentId} />
    </SkillPanel>
  );
}

function VirtualClassroom() {
  const [lessonPlanId, setLessonPlanId] = useState(() => localStorage.getItem("campusclaw_lesson_plan_id") || "");
  const [contentId, setContentId] = useState("");
  return (
    <SkillPanel
      title="虚拟学生课堂演练"
      description="不同基础虚拟学生模拟提问；报告标注为教学演练，不评价真实学生。"
      actionLabel="开始演练"
      onRun={async () => {
        const task = await api<{ id: string; objectId: string }>("/api/v1/skills/virtual-classroom", {
          method: "POST",
          headers: { "Idempotency-Key": newIdempotencyKey("virtual-classroom") },
          body: JSON.stringify({
            lessonPlanId,
            objective: "检验学生能否解释等价变换条件",
            rounds: 3,
          }),
        });
        setContentId(task.objectId);
        for (let attempt = 0; attempt < 40; attempt += 1) {
          await new Promise((resolve) => window.setTimeout(resolve, 250));
          const current = await api<{ status: string }>(`/api/v1/tasks/${task.id}`);
          if (current.status === "failed") throw new Error("课堂演练任务失败，原教案未被修改");
          if (current.status === "pending_confirmation") return api(`/api/v1/contents/${task.objectId}`);
        }
        throw new Error("课堂演练仍在运行，请稍后按内容编号查询");
      }}
    >
      <label className="muted">已确认教案编号
        <input value={lessonPlanId} onChange={(event) => setLessonPlanId(event.target.value)} placeholder="先生成并确认一份教案" />
      </label>
      <ConfirmPublish contentId={contentId} />
    </SkillPanel>
  );
}


function Cluster() {
  const [sessionId, setSessionId] = useState(() => localStorage.getItem("campusclaw_classroom_session") || "");
  const [prompt, setPrompt] = useState("为什么初等行变换保持方程组解集不变？");
  return (
    <SkillPanel
      title="课堂回答聚类"
      description="&lt;5 人不调模型；最多 4 类；结果不计成绩。"
      actionLabel={sessionId ? "关闭并分析" : "发起课堂互动"}
      onRun={async () => {
        if (!sessionId) {
          const interaction = await api<{ id: string }>("/api/v1/classroom/interactions", {
            method: "POST",
            body: JSON.stringify({ prompt }),
          });
          setSessionId(interaction.id);
          localStorage.setItem("campusclaw_classroom_session", interaction.id);
          return interaction;
        }
        await api(`/api/v1/classroom/interactions/${sessionId}/close`, {
          method: "POST",
          body: "{}",
        });
        const analysis = await api("/api/v1/skills/classroom-cluster", {
          method: "POST",
          body: JSON.stringify({ sessionId }),
        });
        localStorage.removeItem("campusclaw_classroom_session");
        setSessionId("");
        return analysis;
      }}
    >
      <label className="muted">互动问题
        <textarea rows={3} value={prompt} onChange={(event) => setPrompt(event.target.value)} disabled={Boolean(sessionId)} />
      </label>
      {sessionId && <p className="notice info">互动编号：{sessionId}。请学生提交回答后再关闭并分析。</p>}
    </SkillPanel>
  );
}

function PreviewSheet() {
  const [contentId, setContentId] = useState("");
  const material = useMaterialSelection();
  return (
    <SkillPanel
      title="智能预习单"
      description="有效知识卡 &lt;3 不保存；教师确认后推送。"
      onRun={async () => {
        if (!material.materialId) throw new Error("请先为当前班级入库并选择讲义");
        const res = await api<{ contentId: string }>("/api/v1/skills/preview-sheet", {
          method: "POST",
          body: JSON.stringify({
            topic: "高斯消元课前预习",
            materialId: material.materialId,
            count: 3,
            idempotencyKey: newIdempotencyKey("preview-sheet"),
          }),
        });
        setContentId(res.contentId);
        return res;
      }}
    >
      <MaterialSelect {...material} />
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
  ["materials", "讲义与检索"],
  ["assistant", "班级助手"],
  ["assignments", "作业发布"],
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
  ["mcp", "MCP 教学工具"],
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
          <Route path="materials" element={<MaterialsPage />} />
          <Route path="assistant" element={<AssistantConfigPage />} />
          <Route path="assignments" element={<AssignmentBuilderPage />} />
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
          <Route path="mcp" element={<MCPToolsPage />} />
          <Route path="audit" element={<Audit />} />
          <Route path="*" element={<p className="empty-hint">从左侧选择一项技能开始。</p>} />
        </Routes>
      </div>
    </div>
  );
}

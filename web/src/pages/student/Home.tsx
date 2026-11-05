import { Link, Route, Routes } from "react-router-dom";
import { useEffect, useState } from "react";
import { api, classApiPath, newIdempotencyKey, uploadAttachment } from "../../api";
import { SkillPanel } from "../../components/SkillPanel";
import { MCPToolsPage } from "../../components/MCPTools";
import { AssistantPage, AssignmentsPage } from "./Learning";

function Preview() {
  return (
    <SkillPanel
      title="预习单"
      description="仅查看教师确认并发布的预习内容。"
      actionLabel="查看已发布预习单"
      onRun={() => api(classApiPath("/contents?skill=preview_sheet"))}
    />
  );
}

function ReviewPath() {
  return (
    <SkillPanel
      title="个性化复习路径"
      description="根据近两周错题生成 3～5 步计划；自测不计成绩。"
      onRun={() => api("/api/v1/skills/review-path", { method: "POST", body: "{}" })}
    />
  );
}

function Adaptive() {
  const [sessionId, setSessionId] = useState("");
  const [question, setQuestion] = useState<{ id: string; stem: string; difficulty: string } | null>(null);
  const [answer, setAnswer] = useState("");
  const [knowledgePoint, setKnowledgePoint] = useState("");
  return (
    <div className="grid">
      <SkillPanel
        title="自适应练习闯关"
        description="状态转移可解释；动态题经校验后展示；不计正式成绩。"
        actionLabel="开始练习"
        onRun={async () => {
          if (!knowledgePoint.trim()) throw new Error("请先输入教师已发布作业中的知识点");
          const res = await api<{ id: string; question: { id: string; stem: string; difficulty: string } }>("/api/v1/skills/adaptive-practice/start", {
            method: "POST",
            body: JSON.stringify({ knowledgePoint: knowledgePoint.trim() }),
          });
          setSessionId(res.id);
          setQuestion(res.question);
          return res;
        }}
      >
        <label className="muted">知识点
          <input value={knowledgePoint} onChange={(event) => setKnowledgePoint(event.target.value)} placeholder="例如：高斯消元" />
        </label>
      </SkillPanel>
      {sessionId && (
        <section className="panel grid">
          <h2>当前题目</h2>
          <p><span className="pill">{question?.difficulty}</span> {question?.stem}</p>
          <textarea rows={3} value={answer} onChange={(event) => setAnswer(event.target.value)} placeholder="输入答案" />
          <div className="row">
            <button onClick={async () => {
              const result = await api<{ finished: boolean; question?: { id: string; stem: string; difficulty: string } }>(
                "/api/v1/skills/adaptive-practice/submit",
                {
                  method: "POST",
                  body: JSON.stringify({
                    sessionId,
                    submissionId: newIdempotencyKey("practice"),
                    answerText: answer,
                  }),
                },
              );
              setAnswer("");
              setQuestion(result.question || null);
              if (result.finished) setSessionId("");
              alert(JSON.stringify(result, null, 2));
            }} disabled={!answer.trim()}>提交答案</button>
            <button className="secondary" onClick={async () => {
              const result = await api("/api/v1/skills/adaptive-practice/hint", {
                method: "POST", body: JSON.stringify({ sessionId }),
              });
              alert(JSON.stringify(result, null, 2));
            }}>查看提示</button>
          </div>
        </section>
      )}
    </div>
  );
}

function Tutor() {
  const [sessionId, setSessionId] = useState("");
  const [reply, setReply] = useState("");
  return (
    <div className="grid">
      <SkillPanel
        title="追问式辅导"
        description="连续两轮击中要点 / 达轮次上限 / 要求讲解后才释放完整解析。"
        actionLabel="开始辅导"
        onRun={async () => {
          const res = await api<{ id: string }>("/api/v1/skills/socratic-tutor/start", {
            method: "POST",
            body: JSON.stringify({ questionId: "" }),
          });
          setSessionId(res.id);
          return res;
        }}
      />
      {sessionId && (
        <SkillPanel
          title="回答追问"
          description="根据系统追问补充你的推理；连续两轮命中关键概念后释放完整解析。"
          actionLabel="提交回答"
          onRun={() =>
            api("/api/v1/skills/socratic-tutor", {
              method: "POST",
              body: JSON.stringify({ sessionId, reply }),
            })
          }
        >
          <textarea value={reply} onChange={(e) => setReply(e.target.value)} rows={3} />
          <button className="secondary" onClick={() => setReply("请直接讲解")}>要求直接讲解</button>
        </SkillPanel>
      )}
    </div>
  );
}

function Classroom() {
  const [text, setText] = useState("秩相等说明有解");
  return (
    <SkillPanel
      title="课堂短文本互动"
      description="提交短答供教师聚类；不计成绩。"
      actionLabel="提交回答"
      onRun={async () => {
        const active = await api<{ interaction: { id: string } }>("/api/v1/classroom/interactions/active");
        return api("/api/v1/skills/classroom-cluster/answer", {
          method: "POST",
          body: JSON.stringify({ sessionId: active.interaction.id, text }),
        });
      }}
    >
      <textarea value={text} onChange={(e) => setText(e.target.value)} rows={3} />
    </SkillPanel>
  );
}

type MultimodalSubmission = {
  id: string;
  status: string;
  transcription?: string;
  score?: number;
  maxScore?: number;
  needsReview?: boolean;
};

function MultimodalAssignment() {
  const [assignments, setAssignments] = useState<Array<{ id: string; title: string }>>([]);
  const [assignmentId, setAssignmentId] = useState("");
  const [answerText, setAnswerText] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [submission, setSubmission] = useState<MultimodalSubmission | null>(null);
  const [transcription, setTranscription] = useState("");
  useEffect(() => {
    api<Array<{ id: string; title: string }>>(classApiPath("/assignments"))
      .then((items) => {
        setAssignments(items);
        if (items[0]) setAssignmentId(items[0].id);
      })
      .catch(() => undefined);
  }, []);
  return (
    <div className="grid">
      <SkillPanel
        title="主观题文本 / 手写图片提交"
        description="图片先真实上传到对象存储；异步转写完成后必须由你确认，才会进入建议评分。"
        actionLabel="提交并等待转写"
        onRun={async () => {
          const attachment = file ? await uploadAttachment(file) : null;
          const accepted = await api<{ submissionId: string }>("/api/v1/skills/multimodal-grade", {
            method: "POST",
            headers: { "Idempotency-Key": newIdempotencyKey("multimodal-grade") },
            body: JSON.stringify({
              assignmentId,
              answerText: answerText.trim(),
              imageDigest: attachment?.digest || "",
            }),
          });
          let current: MultimodalSubmission | null = null;
          for (let attempt = 0; attempt < 40; attempt += 1) {
            await new Promise((resolve) => window.setTimeout(resolve, 250));
            current = await api<MultimodalSubmission>(`/api/v1/skills/multimodal-grade/${accepted.submissionId}`);
            if (!["queued", "ocr_processing"].includes(current.status)) break;
          }
          if (!current) throw new Error("无法读取提交状态");
          setSubmission(current);
          setTranscription(current.transcription || "");
          return current;
        }}
      >
        <label className="muted">作业编号
          <select value={assignmentId} onChange={(event) => setAssignmentId(event.target.value)}>
            {assignments.map((assignment) => <option key={assignment.id} value={assignment.id}>{assignment.title}</option>)}
          </select>
        </label>
        <label className="muted">文本作答（与图片二选一）
          <textarea rows={4} value={answerText} onChange={(event) => setAnswerText(event.target.value)} disabled={Boolean(file)} />
        </label>
        <label className="muted">手写图片（JPEG / PNG / WebP，最大 10 MiB）
          <input type="file" accept="image/jpeg,image/png,image/webp" onChange={(event) => setFile(event.target.files?.[0] || null)} />
        </label>
      </SkillPanel>
      {submission?.status === "awaiting_transcription_confirm" && (
        <section className="panel grid">
          <h2>确认图片转写</h2>
          <textarea rows={6} value={transcription} onChange={(event) => setTranscription(event.target.value)} />
          <button onClick={async () => {
            const result = await api<{ submission: MultimodalSubmission }>("/api/v1/skills/multimodal-grade/confirm-transcription", {
              method: "POST",
              body: JSON.stringify({ submissionId: submission.id, text: transcription }),
            });
            setSubmission(result.submission);
          }}>确认转写并开始评分</button>
        </section>
      )}
      {submission && submission.status !== "awaiting_transcription_confirm" && (
        <section className="notice info">
          状态：{submission.status}；得分：{submission.score ?? "待教师确认"} / {submission.maxScore ?? 10}
          {submission.status === "failed_submission_kept" && (
            <button className="secondary" onClick={async () => {
              await api(`/api/v1/skills/multimodal-grade/${submission.id}/retry`, { method: "POST" });
              setSubmission({ ...submission, status: "queued" });
            }}>重试转写</button>
          )}
        </section>
      )}
    </div>
  );
}

function Attempt() {
  const [attemptId, setAttemptId] = useState("");
  const [content, setContent] = useState("");
  return (
    <div className="grid">
      <SkillPanel
        title="作答与版本回看"
        description="定时/主动保存快照；可补充引用与 AI 使用说明。"
        actionLabel="保存版本"
        onRun={async () => {
          const res = await api<{ attemptId: string }>("/api/v1/skills/answer-replay/snapshot", {
            method: "POST",
            body: JSON.stringify({ attemptId: attemptId || undefined, content, trigger: "manual" }),
          });
          setAttemptId(res.attemptId);
          return res;
        }}
      >
        <textarea value={content} onChange={(e) => setContent(e.target.value)} rows={4} placeholder="输入作答内容…" />
      </SkillPanel>
      {attemptId && (
        <SkillPanel
          title="回放时间线"
          description={`attempt: ${attemptId}`}
          actionLabel="查看"
          onRun={() => api(`/api/v1/skills/answer-replay/${attemptId}/timeline`)}
        />
      )}
    </div>
  );
}

const studentLinks = [
  ["assignments", "已发布作业"],
  ["multimodal", "主观题图片提交"],
  ["assistant", "班级学习助手"],
  ["preview", "预习单"],
  ["review-path", "复习路径"],
  ["adaptive", "自适应闯关"],
  ["tutor", "追问式辅导"],
  ["classroom", "课堂互动"],
  ["mcp", "MCP 学习工具"],
  ["attempt", "作答与回看"],
] as const;

export function StudentHome() {
  return (
    <div className="workspace">
      <aside>
        <h1 className="workspace-title">学生学习台</h1>
        <p className="workspace-sub">练习 · 辅导 · 过程回看</p>
        <nav className="workspace-nav">
          {studentLinks.map(([to, label]) => (
            <Link key={to} to={to}>{label}</Link>
          ))}
        </nav>
      </aside>
      <div className="workspace-main">
        <Routes>
          <Route path="assignments" element={<AssignmentsPage />} />
          <Route path="multimodal" element={<MultimodalAssignment />} />
          <Route path="assistant" element={<AssistantPage />} />
          <Route path="preview" element={<Preview />} />
          <Route path="review-path" element={<ReviewPath />} />
          <Route path="adaptive" element={<Adaptive />} />
          <Route path="tutor" element={<Tutor />} />
          <Route path="classroom" element={<Classroom />} />
          <Route path="mcp" element={<MCPToolsPage />} />
          <Route path="attempt" element={<Attempt />} />
          <Route path="*" element={<p className="empty-hint">从左侧选择一项学习活动。</p>} />
        </Routes>
      </div>
    </div>
  );
}

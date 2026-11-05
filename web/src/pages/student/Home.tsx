import { Link, Route, Routes } from "react-router-dom";
import { useState } from "react";
import { api } from "../../api";
import { SkillPanel } from "../../components/SkillPanel";

function Assignments() {
  return (
    <SkillPanel
      title="已发布试卷 / 作业"
      description="学生提交前看不到答案。"
      actionLabel="刷新列表"
      onRun={() => api("/api/v1/classes/class-demo/assignments")}
    />
  );
}

function Preview() {
  return (
    <SkillPanel
      title="预习单"
      description="仅查看教师确认并发布的预习内容。"
      actionLabel="查看讲义"
      onRun={() => api("/api/v1/classes/class-demo/materials")}
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
  return (
    <div className="grid">
      <SkillPanel
        title="自适应练习闯关"
        description="状态转移可解释；动态题经校验后展示；不计正式成绩。"
        actionLabel="开始练习"
        onRun={async () => {
          const res = await api<{ sessionId: string }>("/api/v1/skills/adaptive-practice/start", {
            method: "POST",
            body: JSON.stringify({ knowledgePoint: "高斯消元" }),
          });
          setSessionId(res.sessionId);
          return res;
        }}
      />
      {sessionId && (
        <section className="panel grid">
          <h2>作答分支</h2>
          <p className="muted">session: {sessionId}</p>
          <div className="row">
            {(["correct", "conceptual_error", "calculation_error", "quit"] as const).map((o) => (
              <button
                key={o}
                className="secondary"
                onClick={async () => {
                  const res = await api("/api/v1/skills/adaptive-practice/submit", {
                    method: "POST",
                    body: JSON.stringify({ sessionId, outcome: o }),
                  });
                  alert(JSON.stringify(res, null, 2));
                }}
              >
                {o}
              </button>
            ))}
          </div>
        </section>
      )}
    </div>
  );
}

function Tutor() {
  const [sessionId, setSessionId] = useState("");
  const [reply, setReply] = useState("右端也要同步变换");
  return (
    <div className="grid">
      <SkillPanel
        title="追问式辅导"
        description="连续两轮击中要点 / 达轮次上限 / 要求讲解后才释放完整解析。"
        actionLabel="开始辅导"
        onRun={async () => {
          const res = await api<{ sessionId: string }>("/api/v1/skills/socratic-tutor/start", {
            method: "POST",
            body: JSON.stringify({ questionId: "q-wrong-1" }),
          });
          setSessionId(res.sessionId);
          return res;
        }}
      />
      {sessionId && (
        <SkillPanel
          title="回答追问"
          description="试试包含「右端/增广/同步」以击中要点。"
          actionLabel="提交回答"
          onRun={() =>
            api("/api/v1/skills/socratic-tutor", {
              method: "POST",
              body: JSON.stringify({ sessionId, reply }),
            })
          }
        >
          <textarea value={reply} onChange={(e) => setReply(e.target.value)} rows={3} />
          <button className="secondary" onClick={() => setReply("直接讲解")}>要求直接讲解</button>
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
      onRun={() =>
        api("/api/v1/skills/classroom-cluster/answer", {
          method: "POST",
          body: JSON.stringify({ sessionId: "class-session-1", text }),
        })
      }
    >
      <textarea value={text} onChange={(e) => setText(e.target.value)} rows={3} />
    </SkillPanel>
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
  ["preview", "预习单"],
  ["review-path", "复习路径"],
  ["adaptive", "自适应闯关"],
  ["tutor", "追问式辅导"],
  ["classroom", "课堂互动"],
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
          <Route path="assignments" element={<Assignments />} />
          <Route path="preview" element={<Preview />} />
          <Route path="review-path" element={<ReviewPath />} />
          <Route path="adaptive" element={<Adaptive />} />
          <Route path="tutor" element={<Tutor />} />
          <Route path="classroom" element={<Classroom />} />
          <Route path="attempt" element={<Attempt />} />
          <Route path="*" element={<p className="empty-hint">从左侧选择一项学习活动。</p>} />
        </Routes>
      </div>
    </div>
  );
}

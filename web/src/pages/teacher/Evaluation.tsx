import { useEffect, useState } from "react";
import { api } from "../../api";

/* ---------- 共享工具 ---------- */

function errMsg(e: unknown) {
  return e instanceof Error ? e.message : String(e);
}

async function pollUntil<T>(
  fetcher: () => Promise<T>,
  pred: (v: T) => boolean,
  { interval = 450, attempts = 16 } = {}
): Promise<T | undefined> {
  let last: T | undefined;
  for (let i = 0; i < attempts; i++) {
    try {
      last = await fetcher();
      if (pred(last)) return last;
    } catch {
      /* 瞬时错误继续轮询 */
    }
    await new Promise((r) => setTimeout(r, interval));
  }
  return last;
}

function Raw({ data }: { data: unknown }) {
  if (data == null) return null;
  return (
    <details className="raw">
      <summary>原始返回</summary>
      <pre className="json">{JSON.stringify(data, null, 2)}</pre>
    </details>
  );
}

function Notice({ tone = "info", children }: { tone?: string; children: React.ReactNode }) {
  if (!children) return null;
  return <p className={`notice ${tone}`}>{children}</p>;
}

function GuardChips({ items }: { items: string[] }) {
  return (
    <div className="row guard">
      {items.map((s) => (
        <span key={s} className="chip">{s}</span>
      ))}
    </div>
  );
}

function Bar({ value, max, tone = "accent" }: { value: number; max: number; tone?: string }) {
  const pct = Math.max(0, Math.min(100, Math.round((value / (max || 1)) * 100)));
  return (
    <div className={`meter ${tone}`}>
      <div className="fill" style={{ width: `${pct}%` }} />
    </div>
  );
}

/* ============================================================
 * 多模态主观题批改
 * ========================================================== */

type Step = { index: number; text: string; ok: boolean; comment?: string };
type HistoryEntry = { before: number | null; after: number; note?: string; teacherId?: string; at?: string };
type Sub = {
  id: string;
  studentId?: string;
  assignmentId?: string;
  status: string;
  answerText?: string;
  transcription?: string;
  transcriptionConfirmed?: boolean;
  steps?: Step[];
  score?: number | null;
  suggestedScore?: number | null;
  maxScore?: number;
  firstErrorStep?: string;
  errorCause?: string;
  confidence?: number;
  needsReview?: boolean;
  teacherNote?: string;
  scoreHistory?: HistoryEntry[];
  modelFailed?: boolean;
  imageDigest?: string;
};

const STATUS_META: Record<string, { label: string; tone: string; stage: number }> = {
  queued: { label: "排队中", tone: "info", stage: 0 },
  ocr_processing: { label: "转写中", tone: "info", stage: 1 },
  awaiting_transcription_confirm: { label: "待确认转写", tone: "todo", stage: 2 },
  grading: { label: "AI 评分中", tone: "info", stage: 3 },
  needs_teacher_review: { label: "待教师复核", tone: "todo", stage: 4 },
  graded_suggested: { label: "已给建议分", tone: "done", stage: 5 },
  teacher_final: { label: "教师定分", tone: "done", stage: 5 },
  failed_submission_kept: { label: "模型失败（提交保留）", tone: "err", stage: 1 },
  drafting: { label: "草稿", tone: "info", stage: 0 },
};

const GRADE_PIPELINE = ["投递落库", "多模态转写", "学生确认转写", "步骤 / 首错 / 建议分", "教师复核", "定分"];

function StatusBadge({ status }: { status: string }) {
  const m = STATUS_META[status] || { label: status, tone: "info" };
  return <span className={`badge ${m.tone}`}>{m.label}</span>;
}

function Stepper({ stage, failed }: { stage: number; failed?: boolean }) {
  return (
    <ol className="stepper">
      {GRADE_PIPELINE.map((s, i) => (
        <li key={s} className={i < stage ? "done" : i === stage ? failed ? "failed" : "current" : ""}>
          <span className="dot" />
          <span className="lbl">{s}</span>
        </li>
      ))}
    </ol>
  );
}

export function GradePage() {
  const [list, setList] = useState<Sub[]>([]);
  const [sub, setSub] = useState<Sub | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [toast, setToast] = useState("");
  const [draft, setDraft] = useState("步骤1: 写出增广矩阵\n步骤2: 交换第1、2行\n步骤3: 回代得 x=1（此处计算失误）");
  const [editText, setEditText] = useState("");
  const [finalScore, setFinalScore] = useState("");
  const [finalNote, setFinalNote] = useState("");
  const [raw, setRaw] = useState<unknown>(null);

  const [objType, setObjType] = useState("choice");
  const [objGot, setObjGot] = useState("A");
  const [objWant, setObjWant] = useState("A");
  const [objResult, setObjResult] = useState<{ score: number; maxScore: number; correct: boolean; method: string; usedLLM: boolean } | null>(null);

  async function refreshList() {
    const data = await api<Sub[]>("/api/v1/skills/multimodal-grade?assignmentId=asg-1");
    setList(data || []);
  }

  useEffect(() => {
    refreshList().catch(() => undefined);
  }, []);

  async function select(id: string) {
    setErr("");
    const d = await api<Sub>(`/api/v1/skills/multimodal-grade/${id}`);
    setSub(d);
    setEditText(d.transcription || "");
    setFinalScore("");
    setFinalNote("");
  }

  async function enqueue() {
    setBusy(true);
    setErr("");
    setToast("已落库并投递，等待 Worker 转写…");
    try {
      const res = await api<{ submissionId: string; note: string }>("/api/v1/skills/multimodal-grade", {
        method: "POST",
        body: JSON.stringify({ assignmentId: "asg-1", answerText: draft, imageDigest: "sha256:demo", maxScore: 10 }),
      });
      setRaw(res);
      const done = await pollUntil<Sub>(
        () => api<Sub>(`/api/v1/skills/multimodal-grade/${res.submissionId}`),
        (v) => v.status !== "queued" && v.status !== "ocr_processing"
      );
      if (done) {
        setSub(done);
        setEditText(done.transcription || "");
        setToast(
          done.status === "failed_submission_kept"
            ? ""
            : "转写完成，请学生确认转写文本后进入评分。"
        );
        if (done.status === "failed_submission_kept") setErr("模型失败：提交已保留，可稍后重试，不会丢失。");
      }
      refreshList().catch(() => undefined);
    } catch (e) {
      setErr(errMsg(e));
    } finally {
      setBusy(false);
    }
  }

  async function confirmTranscription() {
    setBusy(true);
    setErr("");
    try {
      const res = await api<{ submission: Sub; pipeline: string[] }>("/api/v1/skills/multimodal-grade/confirm-transcription", {
        method: "POST",
        body: JSON.stringify({ submissionId: sub?.id || "", text: editText }),
      });
      setSub(res.submission);
      setRaw(res.pipeline);
      setToast(res.submission.needsReview ? "转写已确认；置信度偏低，已转入教师复核。" : "转写已确认，评分完成。");
      refreshList().catch(() => undefined);
    } catch (e) {
      setErr(errMsg(e));
    } finally {
      setBusy(false);
    }
  }

  async function override() {
    if (!sub || finalScore === "") return;
    setBusy(true);
    setErr("");
    try {
      const res = await api<{ before: number | null; after: number; authoritative: string; scoreHistory: HistoryEntry[] }>(
        "/api/v1/skills/multimodal-grade/override",
        {
          method: "POST",
          body: JSON.stringify({ resultId: sub.id, score: Number(finalScore), note: finalNote || "教师复核后改分" }),
        }
      );
      setRaw(res.scoreHistory);
      const max = sub.maxScore || 10;
      setToast(`已定分：${res.before ?? "—"} → ${res.after} / ${max}，前后记录已保留，以教师为准。`);
      await select(sub.id);
      refreshList().catch(() => undefined);
    } catch (e) {
      setErr(errMsg(e));
    } finally {
      setBusy(false);
    }
  }

  async function runObjective() {
    setErr("");
    try {
      setObjResult(
        await api("/api/v1/skills/multimodal-grade/objective", {
          method: "POST",
          body: JSON.stringify({ type: objType, studentAnswer: objGot, standardAnswer: objWant }),
        })
      );
    } catch (e) {
      setErr(errMsg(e));
    }
  }

  const stage = sub ? STATUS_META[sub.status]?.stage ?? 0 : -1;
  const failed = sub?.status === "failed_submission_kept";
  const max = sub?.maxScore || 10;
  const confidence = sub?.confidence ?? 0;

  return (
    <div className="grid">
      <header className="page-head">
        <div>
          <h2>多模态主观题批改</h2>
          <p className="muted">异步转写 → 学生确认 → 步骤划分 / 首错定位 / 建议分；低置信度自动转入教师复核；改分保留前后记录。</p>
        </div>
        <GuardChips items={["模型失败不丢提交", "置信度 < 0.8 需复核", "改分留痕"]} />
      </header>

      <div className="cols">
        <section className="panel grid">
          <h3>投递批改任务</h3>
          <textarea rows={5} value={draft} onChange={(e) => setDraft(e.target.value)} placeholder="手写作答转写文本，按行分步" />
          <div className="row">
            <button onClick={enqueue} disabled={busy}>{busy ? "处理中…" : "投递批改"}</button>
            <button className="secondary" onClick={() => refreshList().catch((e) => setErr(errMsg(e)))}>刷新列表</button>
          </div>

          <h3>本班提交（{list.length}）</h3>
          {list.length === 0 && <p className="empty-hint">暂无提交，先投递一份批改任务。</p>}
          <ul className="sub-list">
            {list.map((s) => (
              <li key={s.id}>
                <button
                  className={`sub-item secondary ${sub?.id === s.id ? "active" : ""}`}
                  onClick={() => select(s.id)}
                >
                  <span>
                    <b>{s.studentId || s.id}</b>
                    <em>{s.id}</em>
                  </span>
                  <span className="row">
                    <StatusBadge status={s.status} />
                    {s.score != null && <span className="pill">{s.score}/{s.maxScore || max}</span>}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </section>

        {sub ? (
          <section className="panel grid">
            <div className="row spread">
              <h3>批改详情 <StatusBadge status={sub.status} /></h3>
              <span className="muted mono">{sub.id}</span>
            </div>
            <Stepper stage={stage} failed={failed} />
            {failed && <Notice tone="err">模型失败，但提交已保留（不删除），可重新投递批改。</Notice>}
            {toast && <Notice tone="ok">{toast}</Notice>}
            {err && <Notice tone="err">{err}</Notice>}

            <div className="stat-row">
              <div className="stat">
                <span className="muted">建议分</span>
                <b>{sub.suggestedScore ?? "—"}</b>
                <Bar value={sub.suggestedScore ?? 0} max={max} tone="muted" />
              </div>
              <div className="stat">
                <span className="muted">当前得分</span>
                <b>{sub.score ?? "—"} <small>/ {max}</small></b>
                <Bar value={sub.score ?? 0} max={max} tone={sub.status === "teacher_final" ? "good" : "accent"} />
              </div>
              <div className="stat">
                <span className="muted">置信度</span>
                <b>{Math.round(confidence * 100)}% {confidence < 0.8 && <span className="badge todo">偏低</span>}</b>
                <Bar value={confidence} max={1} tone={confidence < 0.8 ? "warn" : "good"} />
              </div>
            </div>

            {(sub.firstErrorStep || sub.errorCause) && (
              <div className="row">
                {sub.firstErrorStep && <span className="badge err">首错：{sub.firstErrorStep}</span>}
                {sub.errorCause && <span className="badge warn">错因：{sub.errorCause}</span>}
                {sub.needsReview && <span className="badge todo">待教师复核</span>}
              </div>
            )}

            <div>
              <h4>转写文本（{sub.transcriptionConfirmed ? "已确认" : "待确认"}）</h4>
              <textarea rows={4} value={editText} onChange={(e) => setEditText(e.target.value)} />
              <div className="row">
                <button onClick={confirmTranscription} disabled={busy || !sub.transcription}>
                  确认转写（学生）
                </button>
                {sub.imageDigest && <span className="muted mono">图片摘要 {sub.imageDigest}</span>}
              </div>
            </div>

            {sub.steps && sub.steps.length > 0 && (
              <div>
                <h4>步骤划分（{sub.steps.filter((s) => s.ok).length}/{sub.steps.length} 正确）</h4>
                <div className="step-cards">
                  {sub.steps.map((st) => (
                    <div key={st.index} className={`step-card ${st.ok ? "ok" : "bad"} ${sub.firstErrorStep === `步骤${st.index}` ? "first" : ""}`}>
                      <span className="mark">{st.ok ? "✓" : "✗"}</span>
                      <div>
                        <b>步骤 {st.index}</b>
                        <p>{st.text}</p>
                        {st.comment && <span className="muted">评语：{st.comment}</span>}
                      </div>
                      {sub.firstErrorStep === `步骤${st.index}` && <span className="badge err">首错</span>}
                    </div>
                  ))}
                </div>
              </div>
            )}

            <div className="override">
              <h4>教师定分（覆盖 AI 建议，保留前后记录）</h4>
              <div className="row">
                <input
                  type="number"
                  style={{ width: 110 }}
                  min={0}
                  max={max}
                  step={0.5}
                  value={finalScore}
                  onChange={(e) => setFinalScore(e.target.value)}
                  placeholder={`0–${max}`}
                />
                <input
                  style={{ flex: 1, minWidth: 180 }}
                  value={finalNote}
                  onChange={(e) => setFinalNote(e.target.value)}
                  placeholder="改分说明（写入审计与历史）"
                />
                <button className="danger" onClick={override} disabled={busy || finalScore === ""}>定分</button>
              </div>
            </div>

            {sub.scoreHistory && sub.scoreHistory.length > 0 && (
              <div>
                <h4>改分历史</h4>
                <ol className="vlist">
                  {sub.scoreHistory.map((h, i) => (
                    <li key={i}>
                      <span className="mono">{h.before ?? "—"} → <b>{h.after}</b></span>
                      <span>{h.note}</span>
                      <span className="muted">{h.teacherId} · {h.at ? new Date(h.at).toLocaleTimeString() : ""}</span>
                    </li>
                  ))}
                </ol>
              </div>
            )}
            {sub.teacherNote && !sub.scoreHistory?.length && <p className="muted">教师备注：{sub.teacherNote}</p>}
            <Raw data={raw} />
          </section>
        ) : (
          <section className="panel">
            <p className="empty-hint">从左侧投递批改任务或选择一份提交查看详情。</p>
          </section>
        )}
      </div>

      <section className="panel grid">
        <h3>客观题规则判分</h3>
        <p className="muted">确定性规则，即时返回，不进入异步队列、不调用大模型。</p>
        <div className="row">
          <select value={objType} onChange={(e) => setObjType(e.target.value)} style={{ width: 120 }}>
            <option value="choice">选择题</option>
            <option value="fill">填空题</option>
          </select>
          <input style={{ width: 140 }} value={objGot} onChange={(e) => setObjGot(e.target.value)} placeholder="学生答案" />
          <input style={{ width: 160 }} value={objWant} onChange={(e) => setObjWant(e.target.value)} placeholder="标准答案（| 分隔多个）" />
          <button className="secondary" onClick={runObjective}>判分</button>
        </div>
        {objResult && (
          <div className={`obj-card ${objResult.correct ? "ok" : "bad"}`}>
            <b>{objResult.correct ? "✓ 正确" : "✗ 错误"}</b>
            <div>
              <Bar value={objResult.score} max={objResult.maxScore} tone={objResult.correct ? "good" : "err"} />
            </div>
            <span>{objResult.score} / {objResult.maxScore} 分</span>
            <span className="pill">{objResult.method}</span>
            <span className="pill">{objResult.usedLLM ? "已用模型" : "未调用模型"}</span>
          </div>
        )}
      </section>
    </div>
  );
}

/* ============================================================
 * 班级讲评
 * ========================================================== */

type Content = {
  id: string;
  skill: string;
  status: string;
  payload: Record<string, any>;
  createdAt?: string;
  updatedAt?: string;
};

type ReviewData = {
  outline?: string[];
  examples?: string[];
  remedial?: string[];
  commonCauses?: string[];
  anonymous?: boolean;
  inputCount?: number;
  anonymousInputs?: string[];
  teacherOnly?: boolean;
};

const CONTENT_STATUS: Record<string, { label: string; tone: string }> = {
  generating: { label: "生成中", tone: "info" },
  pending: { label: "待教师确认", tone: "todo" },
  confirmed: { label: "已确认", tone: "done" },
  published: { label: "已发布（冻结）", tone: "done" },
  rejected: { label: "已驳回", tone: "err" },
};

export function ReviewPage() {
  const [feedbacks, setFeedbacks] = useState(
    "李同学：秩判定弄反了\n王同学：行变换只变了系数矩阵\n张同学：概念混淆\n赵同学：回代计算失误\n钱同学：漏看增广列"
  );
  const [result, setResult] = useState<Record<string, any> | null>(null);
  const [content, setContent] = useState<Content | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  async function loadContent(id: string) {
    setContent(await api<Content>(`/api/v1/contents/${id}`));
  }

  async function generate() {
    setBusy(true);
    setErr("");
    setResult(null);
    setContent(null);
    try {
      const lines = feedbacks.split("\n").map((s) => s.trim()).filter(Boolean);
      const res = await api<Record<string, any>>("/api/v1/skills/class-review", {
        method: "POST",
        body: JSON.stringify({ assignmentId: "asg-1", feedbacks: lines }),
      });
      setResult(res);
      if (res.skipped) return;
      const done = await pollUntil<Content>(
        () => api<Content>(`/api/v1/contents/${res.contentId}`),
        (c) => Boolean(c.payload?.review),
        { attempts: 20 }
      );
      setContent(done ?? null);
    } catch (e) {
      setErr(errMsg(e));
    } finally {
      setBusy(false);
    }
  }

  async function act(path: "confirm" | "publish" | "reject") {
    if (!content) return;
    setBusy(true);
    setErr("");
    try {
      await api(`/api/v1/contents/${content.id}/${path}`, { method: "POST", body: path === "reject" ? JSON.stringify({ reason: "教师驳回" }) : "{}" });
      await loadContent(content.id);
    } catch (e) {
      setErr(errMsg(e));
    } finally {
      setBusy(false);
    }
  }

  const review = (content?.payload?.review || null) as ReviewData | null;
  const skipped = result?.skipped === true;
  const statusMeta = content ? CONTENT_STATUS[content.status] || { label: content.status, tone: "info" } : null;

  return (
    <div className="grid">
      <header className="page-head">
        <div>
          <h2>班级讲评</h2>
          <p className="muted">有效提交 &lt; 5 人不调模型；送模前逐条匿名化；讲评结果仅教师可见，发布后学生才可见全班共性部分。</p>
        </div>
        <GuardChips items={["人数门槛 5", "送模前匿名化", "个人对比仅教师"]} />
      </header>

      <section className="panel grid">
        <label className="muted">
          有效反馈（每行一条，含学生姓名以演示匿名化）
          <textarea rows={5} value={feedbacks} onChange={(e) => setFeedbacks(e.target.value)} />
        </label>
        <div className="row">
          <button onClick={generate} disabled={busy}>生成讲评</button>
          <span className="muted">当前 {feedbacks.split("\n").filter((s) => s.trim()).length} 条</span>
        </div>
        {err && <Notice tone="err">{err}</Notice>}
      </section>

      {skipped && result && (
        <section className="panel grid">
          <Notice tone="warn">{result.reason}（{result.count} / {result.threshold}）—— 未调用模型。</Notice>
          <Bar value={Math.min(result.count, result.threshold)} max={result.threshold} tone="warn" />
        </section>
      )}

      {content && (
        <section className="panel grid">
          <div className="row spread">
            <h3>
              讲评内容 {statusMeta && <span className={`badge ${statusMeta.tone}`}>{statusMeta.label}</span>}
            </h3>
            <span className="muted mono">{content.id}</span>
          </div>

          {result?.anonymized && (
            <div className="row">
              <span className="badge done">已匿名化</span>
              <span className="badge info">样本 {result.sampleCount} 人</span>
              <span className="badge todo">教师可见</span>
            </div>
          )}

          {!review && <p className="empty-hint">讲评生成中… Worker 处理完成后自动渲染。</p>}

          {review && (
            <>
              <div className="grid two">
                <div className="card-list">
                  <h4>讲评大纲</h4>
                  <ol className="vlist numbered">
                    {(review.outline || []).map((o) => <li key={o}>{o}</li>)}
                  </ol>
                </div>
                <div className="card-list">
                  <h4>共性错因</h4>
                  <div className="row wrap">
                    {(review.commonCauses || []).map((c) => <span key={c} className="chip warn">{c}</span>)}
                  </div>
                  <h4 style={{ marginTop: 12 }}>对比演示</h4>
                  <ul className="plain">
                    {(review.examples || []).map((ex) => <li key={ex}>{ex}</li>)}
                  </ul>
                  <h4 style={{ marginTop: 12 }}>补救建议</h4>
                  <ul className="plain">
                    {(review.remedial || []).map((r) => <li key={r}>{r}</li>)}
                  </ul>
                </div>
              </div>

              <div>
                <h4>送模输入（已匿名，姓名/称谓已剥离）</h4>
                <ul className="anon-list">
                  {(review.anonymousInputs || (content.payload?.anonymousInputs as string[] | undefined) || []).map((a, i) => (
                    <li key={i}><span className="muted mono">#{i + 1}</span> {a}</li>
                  ))}
                </ul>
              </div>

              <div className="row">
                <button className="secondary" onClick={() => act("confirm")} disabled={busy || content.status === "published"}>教师确认</button>
                <button onClick={() => act("publish")} disabled={busy || content.status !== "confirmed"}>发布给学生</button>
                <button className="danger" onClick={() => act("reject")} disabled={busy || content.status === "published"}>驳回</button>
                {content.status === "published" && <span className="badge done">已发布并冻结，不可再编辑</span>}
              </div>
            </>
          )}
          <Raw data={result} />
        </section>
      )}
    </div>
  );
}

/* ============================================================
 * 学习诊断
 * ========================================================== */

type DiagEvidence = { id: string; kind: string; summary: string; kp?: string; at?: string };
type Diagnosis = {
  studentId?: string;
  mastery?: { kp: string; level: string; evidence?: string[] }[];
  trend?: string;
  evidenceEvents?: DiagEvidence[];
  studentVisible?: boolean;
  requiresConfirm?: boolean;
  message?: string;
  sampleCount?: number;
  threshold?: number;
};

const EVENT_KIND: Record<string, string> = { assignment: "作业", practice: "练习", help: "求助", tutoring: "辅导" };
const LEVEL_META: Record<string, { label: string; tone: string; pct: number }> = {
  weak: { label: "薄弱", tone: "err", pct: 30 },
  developing: { label: "发展中", tone: "warn", pct: 60 },
  strong: { label: "已掌握", tone: "good", pct: 90 },
};

export function DiagnosisPage() {
  const [studentId, setStudentId] = useState("stu-1");
  const [result, setResult] = useState<Record<string, any> | null>(null);
  const [content, setContent] = useState<Content | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [released, setReleased] = useState(false);

  async function generate() {
    setBusy(true);
    setErr("");
    setResult(null);
    setContent(null);
    setReleased(false);
    try {
      const res = await api<Record<string, any>>("/api/v1/skills/diagnosis", {
        method: "POST",
        body: JSON.stringify({ studentId }),
      });
      setResult(res);
      if (res.intercepted) return;
      const done = await pollUntil<Content>(
        () => api<Content>(`/api/v1/contents/${res.contentId}`),
        (c) => (c.payload?.diagnosis as Diagnosis | undefined)?.mastery !== undefined,
        { attempts: 20 }
      );
      setContent(done ?? null);
    } catch (e) {
      setErr(errMsg(e));
    } finally {
      setBusy(false);
    }
  }

  async function release() {
    if (!content) return;
    setBusy(true);
    setErr("");
    try {
      const res = await api<{ studentVisible: boolean; status: string }>(`/api/v1/skills/diagnosis/${content.id}/release`, {
        method: "POST",
        body: "{}",
      });
      setReleased(res.studentVisible === true);
      setContent(await api<Content>(`/api/v1/contents/${content.id}`));
    } catch (e) {
      setErr(errMsg(e));
    } finally {
      setBusy(false);
    }
  }

  const diag = (result?.diagnosis as Diagnosis | undefined) ?? (content?.payload?.diagnosis as Diagnosis | undefined);
  const intercepted = result?.intercepted === true;
  const visible = released || content?.payload?.studentVisible === true || diag?.studentVisible === true;

  return (
    <div className="grid">
      <header className="page-head">
        <div>
          <h2>学习诊断</h2>
          <p className="muted">作答事件 &lt; 3 条只显示「样本不足」；每项结论绑定证据事件；教师确认前对学生不可见。</p>
        </div>
        <GuardChips items={["样本门槛 3", "结论绑定证据", "教师放行才对可见"]} />
      </header>

      <section className="panel row">
        <label className="muted" style={{ minWidth: 180 }}>
          学生 ID
          <input value={studentId} onChange={(e) => setStudentId(e.target.value)} list="stu-ids" />
          <datalist id="stu-ids">
            <option value="stu-1" />
            <option value="stu-2" />
            <option value="stu-unknown" />
          </datalist>
        </label>
        <button onClick={generate} disabled={busy} style={{ alignSelf: "flex-end" }}>{busy ? "生成中…" : "生成诊断"}</button>
        <span className="muted" style={{ alignSelf: "flex-end" }}>stu-unknown 演示样本不足拦截</span>
        {err && <p className="error">{err}</p>}
      </section>

      {intercepted && diag && (
        <section className="panel grid">
          <Notice tone="warn">样本不足：仅 {diag.sampleCount ?? result?.diagnosis?.sampleCount ?? 0} 条事件（门槛 {diag.threshold ?? 3} 条），不生成诊断结论。</Notice>
          <Bar value={Math.min(diag.sampleCount ?? 0, diag.threshold ?? 3)} max={diag.threshold ?? 3} tone="warn" />
        </section>
      )}

      {content && diag?.mastery && (
        <section className="panel grid">
          <div className="row spread">
            <h3>
              {diag.studentId || studentId} 的诊断报告
              <span className={`badge ${visible ? "done" : "todo"}`} style={{ marginLeft: 8 }}>
                {visible ? "已对学生开放" : "仅教师可见"}
              </span>
            </h3>
            <span className="muted mono">{content.id}</span>
          </div>
          {diag.trend && <div className="trend">趋势：{diag.trend}</div>}

          <div className="mastery-grid">
            {diag.mastery.map((m) => {
              const lv = LEVEL_META[m.level] || { label: m.level, tone: "info", pct: 50 };
              return (
                <div key={m.kp} className="mastery-card">
                  <div className="row spread">
                    <b>{m.kp}</b>
                    <span className={`badge ${lv.tone}`}>{lv.label}</span>
                  </div>
                  <Bar value={lv.pct} max={100} tone={lv.tone} />
                  <div className="row wrap">
                    <span className="muted">证据：</span>
                    {(m.evidence || []).map((id) => (
                      <span key={id} className="chip mono">{id}</span>
                    ))}
                  </div>
                </div>
              );
            })}
          </div>

          <div>
            <h4>证据事件（{diag.evidenceEvents?.length ?? 0}）</h4>
            <ol className="timeline">
              {(diag.evidenceEvents || []).map((ev) => (
                <li key={ev.id}>
                  <span className={`badge ${EVENT_KIND[ev.kind] ? "info" : "muted"}`}>{EVENT_KIND[ev.kind] || ev.kind}</span>
                  <div>
                    {ev.summary}
                    <div className="muted mono">{ev.id}{ev.kp ? ` · ${ev.kp}` : ""}{ev.at ? ` · ${new Date(ev.at).toLocaleString()}` : ""}</div>
                  </div>
                </li>
              ))}
            </ol>
          </div>

          <div className="row">
            <button onClick={release} disabled={busy || visible}>{visible ? "已开放给学生" : "教师确认并向学生开放"}</button>
            {visible && <span className="badge done">个人报告已对学生可见</span>}
          </div>
          <Raw data={result} />
        </section>
      )}
    </div>
  );
}

/* ============================================================
 * 作答回放与原创性审查
 * ========================================================== */

type Snapshot = { version: number; content: string; createdAt: string; trigger: string };
type Timeline = { attemptId: string; snapshots: Snapshot[]; citations?: string[]; aiUsageNote?: string; policy?: string };
type SimItem = { fragment: string; source: string; similarity: number; location: string };
type SimReport = { attemptId: string; evidence: SimItem[]; note: string; verdict: unknown };

export function ReplayPage() {
  const [attemptId, setAttemptId] = useState("");
  const [content, setContent] = useState("秩等于增广矩阵秩时有解。高斯消元法通过初等行变换求阶梯型。");
  const [timeline, setTimeline] = useState<Timeline | null>(null);
  const [sim, setSim] = useState<SimReport | null>(null);
  const [err, setErr] = useState("");
  const [toast, setToast] = useState("");
  const [cites, setCites] = useState("讲义§3, 讲义§2");
  const [aiNote, setAiNote] = useState("使用过语法检查，未生成解题步骤");
  const [busy, setBusy] = useState(false);

  async function snapshot(trigger: "manual" | "interval" | "keydown") {
    setBusy(true);
    setErr("");
    setToast("");
    try {
      const res = await api<{ attemptId: string; version: number; trigger: string; keylogging: boolean }>(
        "/api/v1/skills/answer-replay/snapshot",
        { method: "POST", body: JSON.stringify({ attemptId: attemptId || undefined, content, trigger }) }
      );
      setAttemptId(res.attemptId);
      setToast(trigger === "keydown" ? "意外成功（不应发生）" : `已保存版本 v${res.version}（${trigger === "manual" ? "主动保存" : "定时快照"}）`);
      setTimeline(await api<Timeline>(`/api/v1/skills/answer-replay/${res.attemptId}/timeline`));
    } catch (e) {
      if (trigger === "keydown") {
        setErr(`已按策略拒绝逐键采集：${errMsg(e)}`);
      } else {
        setErr(errMsg(e));
      }
    } finally {
      setBusy(false);
    }
  }

  async function saveMeta() {
    setBusy(true);
    setErr("");
    try {
      await api("/api/v1/skills/answer-replay/meta", {
        method: "POST",
        body: JSON.stringify({ attemptId, citations: cites.split(",").map((s) => s.trim()).filter(Boolean), aiUsageNote: aiNote }),
      });
      setToast("引用与 AI 使用说明已补充");
      setTimeline(await api<Timeline>(`/api/v1/skills/answer-replay/${attemptId}/timeline`));
    } catch (e) {
      setErr(errMsg(e));
    } finally {
      setBusy(false);
    }
  }

  async function loadSim() {
    setBusy(true);
    setErr("");
    try {
      setSim(await api<SimReport>(`/api/v1/skills/answer-replay/${attemptId}/similarity`));
    } catch (e) {
      setErr(errMsg(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="grid">
      <header className="page-head">
        <div>
          <h2>作答过程回放与原创性审查</h2>
          <p className="muted">仅保存定时 / 主动快照，绝不记录键盘行为；相似片段并排呈现证据，由教师判断，AI 不自动生成违规结论。</p>
        </div>
        <GuardChips items={["仅 interval / manual", "逐键采集被拒绝", "不自动判定违规"]} />
      </header>

      <div className="cols">
        <section className="panel grid">
          <h3>作答编辑区</h3>
          <textarea rows={6} value={content} onChange={(e) => setContent(e.target.value)} />
          <div className="row">
            <button onClick={() => snapshot("manual")} disabled={busy}>主动保存版本</button>
            <button className="secondary" onClick={() => snapshot("interval")} disabled={busy}>模拟定时快照</button>
            <button className="danger" onClick={() => snapshot("keydown")} disabled={busy}>尝试逐键采集（应被拒绝）</button>
          </div>
          {attemptId && <span className="pill mono">attempt: {attemptId}</span>}
          {err && <Notice tone="err">{err}</Notice>}
          {toast && <Notice tone="ok">{toast}</Notice>}

          <div>
            <h4>引用与 AI 使用说明</h4>
            <label className="muted">引用（逗号分隔）
              <input value={cites} onChange={(e) => setCites(e.target.value)} />
            </label>
            <label className="muted" style={{ marginTop: 8 }}>AI 工具使用说明
              <input value={aiNote} onChange={(e) => setAiNote(e.target.value)} />
            </label>
            <div className="row" style={{ marginTop: 8 }}>
              <button className="secondary" onClick={saveMeta} disabled={busy || !attemptId}>补充声明</button>
            </div>
          </div>
        </section>

        <section className="panel grid">
          <h3>版本时间线</h3>
          {!timeline && <p className="empty-hint">保存一次快照后自动显示时间线。</p>}
          {timeline && (
            <>
              <ol className="timeline">
                {timeline.snapshots.map((s) => (
                  <li key={s.version}>
                    <span className={`badge ${s.trigger === "manual" ? "info" : "muted"}`}>v{s.version} · {s.trigger === "manual" ? "主动" : "定时"}</span>
                    <div>
                      <p className="snap-text">{s.content}</p>
                      <span className="muted mono">{new Date(s.createdAt).toLocaleTimeString()}</span>
                    </div>
                  </li>
                ))}
              </ol>
              {timeline.policy && <span className="pill">{timeline.policy}</span>}
              {(timeline.citations?.length || timeline.aiUsageNote) && (
                <div className="row wrap">
                  {(timeline.citations || []).map((c) => <span key={c} className="chip">{c}</span>)}
                  {timeline.aiUsageNote && <span className="chip warn">AI：{timeline.aiUsageNote}</span>}
                </div>
              )}
            </>
          )}
        </section>
      </div>

      <section className="panel grid">
        <div className="row spread">
          <h3>原创性审查证据</h3>
          <button onClick={loadSim} disabled={busy || !attemptId}>拉取相似证据</button>
        </div>
        {!sim && <p className="empty-hint">先创建作答，再拉取相似片段对照。</p>}
        {sim && (
          <>
            <Notice tone="info">{sim.note}（结论字段恒为空：不自动判定违规）</Notice>
            {sim.evidence.map((it, i) => (
              <div key={i} className="sim-card">
                <div className="row spread">
                  <span className="muted">来源：{it.source}</span>
                  <span className={`badge ${it.similarity >= 0.8 ? "err" : it.similarity >= 0.6 ? "warn" : "info"}`}>
                    相似度 {Math.round(it.similarity * 100)}%
                  </span>
                </div>
                <Bar value={it.similarity} max={1} tone={it.similarity >= 0.8 ? "err" : "warn"} />
                <div className="frag">{it.fragment}</div>
                <span className="muted">{it.location}</span>
              </div>
            ))}
          </>
        )}
      </section>
    </div>
  );
}

import { FormEvent, useEffect, useMemo, useState } from "react";
import { api, classApiPath, getToken } from "../../api";

type Citation = {
  id: string;
  source: string;
  text: string;
  contentHash: string;
};

type Message = {
  id: string;
  role: "user" | "assistant";
  content: string;
  citations?: Citation[];
};

type Conversation = {
  id: string;
  title: string;
  messages: Message[];
  updatedAt: string;
};

function errorText(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}

async function streamMessage(
  conversationId: string,
  content: string,
  onDelta: (text: string) => void,
  onCitations: (citations: Citation[]) => void,
) {
  const response = await fetch(`/api/v1/conversations/${conversationId}/messages`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${getToken()}`,
    },
    body: JSON.stringify({ content }),
  });
  if (!response.ok) {
    const payload = await response.json().catch(() => ({})) as { message?: string };
    throw new Error(payload.message || response.statusText);
  }
  if (!response.body) throw new Error("浏览器不支持流式响应");

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  for (;;) {
    const { done, value } = await reader.read();
    buffer += decoder.decode(value, { stream: !done });
    const blocks = buffer.split("\n\n");
    buffer = done ? "" : blocks.pop() || "";
    for (const block of blocks) {
      let event = "";
      let data = "";
      for (const line of block.split("\n")) {
        if (line.startsWith("event:")) event = line.slice(6).trim();
        if (line.startsWith("data:")) data += line.slice(5).trim();
      }
      if (!data) continue;
      const payload = JSON.parse(data) as { text?: string } | Citation[];
      if (event === "delta" && !Array.isArray(payload)) onDelta(payload.text || "");
      if (event === "citations" && Array.isArray(payload)) onCitations(payload);
    }
    if (done) break;
  }
}

export function AssistantPage() {
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [activeId, setActiveId] = useState("");
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState("高斯消元法的基本步骤是什么？");
  const [error, setError] = useState("");
  const [sending, setSending] = useState(false);

  async function refreshList() {
    const result = await api<Conversation[]>("/api/v1/conversations");
    setConversations(result);
    return result;
  }

  async function open(id: string) {
    setActiveId(id);
    const conversation = await api<Conversation>(`/api/v1/conversations/${id}`);
    setMessages(conversation.messages || []);
  }

  useEffect(() => {
    refreshList()
      .then((items) => {
        if (items[0]) void open(items[0].id);
      })
      .catch((value) => setError(errorText(value)));
  }, []);

  async function createConversation() {
    setError("");
    try {
      const conversation = await api<Conversation>(classApiPath("/conversations"), {
        method: "POST",
        body: JSON.stringify({ title: "课程问答" }),
      });
      setConversations((current) => [conversation, ...current]);
      setActiveId(conversation.id);
      setMessages([]);
    } catch (value) {
      setError(errorText(value));
    }
  }

  async function send(event: FormEvent) {
    event.preventDefault();
    const question = input.trim();
    if (!activeId || !question || sending) return;
    setError("");
    setSending(true);
    setInput("");
    const localUser: Message = { id: `local-user-${Date.now()}`, role: "user", content: question };
    const localAssistant: Message = { id: `local-ai-${Date.now()}`, role: "assistant", content: "", citations: [] };
    setMessages((current) => [...current, localUser, localAssistant]);
    try {
      await streamMessage(
        activeId,
        question,
        (delta) => setMessages((current) => current.map((message) => message.id === localAssistant.id ? { ...message, content: message.content + delta } : message)),
        (citations) => setMessages((current) => current.map((message) => message.id === localAssistant.id ? { ...message, citations } : message)),
      );
      await refreshList();
    } catch (value) {
      setError(errorText(value));
      setMessages((current) => current.filter((message) => message.id !== localAssistant.id));
    } finally {
      setSending(false);
    }
  }

  return (
    <div className="grid">
      <header className="page-head">
        <div>
          <h2>班级学习助手</h2>
          <p className="muted">回答以本班讲义检索结果为依据，并随答案显示可追溯引用。</p>
        </div>
        <button onClick={createConversation}>新建会话</button>
      </header>
      {error && <p className="notice err">{error}</p>}
      <div className="cols">
        <section className="panel grid">
          <h3>我的会话</h3>
          <div className="sub-list">
            {conversations.map((conversation) => (
              <button
                className={`sub-item secondary ${conversation.id === activeId ? "active" : ""}`}
                key={conversation.id}
                onClick={() => open(conversation.id).catch((value) => setError(errorText(value)))}
              >
                <span><b>{conversation.title}</b><em>{conversation.id}</em></span>
              </button>
            ))}
          </div>
        </section>
        <section className="panel grid">
          {!activeId ? <p className="empty-hint">新建或选择一个会话。</p> : (
            <>
              <div className="chat-log">
                {messages.length === 0 && <p className="empty-hint">输入课程问题开始问答。</p>}
                {messages.map((message) => (
                  <article className={`chat-message ${message.role}`} key={message.id}>
                    <b>{message.role === "user" ? "我" : "班级助手"}</b>
                    <p>{message.content || (sending ? "正在生成…" : "")}</p>
                    {message.citations && message.citations.length > 0 && (
                      <details>
                        <summary>查看 {message.citations.length} 条讲义引用</summary>
                        <ul className="vlist">
                          {message.citations.map((citation) => (
                            <li key={citation.id}>
                              <span>{citation.source}</span>
                              <span>{citation.text}</span>
                              <span className="mono">{citation.contentHash.slice(0, 8)}</span>
                            </li>
                          ))}
                        </ul>
                      </details>
                    )}
                  </article>
                ))}
              </div>
              <form className="grid" onSubmit={send}>
                <textarea rows={3} value={input} onChange={(event) => setInput(event.target.value)} placeholder="输入课程问题…" />
                <button type="submit" disabled={sending || !input.trim()}>{sending ? "回答中…" : "发送问题"}</button>
              </form>
            </>
          )}
        </section>
      </div>
    </div>
  );
}

type Assignment = {
  id: string;
  title: string;
  status: string;
  items: Array<{ id: string; stem: string; type: string }>;
};

type Submission = {
  id: string;
  status: string;
  score?: number;
  suggestedScore?: number;
  maxScore?: number;
  teacherNote?: string;
  needsReview?: boolean;
};

type Mistake = {
  questionStem: string;
  errorCause: string;
  hint: string;
  answerRevealed: boolean;
};

export function AssignmentsPage() {
  const [assignments, setAssignments] = useState<Assignment[]>([]);
  const [selectedId, setSelectedId] = useState("");
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const [taskStatus, setTaskStatus] = useState("");
  const [submission, setSubmission] = useState<Submission | null>(null);
  const [mistake, setMistake] = useState<Mistake | null>(null);
  const [error, setError] = useState("");
  const selected = useMemo(() => assignments.find((assignment) => assignment.id === selectedId), [assignments, selectedId]);

  useEffect(() => {
    api<Assignment[]>(classApiPath("/assignments"))
      .then((items) => {
        setAssignments(items);
        if (items[0]) setSelectedId(items[0].id);
      })
      .catch((value) => setError(errorText(value)));
  }, []);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!selected) return;
    setError("");
    setSubmission(null);
    setMistake(null);
    try {
      const accepted = await api<{ taskId: string; submissionId: string }>(`/api/v1/assignments/${selected.id}/submissions`, {
        method: "POST",
        headers: { "Idempotency-Key": crypto.randomUUID() },
        body: JSON.stringify({ answers }),
      });
      setTaskStatus("pending");
      let status = "pending";
      for (let attempt = 0; attempt < 30 && !["succeeded", "failed"].includes(status); attempt += 1) {
        await new Promise((resolve) => window.setTimeout(resolve, 200));
        const task = await api<{ status: string }>(`/api/v1/tasks/${accepted.taskId}`);
        status = task.status;
        setTaskStatus(status);
      }
      if (status !== "succeeded") throw new Error(status === "failed" ? "批改任务失败，提交仍已保存" : "批改仍在进行，请稍后重试");
      const result = await api<Submission>(`/api/v1/submissions/${accepted.submissionId}`);
      setSubmission(result);
      if (result.status === "graded" && result.score != null && result.score < (result.maxScore ?? 10)) {
        const explanation = await api<Mistake>(`/api/v1/submissions/${accepted.submissionId}/mistake-explanation`);
        setMistake(explanation);
      }
    } catch (value) {
      setError(errorText(value));
    }
  }

  return (
    <div className="grid">
      <header className="page-head">
        <div>
          <h2>作业提交与批改</h2>
          <p className="muted">提交后返回任务编号；异步批改、教师改分和错题提示均可追溯。</p>
        </div>
      </header>
      {error && <p className="notice err">{error}</p>}
      <div className="cols">
        <section className="panel grid">
          <h3>已发布作业</h3>
          <div className="sub-list">
            {assignments.map((assignment) => (
              <button
                className={`sub-item secondary ${assignment.id === selectedId ? "active" : ""}`}
                key={assignment.id}
                onClick={() => {
                  setSelectedId(assignment.id);
                  setAnswers({});
                  setSubmission(null);
                  setMistake(null);
                }}
              >
                <span><b>{assignment.title}</b><em>{assignment.id}</em></span>
                <span className="badge done">已发布</span>
              </button>
            ))}
          </div>
        </section>
        <form className="panel grid" onSubmit={submit}>
          {!selected ? <p className="empty-hint">暂无已发布作业。</p> : (
            <>
              <h3>{selected.title}</h3>
              {selected.items.map((item, index) => (
                <label className="muted" key={item.id}>
                  {index + 1}. {item.stem}
                  <textarea
                    rows={3}
                    value={answers[item.id] || ""}
                    onChange={(event) => setAnswers((current) => ({ ...current, [item.id]: event.target.value }))}
                  />
                </label>
              ))}
              <button type="submit">提交并开始批改</button>
            </>
          )}
          {taskStatus && <p className="notice info">任务状态：{taskStatus}</p>}
          {submission && (
            <section className="notice ok">
              {submission.needsReview ? (
                <>主观题待教师复核；AI 建议分：<b>{submission.suggestedScore ?? "无"} / {submission.maxScore ?? 10}</b></>
              ) : (
                <>最终得分：<b>{submission.score ?? "待定"} / {submission.maxScore ?? 10}</b></>
              )}
              {submission.teacherNote && <span> · 教师说明：{submission.teacherNote}</span>}
            </section>
          )}
          {mistake && (
            <section className="notice warn">
              <b>错因：</b>{mistake.errorCause}<br />
              <b>提示：</b>{mistake.hint}<br />
              <small>完整答案未直接展示，请根据提示重新思考。</small>
            </section>
          )}
        </form>
      </div>
    </div>
  );
}

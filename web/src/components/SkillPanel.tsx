import { useEffect, useState } from "react";
import { api } from "../api";

type Props = {
  title: string;
  description: string;
  actionLabel?: string;
  onRun: () => Promise<unknown>;
  children?: React.ReactNode;
};

export function SkillPanel({ title, description, actionLabel = "运行", onRun, children }: Props) {
  const [result, setResult] = useState<unknown>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function run() {
    setLoading(true);
    setError("");
    try {
      const data = await onRun();
      setResult(data);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }

  return (
    <section className="panel grid">
      <div>
        <h2>{title}</h2>
        <p className="muted">{description}</p>
      </div>
      {children}
      <div className="row">
        <button onClick={run} disabled={loading}>{loading ? "处理中…" : actionLabel}</button>
      </div>
      {error && <p className="error">{error}</p>}
      {result != null && <pre className="json">{JSON.stringify(result, null, 2)}</pre>}
    </section>
  );
}

export function ConfirmPublish({ contentId }: { contentId?: string }) {
  const [msg, setMsg] = useState("");
  const [error, setError] = useState("");
  const [payloadText, setPayloadText] = useState("");
  const [production, setProduction] = useState(false);
  const [revisionReason, setRevisionReason] = useState("请按课堂目标修订后重新提交审核");

  useEffect(() => {
    let active = true;
    setMsg("");
    setError("");
    setPayloadText("");
    setProduction(false);
    if (!contentId) return () => { active = false; };
    api<Record<string, unknown>>(`/api/v1/contents/${contentId}`)
      .then((record) => {
        if (!active) return;
        const editable = typeof record.kind === "string" && record.payload != null;
        setProduction(editable);
        if (editable) setPayloadText(JSON.stringify(record.payload, null, 2));
      })
      .catch((value) => {
        if (active) setError(value instanceof Error ? value.message : String(value));
      });
    return () => { active = false; };
  }, [contentId]);

  if (!contentId) return null;

  async function confirm() {
    setError("");
    try {
      let body = "{}";
      if (production) {
        const payload = JSON.parse(payloadText) as unknown;
        if (payload == null || Array.isArray(payload) || typeof payload !== "object") {
          throw new Error("审核内容必须是 JSON 对象");
        }
        body = JSON.stringify({ payload });
      }
      const record = await api<Record<string, unknown>>(`/api/v1/contents/${contentId}/confirm`, { method: "POST", body });
      if (production && record.payload) setPayloadText(JSON.stringify(record.payload, null, 2));
      setMsg("已确认");
    } catch (value) {
      setError(value instanceof Error ? value.message : String(value));
    }
  }

  return (
    <div className="grid">
      <span className="pill">{contentId}</span>
      {production && (
        <>
          <label className="muted">审核并修改生成内容（JSON）
            <textarea rows={10} value={payloadText} onChange={(event) => setPayloadText(event.target.value)} />
          </label>
          <label className="muted">退回修订意见
            <input value={revisionReason} onChange={(event) => setRevisionReason(event.target.value)} />
          </label>
        </>
      )}
      <div className="row">
        <button className="secondary" onClick={confirm}>教师确认</button>
        {production && (
          <button
            className="danger"
            onClick={async () => {
              setError("");
              try {
                await api(`/api/v1/contents/${contentId}/reject`, {
                  method: "POST",
                  body: JSON.stringify({ reason: revisionReason }),
                });
                setMsg("已退回修订");
              } catch (value) {
                setError(value instanceof Error ? value.message : String(value));
              }
            }}
          >
            退回修订
          </button>
        )}
        <button
          onClick={async () => {
            setError("");
            try {
              await api(`/api/v1/contents/${contentId}/publish`, { method: "POST", body: "{}" });
              setMsg("已发布并冻结");
            } catch (value) {
              setError(value instanceof Error ? value.message : String(value));
            }
          }}
        >
          发布
        </button>
      </div>
      {msg && <span className="muted">{msg}</span>}
      {error && <span className="error">{error}</span>}
    </div>
  );
}

import { useState } from "react";
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
  if (!contentId) return null;
  return (
    <div className="row">
      <span className="pill">{contentId}</span>
      <button
        className="secondary"
        onClick={async () => {
          await api(`/api/v1/contents/${contentId}/confirm`, { method: "POST", body: "{}" });
          setMsg("已确认");
        }}
      >
        教师确认
      </button>
      <button
        onClick={async () => {
          await api(`/api/v1/contents/${contentId}/publish`, { method: "POST", body: "{}" });
          setMsg("已发布并冻结");
        }}
      >
        发布
      </button>
      {msg && <span className="muted">{msg}</span>}
    </div>
  );
}

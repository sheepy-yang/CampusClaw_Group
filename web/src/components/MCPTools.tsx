import { useEffect, useState } from "react";
import { api } from "../api";

type Tool = {
  name: string;
  description: string;
  inputSchema: Record<string, unknown>;
};

export function MCPToolsPage() {
  const [tools, setTools] = useState<Tool[]>([]);
  const [selected, setSelected] = useState("");
  const [argumentsText, setArgumentsText] = useState("{}");
  const [result, setResult] = useState<unknown>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api<{ tools: Tool[] }>("/api/v1/mcp/tools")
      .then(({ tools: available }) => {
        setTools(available);
        setSelected(available[0]?.name || "");
      })
      .catch((value) => setError(value instanceof Error ? value.message : String(value)));
  }, []);

  const current = tools.find((tool) => tool.name === selected);

  async function callTool() {
    setBusy(true);
    setError("");
    try {
      const toolArguments = JSON.parse(argumentsText) as unknown;
      if (toolArguments == null || Array.isArray(toolArguments) || typeof toolArguments !== "object") {
        throw new Error("工具参数必须是 JSON 对象");
      }
      setResult(await api(`/api/v1/mcp/tools/${encodeURIComponent(selected)}/call`, {
        method: "POST",
        body: JSON.stringify({ arguments: toolArguments }),
      }));
    } catch (value) {
      setError(value instanceof Error ? value.message : String(value));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="panel grid">
      <header className="page-head">
        <div>
          <h2>MCP 教学工具</h2>
          <p className="muted">工具列表由服务端按当前角色过滤；每次调用都会经过参数校验、超时控制和审计。</p>
        </div>
      </header>
      <label className="muted">可用工具
        <select value={selected} onChange={(event) => {
          setSelected(event.target.value);
          setResult(null);
        }}>
          {tools.map((tool) => <option key={tool.name} value={tool.name}>{tool.name}</option>)}
        </select>
      </label>
      {current && (
        <>
          <p>{current.description}</p>
          <details>
            <summary>查看参数 Schema</summary>
            <pre className="json">{JSON.stringify(current.inputSchema, null, 2)}</pre>
          </details>
          <label className="muted">调用参数（JSON）
            <textarea rows={8} value={argumentsText} onChange={(event) => setArgumentsText(event.target.value)} />
          </label>
          <button onClick={callTool} disabled={busy || !selected}>{busy ? "调用中…" : "调用工具"}</button>
        </>
      )}
      {!current && !error && <p className="empty-hint">当前角色没有可调用工具。</p>}
      {error && <p className="notice err">{error}</p>}
      {result != null && <pre className="json">{JSON.stringify(result, null, 2)}</pre>}
    </section>
  );
}

import { FormEvent, useEffect, useState } from "react";
import { api, classApiPath } from "../../api";

type Material = {
  id: string;
  title: string;
  version: string;
  contentHash: string;
  createdAt: string;
};

type SearchHit = {
  id: string;
  materialId: string;
  materialVersion: string;
  source: string;
  text: string;
  contentHash: string;
  fusionScore: number;
};

function errorText(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}

export function MaterialsPage() {
  const [materials, setMaterials] = useState<Material[]>([]);
  const [title, setTitle] = useState("数据结构讲义");
  const [content, setContent] = useState("栈是后进先出的线性表。队列是先进先出的线性表。");
  const [query, setQuery] = useState("高斯消元");
  const [hits, setHits] = useState<SearchHit[]>([]);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");

  async function refresh() {
    setMaterials(await api<Material[]>(classApiPath("/materials")));
  }

  useEffect(() => {
    refresh().catch((value) => setError(errorText(value)));
  }, []);

  async function ingest(event: FormEvent) {
    event.preventDefault();
    setError("");
    try {
      const material = await api<Material>(classApiPath("/materials"), {
        method: "POST",
        body: JSON.stringify({ title, content }),
      });
      setNotice(`已入库 ${material.title}（${material.version}）`);
      await refresh();
    } catch (value) {
      setError(errorText(value));
    }
  }

  async function search(event: FormEvent) {
    event.preventDefault();
    setError("");
    try {
      const result = await api<{ hits: SearchHit[] }>(classApiPath("/search"), {
        method: "POST",
        body: JSON.stringify({ query, limit: 5 }),
      });
      setHits(result.hits || []);
    } catch (value) {
      setError(errorText(value));
    }
  }

  return (
    <div className="grid">
      <header className="page-head">
        <div>
          <h2>讲义入库与检索</h2>
          <p className="muted">文本切片、向量检索、班级隔离与引用溯源。</p>
        </div>
      </header>
      {notice && <p className="notice ok">{notice}</p>}
      {error && <p className="notice err">{error}</p>}
      <div className="cols">
        <form className="panel grid" onSubmit={ingest}>
          <h3>入库讲义</h3>
          <label className="muted">标题<input value={title} onChange={(event) => setTitle(event.target.value)} /></label>
          <label className="muted">正文<textarea rows={10} value={content} onChange={(event) => setContent(event.target.value)} /></label>
          <button type="submit">切片并入库</button>
        </form>
        <section className="panel grid">
          <h3>本班资料</h3>
          <ul className="vlist">
            {materials.map((material) => (
              <li key={material.id}>
                <span className="mono">{material.version}</span>
                <span>{material.title}</span>
                <span className="badge done">{material.id}</span>
              </li>
            ))}
          </ul>
          <form className="grid" onSubmit={search}>
            <label className="muted">检索问题<input value={query} onChange={(event) => setQuery(event.target.value)} /></label>
            <button type="submit">检索本班讲义</button>
          </form>
          {hits.length === 0 ? <p className="muted">尚无检索结果。</p> : (
            <div className="step-cards">
              {hits.map((hit) => (
                <article className="step-card ok" key={hit.id}>
                  <span className="mark">✓</span>
                  <div>
                    <b>{hit.source}</b>
                    <p>{hit.text}</p>
                    <small className="muted mono">{hit.materialVersion} · {hit.contentHash.slice(0, 12)} · {hit.fusionScore.toFixed(3)}</small>
                  </div>
                </article>
              ))}
            </div>
          )}
        </section>
      </div>
    </div>
  );
}

type AssistantConfig = {
  id: string;
  name: string;
  systemPrompt: string;
  enabledSkills: string[];
};

export function AssistantConfigPage() {
  const [name, setName] = useState("");
  const [prompt, setPrompt] = useState("");
  const [skills, setSkills] = useState<string[]>([]);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    api<AssistantConfig>(classApiPath("/assistant"))
      .then((assistant) => {
        setName(assistant.name);
        setPrompt(assistant.systemPrompt);
        setSkills(assistant.enabledSkills || []);
      })
      .catch((value) => setError(errorText(value)));
  }, []);

  async function save(event: FormEvent) {
    event.preventDefault();
    setError("");
    try {
      await api(classApiPath("/assistant"), {
        method: "PUT",
        body: JSON.stringify({ name, systemPrompt: prompt, enabledSkills: skills }),
      });
      setNotice("助手配置已保存，后续会话立即使用新配置。");
    } catch (value) {
      setError(errorText(value));
    }
  }

  return (
    <form className="panel grid" onSubmit={save}>
      <div>
        <h2>班级助手配置</h2>
        <p className="muted">提示词与技能由教师管理；模型只能读取本班检索结果。</p>
      </div>
      {notice && <p className="notice ok">{notice}</p>}
      {error && <p className="notice err">{error}</p>}
      <label className="muted">助手名称<input value={name} onChange={(event) => setName(event.target.value)} /></label>
      <label className="muted">系统提示词<textarea rows={8} value={prompt} onChange={(event) => setPrompt(event.target.value)} /></label>
      <div className="grid two">
        {["rag_search", "mistake_hint", "formula_verify"].map((skill) => (
          <label className="row" key={skill}>
            <input
              type="checkbox"
              checked={skills.includes(skill)}
              onChange={(event) => setSkills((current) => event.target.checked ? [...current, skill] : current.filter((item) => item !== skill))}
            />
            {skill}
          </label>
        ))}
      </div>
      <button type="submit">保存助手配置</button>
    </form>
  );
}

type Assignment = {
  id: string;
  title: string;
  status: string;
  maxScore: number;
  items: Array<{ id: string; stem: string; answer?: string }>;
};

export function AssignmentBuilderPage() {
  const [assignments, setAssignments] = useState<Assignment[]>([]);
  const [title, setTitle] = useState("线性方程组随堂测验");
  const [knowledgePoint, setKnowledgePoint] = useState("线性方程组");
  const [stem, setStem] = useState("齐次线性方程组是否一定有解？");
  const [answer, setAnswer] = useState("是");
  const [maxScore, setMaxScore] = useState(10);
  const [error, setError] = useState("");

  async function refresh() {
    setAssignments(await api<Assignment[]>(classApiPath("/assignments")));
  }

  useEffect(() => {
    refresh().catch((value) => setError(errorText(value)));
  }, []);

  async function create(event: FormEvent) {
    event.preventDefault();
    setError("");
    try {
      await api(classApiPath("/assignments"), {
        method: "POST",
        body: JSON.stringify({
          title,
          maxScore,
          items: [{ id: "q1", stem, answer, knowledgePoint, type: "short", difficulty: "基础", explanation: "依据已发布评分规则" }],
        }),
      });
      await refresh();
    } catch (value) {
      setError(errorText(value));
    }
  }

  async function publish(id: string) {
    setError("");
    try {
      await api(`/api/v1/assignments/${id}/publish`, { method: "POST" });
      await refresh();
    } catch (value) {
      setError(errorText(value));
    }
  }

  return (
    <div className="grid">
      <header className="page-head">
        <div>
          <h2>作业发布</h2>
          <p className="muted">草稿发布后冻结题干、答案与评分规则；学生端不返回答案。</p>
        </div>
      </header>
      {error && <p className="notice err">{error}</p>}
      <div className="cols">
        <form className="panel grid" onSubmit={create}>
          <h3>新建作业草稿</h3>
          <label className="muted">标题<input value={title} onChange={(event) => setTitle(event.target.value)} /></label>
          <label className="muted">知识点<input value={knowledgePoint} onChange={(event) => setKnowledgePoint(event.target.value)} /></label>
          <label className="muted">题干<textarea rows={4} value={stem} onChange={(event) => setStem(event.target.value)} /></label>
          <label className="muted">标准答案<input value={answer} onChange={(event) => setAnswer(event.target.value)} /></label>
          <label className="muted">作业总分<input type="number" min={0.1} max={10000} step={0.5} value={maxScore} onChange={(event) => setMaxScore(Number(event.target.value))} /></label>
          <button type="submit">保存草稿</button>
        </form>
        <section className="panel grid">
          <h3>作业列表</h3>
          <ul className="sub-list">
            {assignments.map((assignment) => (
              <li key={assignment.id} className="row spread">
                <div><b>{assignment.title}</b><small className="muted mono">{assignment.id} · {assignment.maxScore || 10} 分</small></div>
                <div className="row">
                  <span className={`badge ${assignment.status === "published" ? "done" : "todo"}`}>{assignment.status}</span>
                  {assignment.status === "draft" && <button onClick={() => publish(assignment.id)}>发布并冻结</button>}
                </div>
              </li>
            ))}
          </ul>
        </section>
      </div>
    </div>
  );
}

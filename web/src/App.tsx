import { FormEvent, useEffect, useState } from "react";
import { Link, Navigate, Route, Routes } from "react-router-dom";
import { clearSession, getRole, getToken, login, loginWithPassword, logout, refreshSession } from "./api";
import { TeacherHome } from "./pages/teacher/Home";
import { StudentHome } from "./pages/student/Home";
import "./styles.css";

function Login() {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function go(role: string) {
    window.location.assign(role === "teacher" ? "/teacher" : "/student");
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      const data = await loginWithPassword(username, password);
      const role = String(data.user?.role || "").toLowerCase();
      if (role !== "teacher" && role !== "student") {
        setError("登录成功但角色无效，请重试");
        return;
      }
      await go(role);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="login-page">
      <section className="login-visual" aria-label="CampusClaw">
        <h1 className="login-brand">CampusClaw</h1>
        <p className="login-lead">可验证生成、多模态评价与自适应辅导——贯通课前到课后的智能教学闭环。</p>
      </section>

      <section className="login-panel">
        <form className="login-form" onSubmit={onSubmit}>
          <h2>进入课堂</h2>
          <p className="muted">使用演示账号登录教师端或学生端。</p>

          <label className="muted">账号
            <input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" />
          </label>
          <label className="muted">密码
            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
          </label>
          <button type="submit" disabled={loading}>{loading ? "登录中…" : "登录"}</button>
          {error && <p className="error">{error}</p>}

          <div className="quick-row">
            <button
              type="button"
              className="secondary"
              onClick={async () => {
                await login("teacher");
                await go("teacher");
              }}
            >
              教师演示
            </button>
            <button
              type="button"
              className="secondary"
              onClick={async () => {
                await login("student");
                await go("student");
              }}
            >
              学生演示
            </button>
          </div>
        </form>
      </section>
    </div>
  );
}

function Guard({ role, children }: { role: "teacher" | "student"; children: React.ReactNode }) {
  const [verifiedRole, setVerifiedRole] = useState<"teacher" | "student" | "">("");
  const [checking, setChecking] = useState(Boolean(getToken()));
  useEffect(() => {
    let active = true;
    if (!getToken()) {
      setChecking(false);
      return () => { active = false; };
    }
    refreshSession()
      .then((user) => {
        if (!active) return;
        const current = user.role === "teacher" || user.role === "student" ? user.role : "";
        setVerifiedRole(current);
      })
      .catch(() => {
        if (!active) return;
        clearSession();
        setVerifiedRole("");
      })
      .finally(() => {
        if (active) setChecking(false);
      });
    return () => { active = false; };
  }, []);
  if (checking) return <p className="empty-hint">正在校验登录会话…</p>;
  if (!getToken() || !verifiedRole) return <Navigate to="/" replace />;
  if (verifiedRole !== role) return <Navigate to={verifiedRole === "teacher" ? "/teacher" : "/student"} replace />;
  return <>{children}</>;
}

function Shell({ children }: { children: React.ReactNode }) {
  const authed = Boolean(getToken());
  return (
    <div className="app-shell">
      <header className="topbar">
        <div className="brand">CampusClaw</div>
        <nav className="nav">
          <Link to={getRole() === "teacher" ? "/teacher" : "/student"}>工作台</Link>
          {authed && (
            <a
              href="/"
              onClick={async (e) => {
                e.preventDefault();
                await logout();
                window.location.assign("/");
              }}
            >
              退出
            </a>
          )}
        </nav>
      </header>
      {children}
    </div>
  );
}

export function App() {
  return (
    <Routes>
      <Route path="/" element={<Login />} />
      <Route
        path="/teacher/*"
        element={
          <Guard role="teacher">
            <Shell><TeacherHome /></Shell>
          </Guard>
        }
      />
      <Route
        path="/student/*"
        element={
          <Guard role="student">
            <Shell><StudentHome /></Shell>
          </Guard>
        }
      />
    </Routes>
  );
}

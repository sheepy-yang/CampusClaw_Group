import { Link, Route, Routes } from "react-router-dom";
import { TeacherHome } from "./pages/teacher/Home";
import { StudentHome } from "./pages/student/Home";

/** Project skeleton: product routes only, without teaching-business implementation. */
export function App() {
  return (
    <div style={{ fontFamily: "system-ui", padding: 24 }}>
      <header style={{ marginBottom: 24 }}>
        <strong>CampusClaw</strong>
        <nav style={{ display: "inline-flex", gap: 16, marginLeft: 24 }}>
          <Link to="/teacher">教师端</Link>
          <Link to="/student">学生端</Link>
        </nav>
      </header>
      <Routes>
        <Route
          path="/"
          element={
            <main>
              <h1>CampusClaw 小组项目骨架</h1>
              <p>当前版本用于验证启动、分层、配置、健康检查和工程规约，不代表业务功能已经完成。</p>
              <p><a href="/health">查看服务与依赖健康状态</a></p>
            </main>
          }
        />
        <Route path="/teacher/*" element={<TeacherHome />} />
        <Route path="/student/*" element={<StudentHome />} />
      </Routes>
    </div>
  );
}

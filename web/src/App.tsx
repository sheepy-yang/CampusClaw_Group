import { Link, Route, Routes } from "react-router-dom";
import { TeacherHome } from "./pages/teacher/Home";
import { StudentHome } from "./pages/student/Home";

/** 应用骨架：教师端 / 学生端路由占位。 */
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
        <Route path="/" element={<p>骨架已就绪。请按分工实现十二项技能页面。</p>} />
        <Route path="/teacher/*" element={<TeacherHome />} />
        <Route path="/student/*" element={<StudentHome />} />
      </Routes>
    </div>
  );
}

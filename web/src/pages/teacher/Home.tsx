import { Link, Route, Routes } from "react-router-dom";
import { routeManifest } from "../../app/route-manifest";

/** 教师端入口：技能开关、审核、批改、讲评、诊断、演练等占位。 */
export function TeacherHome() {
  const routes = routeManifest.filter((route) => route.role === "teacher");

  return (
    <div>
      <h1>教师端（骨架）</h1>
      <ul>
        {routes.map((route) => (
          <li key={route.path}>
            <Link to={route.path}>{route.label}</Link>
          </li>
        ))}
      </ul>
      <Routes>
        <Route
          path="*"
          element={<p>该能力已在产品路由清单中声明，将由对应 OpenSpec change 实现。</p>}
        />
      </Routes>
    </div>
  );
}

import { Link, Route, Routes } from "react-router-dom";
import { routeManifest } from "../../app/route-manifest";

/** 学生端入口：试卷、预习、练习、辅导、课堂互动等占位。 */
export function StudentHome() {
  const routes = routeManifest.filter((route) => route.role === "student");

  return (
    <div>
      <h1>学生端（骨架）</h1>
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

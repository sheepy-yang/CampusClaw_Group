import { Link, Route, Routes } from "react-router-dom";

/** 学生端入口：试卷、预习、练习、辅导、课堂互动等占位。 */
export function StudentHome() {
  return (
    <div>
      <h1>学生端（骨架）</h1>
      <ul>
        <li><Link to="assignments">已发布试卷 / 作业</Link></li>
        <li><Link to="preview">预习单</Link></li>
        <li><Link to="review-path">个性化复习路径</Link></li>
        <li><Link to="adaptive">自适应练习闯关</Link></li>
        <li><Link to="tutor">追问式辅导</Link></li>
        <li><Link to="classroom">课堂短文本互动</Link></li>
        <li><Link to="attempt">作答与版本回看</Link></li>
      </ul>
      <Routes>
        <Route path="*" element={<p>TODO: 具体页面待实现</p>} />
      </Routes>
    </div>
  );
}

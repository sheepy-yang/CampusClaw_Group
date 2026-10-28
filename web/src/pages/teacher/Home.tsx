import { Link, Route, Routes } from "react-router-dom";

/** 教师端入口：技能开关、审核、批改、讲评、诊断、演练等占位。 */
export function TeacherHome() {
  return (
    <div>
      <h1>教师端（骨架）</h1>
      <ul>
        <li><Link to="proposition">智能命题</Link></li>
        <li><Link to="lesson-plan">智能备课</Link></li>
        <li><Link to="virtual-classroom">虚拟课堂演练</Link></li>
        <li><Link to="grade">多模态批改 / 改分</Link></li>
        <li><Link to="review">班级讲评</Link></li>
        <li><Link to="diagnosis">学习诊断</Link></li>
        <li><Link to="replay">作答回放与原创性审查</Link></li>
        <li><Link to="cluster">课堂回答聚类</Link></li>
        <li><Link to="skills">班级技能开关</Link></li>
      </ul>
      <Routes>
        <Route path="*" element={<p>TODO: 具体页面待实现</p>} />
      </Routes>
    </div>
  );
}

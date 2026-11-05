import { expect, test, type APIRequestContext } from "@playwright/test";

async function login(request: APIRequestContext, username: string) {
  const response = await request.post("/api/v1/auth/login", {
    data: { username, password: "demo123" },
  });
  expect(response.ok(), await response.text()).toBeTruthy();
  const body = await response.json() as { token: string; user: { classId: string; role: string } };
  return body;
}

test("browser verifies the server session before opening the teacher workspace", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("账号").fill("teacher");
  await page.getByLabel("密码").fill("demo123");
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page).toHaveURL(/\/teacher/);
  await expect(page.getByText("教师工作台")).toBeVisible();
});

test("server-enforced class isolation hides another class", async ({ request }) => {
  const other = await login(request, "student_other");
  const own = await request.get(`/api/v1/classes/${other.user.classId}/materials`, {
    headers: { Authorization: `Bearer ${other.token}` },
  });
  expect(own.ok(), await own.text()).toBeTruthy();

  const crossClass = await request.get("/api/v1/classes/class-demo/materials", {
    headers: { Authorization: `Bearer ${other.token}` },
  });
  expect(crossClass.status()).toBe(404);
});

test("published assignment is submitted, graded asynchronously, and keeps the teacher-defined score", async ({ request }) => {
  const teacher = await login(request, "teacher");
  const student = await login(request, "student");
  const teacherHeaders = { Authorization: `Bearer ${teacher.token}` };
  const studentHeaders = { Authorization: `Bearer ${student.token}` };
  const suffix = Date.now().toString(36);

  const createdResponse = await request.post("/api/v1/classes/class-demo/assignments", {
    headers: teacherHeaders,
    data: {
      title: `E2E 随堂测验 ${suffix}`,
      maxScore: 20,
      items: [{
        id: `q-${suffix}`,
        stem: "齐次线性方程组是否一定有解？",
        answer: "是",
        knowledgePoint: "线性方程组",
        type: "choice",
        difficulty: "基础",
      }],
    },
  });
  expect(createdResponse.status(), await createdResponse.text()).toBe(201);
  const assignment = await createdResponse.json() as { id: string; items: Array<{ id: string }> };

  const publishResponse = await request.post(`/api/v1/assignments/${assignment.id}/publish`, {
    headers: teacherHeaders,
  });
  expect(publishResponse.ok(), await publishResponse.text()).toBeTruthy();

  const submitResponse = await request.post(`/api/v1/assignments/${assignment.id}/submissions`, {
    headers: {
      ...studentHeaders,
      "Idempotency-Key": `e2e-assignment-${suffix}`,
    },
    data: { answers: { [assignment.items[0].id]: "是" } },
  });
  expect(submitResponse.status(), await submitResponse.text()).toBe(202);
  const accepted = await submitResponse.json() as { taskId: string; submissionId: string };

  await expect.poll(async () => {
    const taskResponse = await request.get(`/api/v1/tasks/${accepted.taskId}`, { headers: studentHeaders });
    expect(taskResponse.ok(), await taskResponse.text()).toBeTruthy();
    return (await taskResponse.json() as { status: string }).status;
  }).toBe("succeeded");

  const submissionResponse = await request.get(`/api/v1/submissions/${accepted.submissionId}`, {
    headers: studentHeaders,
  });
  expect(submissionResponse.ok(), await submissionResponse.text()).toBeTruthy();
  const submission = await submissionResponse.json() as { score: number; maxScore: number; status: string };
  expect(submission).toMatchObject({ score: 20, maxScore: 20, status: "graded" });
});

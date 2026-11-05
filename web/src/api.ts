import type { ApiError } from "./shared/api/types";

const TOKEN_KEY = "campusclaw_token";
const ROLE_KEY = "campusclaw_role";
const CLASS_KEY = "campusclaw_class_id";

export function getToken() {
  return localStorage.getItem(TOKEN_KEY) || "";
}

export function setSession(token: string, role: string, classId: string) {
  if (localStorage.getItem(CLASS_KEY) !== classId) {
    localStorage.removeItem("campusclaw_lesson_plan_id");
    localStorage.removeItem("campusclaw_classroom_session");
  }
  localStorage.setItem(TOKEN_KEY, token);
  localStorage.setItem(ROLE_KEY, role);
  localStorage.setItem(CLASS_KEY, classId);
}

export function clearSession() {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(ROLE_KEY);
  localStorage.removeItem(CLASS_KEY);
  localStorage.removeItem("campusclaw_lesson_plan_id");
  localStorage.removeItem("campusclaw_classroom_session");
}

export async function logout() {
  try {
    await api("/api/v1/auth/logout", { method: "POST", body: "{}" });
  } finally {
    clearSession();
  }
}

export function getRole() {
  return localStorage.getItem(ROLE_KEY) || "";
}

export function getClassId() {
  return localStorage.getItem(CLASS_KEY) || "";
}

export function classApiPath(suffix = "") {
  const classId = getClassId();
  if (!classId) throw new Error("登录会话缺少班级信息，请退出后重新登录");
  return `/api/v1/classes/${encodeURIComponent(classId)}${suffix}`;
}

export async function api<T = unknown>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (!(init.body instanceof FormData) && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  const token = getToken();
  if (token) headers.set("Authorization", `Bearer ${token}`);
  const res = await fetch(path, { ...init, headers });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    const err = data as Partial<ApiError> & { error?: string };
    const message = err.message || err.error || res.statusText;
    const wrapped = new Error(message) as Error & { code?: string; requestId?: string };
    wrapped.code = err.code;
    wrapped.requestId = err.request_id;
    throw wrapped;
  }
  return data as T;
}

export async function uploadAttachment(file: File) {
  const form = new FormData();
  form.append("file", file);
  return api<{ digest: string; contentType: string; size: number }>("/api/v1/attachments", {
    method: "POST",
    body: form,
  });
}

export function newIdempotencyKey(prefix: string) {
  return `${prefix}:${crypto.randomUUID()}`;
}

export async function login(role: "teacher" | "student") {
  return loginWithPassword(role, "demo123");
}

export async function loginWithPassword(username: string, password: string) {
  const data = await api<{ token: string; user: { role: string; name: string; classId: string } }>(
    "/api/v1/auth/login",
    { method: "POST", body: JSON.stringify({ username, password }) }
  );
  setSession(data.token, data.user.role, data.user.classId);
  return data;
}

export async function refreshSession() {
  const user = await api<{ role: string; name: string; classId: string; userId: string }>("/api/v1/auth/me");
  setSession(getToken(), user.role, user.classId);
  return user;
}

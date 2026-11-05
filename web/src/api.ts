import type { ApiError } from "./shared/api/types";

const TOKEN_KEY = "campusclaw_token";
const ROLE_KEY = "campusclaw_role";

export function getToken() {
  return localStorage.getItem(TOKEN_KEY) || "";
}

export function setSession(token: string, role: string) {
  localStorage.setItem(TOKEN_KEY, token);
  localStorage.setItem(ROLE_KEY, role);
}

export function clearSession() {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(ROLE_KEY);
}

export function getRole() {
  return localStorage.getItem(ROLE_KEY) || "";
}

export async function api<T = unknown>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set("Content-Type", "application/json");
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

export async function login(role: "teacher" | "student") {
  const data = await api<{ token: string; user: { role: string; name: string; classId: string } }>(
    "/api/v1/auth/login",
    { method: "POST", body: JSON.stringify({ role }) }
  );
  setSession(data.token, data.user.role);
  return data;
}

export async function loginWithPassword(username: string, password: string) {
  const data = await api<{ token: string; user: { role: string; name: string; classId: string } }>(
    "/api/v1/auth/login",
    { method: "POST", body: JSON.stringify({ username, password }) }
  );
  setSession(data.token, data.user.role);
  return data;
}

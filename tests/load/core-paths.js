import http from "k6/http";
import { check, sleep } from "k6";

export const options = {
  scenarios: {
    core_reads: {
      executor: "constant-vus",
      vus: Number(__ENV.VUS || 10),
      duration: __ENV.DURATION || "30s",
    },
  },
  thresholds: {
    http_req_failed: ["rate<0.01"],
    http_req_duration: ["p(95)<1000"],
  },
};

const baseURL = __ENV.BASE_URL || "http://host.docker.internal:18088";

export function setup() {
  const response = http.post(
    `${baseURL}/api/v1/auth/login`,
    JSON.stringify({ username: "student", password: "demo123" }),
    { headers: { "Content-Type": "application/json" } },
  );
  check(response, { "login succeeds": (value) => value.status === 200 });
  const token = response.json("token");
  if (!token) throw new Error("load setup could not obtain a session token");
  return { token };
}

export default function (data) {
  const headers = { Authorization: `Bearer ${data.token}`, "Content-Type": "application/json" };
  const materials = http.get(`${baseURL}/api/v1/classes/class-demo/materials`, { headers });
  check(materials, { "materials 200": (response) => response.status === 200 });

  const assignments = http.get(`${baseURL}/api/v1/classes/class-demo/assignments`, { headers });
  check(assignments, { "assignments 200": (response) => response.status === 200 });

  const search = http.post(
    `${baseURL}/api/v1/classes/class-demo/search`,
    JSON.stringify({ query: "高斯消元", limit: 3 }),
    { headers },
  );
  check(search, {
    "search 200": (response) => response.status === 200,
    "search is class scoped": (response) => response.json("hits") !== undefined,
  });
  sleep(0.5);
}

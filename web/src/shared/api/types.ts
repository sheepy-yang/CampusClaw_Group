export interface ApiError {
  code: string;
  message: string;
  request_id: string;
}

export interface HealthResult {
  status: "up" | "down";
}

export interface HealthReport {
  status: "up" | "down";
  checks: Record<string, HealthResult>;
}

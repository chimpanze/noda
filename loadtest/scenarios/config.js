// Shared configuration for all k6 scenarios.

export const BASE_URL = __ENV.BASE_URL || "http://localhost:3000";

// Default thresholds applied to all scenarios.
export const defaultThresholds = {
  http_req_failed: ["rate<0.01"], // <1% errors
  http_req_duration: ["p(95)<500"], // p95 < 500ms
};

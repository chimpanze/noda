import http from "k6/http";
import { check } from "k6";
import { BASE_URL, defaultThresholds } from "./config.js";

export const options = {
  vus: 50,
  duration: "30s",
  thresholds: {
    ...defaultThresholds,
    http_req_duration: ["p(95)<10"], // baseline should be fast
  },
};

export default function () {
  const res = http.get(`${BASE_URL}/health`);
  check(res, {
    "status is 200": (r) => r.status === 200,
    "body has status": (r) => r.json().status === "healthy",
  });
}

import http from "k6/http";
import { check } from "k6";
import { BASE_URL, defaultThresholds } from "./config.js";

export const options = {
  stages: [
    { duration: "30s", target: 50 },
    { duration: "1m", target: 200 },
    { duration: "2m", target: 500 },
    { duration: "1m", target: 500 },
    { duration: "30s", target: 0 },
  ],
  thresholds: {
    ...defaultThresholds,
    http_req_duration: ["p(99)<200"],
  },
};

export default function () {
  const payload = JSON.stringify({ name: `user-${__VU}` });
  const params = { headers: { "Content-Type": "application/json" } };
  const res = http.post(`${BASE_URL}/echo`, payload, params);
  check(res, {
    "status is 200": (r) => r.status === 200,
  });
}

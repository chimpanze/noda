import http from "k6/http";
import { check } from "k6";
import { BASE_URL, defaultThresholds } from "./config.js";

export const options = {
  vus: 50,
  duration: "30s",
  thresholds: {
    ...defaultThresholds,
    http_req_duration: ["p(95)<25"],
  },
};

export default function () {
  const payload = JSON.stringify({ name: "Alice" });
  const params = { headers: { "Content-Type": "application/json" } };
  const res = http.post(`${BASE_URL}/echo`, payload, params);
  check(res, {
    "status is 200": (r) => r.status === 200,
    "has greeting": (r) => JSON.parse(r.body).greeting !== undefined,
  });
}

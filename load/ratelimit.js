// Rate-limit SLI: from one client IP, hammer /products until we see 429s.
// PASS: at least one 429 when RATE_LIMIT_MAX is low; no 5xx.
// Run with API RATE_LIMIT_MAX=30 RATE_LIMIT_WINDOW=1m for a clear signal.
import { check } from "k6";
import { Counter } from "k6/metrics";
import http from "k6/http";
import { BASE_URL, headers, register } from "./helpers.js";

const limited = new Counter("bench_rate_limited");
const ok = new Counter("bench_rate_ok");
const serverErr = new Counter("bench_rate_5xx");

export const options = {
  vus: 5,
  duration: "20s",
  thresholds: {
    bench_rate_limited: ["count>0"],
    bench_rate_5xx: ["count==0"],
  },
};

export function setup() {
  const u = register("ratelimit-user");
  if (!u.ok) throw new Error(`register failed: ${u.status}`);
  return { token: u.token };
}

export default function (data) {
  const res = http.get(`${BASE_URL}/api/v1/products/catalog?limit=5`, {
    headers: headers(data.token),
  });
  if (res.status === 429) {
    limited.add(1);
    check(res, { "got 429": () => true });
  } else if (res.status >= 500) {
    serverErr.add(1);
    check(res, { "no 5xx": () => false });
  } else {
    ok.add(1);
    check(res, { "2xx/other": () => res.status < 500 });
  }
}

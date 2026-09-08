// Ramping load test — finds the realistic ceiling for THIS box, not a
// production cluster. Thresholds are deliberately loose for weak hardware
// (AMD A8 / DDR3 / SATA SSD); tighten them once you know your baseline.
//
// NOTE: RATE_LIMIT_MAX defaults to 100 req/min per IP. At more than a
// handful of VUs from one machine you WILL see 429s — that's the rate
// limiter working as designed, not a bug. Bump RATE_LIMIT_MAX in .env if
// you want to measure raw throughput instead of the rate limiter's ceiling.
import { check, sleep } from "k6";
import http from "k6/http";

const BASE_URL = __ENV.BASE_URL || "http://localhost:8080";
const API_KEY = __ENV.API_KEY || "dev-api-key";

export const options = {
  stages: [
    { duration: "30s", target: 5 }, // ramp up
    { duration: "1m", target: 5 }, // hold — read baseline
    { duration: "30s", target: 15 }, // ramp up
    { duration: "1m", target: 15 }, // hold — find where latency bends
    { duration: "30s", target: 0 }, // ramp down
  ],
  thresholds: {
    http_req_duration: ["p(95)<800"],
    // Rate limiting is expected to reject some requests under load — allow it.
    http_req_failed: ["rate<0.20"],
  },
};

function headers(token) {
  const h = { "Content-Type": "application/json", apikey: API_KEY };
  if (token) h["Authorization"] = `Bearer ${token}`;
  return h;
}

// One registered user per VU, reused across iterations — avoids hammering
// bcrypt (intentionally expensive) on every single request.
export function setup() {
  const users = [];
  for (let i = 0; i < 20; i++) {
    const email = `k6-load-${i}-${Date.now()}@example.com`;
    const res = http.post(
      `${BASE_URL}/api/v1/authentication/register`,
      JSON.stringify({ name: `K6 User ${i}`, email, password: "secret123" }),
      { headers: headers() }
    );
    if (res.status === 201) {
      users.push({ token: res.json("data.tokens.accessToken") });
    }
  }
  return { users };
}

export default function (data) {
  const user = data.users[__VU % data.users.length];
  if (!user) return;

  const create = http.post(
    `${BASE_URL}/api/v1/products`,
    JSON.stringify({ name: `Load Product ${__ITER}`, price: 1000, stock: 5 }),
    { headers: headers(user.token) }
  );
  check(create, {
    "create: 2xx or 429": (r) => r.status < 300 || r.status === 429,
  });

  const list = http.get(`${BASE_URL}/api/v1/products?limit=20`, {
    headers: headers(user.token),
  });
  check(list, {
    "list: 2xx or 429": (r) => r.status < 300 || r.status === 429,
  });

  sleep(Math.random() * 1 + 0.5); // 0.5-1.5s think time — simulate a real client
}

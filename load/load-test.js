// Product CRUD ramp (legacy). Prefer ./scripts/bench.sh commerce matrix.
import { check, sleep } from "k6";
import http from "k6/http";
import { headers, register } from "./helpers.js";

const BASE_URL = __ENV.BASE_URL || "http://localhost:8080";

export const options = {
  stages: [
    { duration: "20s", target: 5 },
    { duration: "40s", target: 5 },
    { duration: "20s", target: 0 },
  ],
  thresholds: {
    http_req_duration: ["p(95)<800"],
    http_req_failed: ["rate<0.20"],
  },
};

export function setup() {
  const users = [];
  for (let i = 0; i < 10; i++) {
    const u = register(`legacy-load-${i}`);
    if (u.ok) users.push(u);
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
  sleep(0.5);
}

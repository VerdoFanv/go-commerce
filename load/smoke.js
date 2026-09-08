// Smoke test: 1 VU, few iterations — proves the happy path works end to end
// before running the heavier load test. Run this first, always.
import { check, sleep } from "k6";
import http from "k6/http";

const BASE_URL = __ENV.BASE_URL || "http://localhost:8080";
const API_KEY = __ENV.API_KEY || "dev-api-key";

export const options = {
  vus: 1,
  iterations: 5,
  thresholds: {
    http_req_failed: ["rate==0"],
  },
};

function headers(token) {
  const h = { "Content-Type": "application/json", apikey: API_KEY };
  if (token) h["Authorization"] = `Bearer ${token}`;
  return h;
}

export default function () {
  const email = `k6-smoke-${__VU}-${__ITER}-${Date.now()}@example.com`;

  const register = http.post(
    `${BASE_URL}/api/v1/authentication/register`,
    JSON.stringify({ name: "K6 Smoke", email, password: "secret123" }),
    { headers: headers() }
  );
  check(register, { "register: 201": (r) => r.status === 201 });

  const token = register.json("data.tokens.accessToken");
  check(token, { "register: got access token": (t) => !!t });

  const create = http.post(
    `${BASE_URL}/api/v1/products`,
    JSON.stringify({ name: "Smoke Product", price: 1000, stock: 1 }),
    { headers: headers(token) }
  );
  check(create, { "create product: 201": (r) => r.status === 201 });

  const id = create.json("data.id");

  const get = http.get(`${BASE_URL}/api/v1/products/${id}`, {
    headers: headers(token),
  });
  check(get, { "get product: 200": (r) => r.status === 200 });

  const list = http.get(`${BASE_URL}/api/v1/products?limit=10`, {
    headers: headers(token),
  });
  check(list, { "list products: 200": (r) => r.status === 200 });

  const health = http.get(`${BASE_URL}/health/ready`);
  check(health, { "health ready: 200": (r) => r.status === 200 });

  sleep(1);
}

// Shared helpers for k6 scenarios. Keep auth/header logic in one place.
import http from "k6/http";

export const BASE_URL = __ENV.BASE_URL || "http://127.0.0.1:8080";
export const API_KEY = __ENV.API_KEY || "dev-api-key";

export function headers(token, extra) {
  const h = { "Content-Type": "application/json", apikey: API_KEY };
  if (token) h.Authorization = `Bearer ${token}`;
  if (extra) Object.assign(h, extra);
  return h;
}

export function register(namePrefix) {
  const email = `${namePrefix}-${__VU || 0}-${Date.now()}-${Math.random()
    .toString(36)
    .slice(2, 8)}@bench.local`;
  const res = http.post(
    `${BASE_URL}/api/v1/authentication/register`,
    JSON.stringify({ name: namePrefix, email, password: "secret123" }),
    { headers: headers() }
  );
  if (res.status !== 201) {
    return { ok: false, status: res.status, body: res.body };
  }
  return {
    ok: true,
    email,
    token: res.json("data.tokens.accessToken"),
    userId: res.json("data.user.id"),
  };
}

export function login(email, password) {
  const res = http.post(
    `${BASE_URL}/api/v1/authentication/login`,
    JSON.stringify({ email, password }),
    { headers: headers() }
  );
  if (res.status !== 200) {
    return { ok: false, status: res.status };
  }
  return {
    ok: true,
    token: res.json("data.tokens.accessToken"),
    role: res.json("data.user.role"),
  };
}

export function createProduct(token, { name, price, stock }) {
  const res = http.post(
    `${BASE_URL}/api/v1/products`,
    JSON.stringify({ name, price, stock }),
    { headers: headers(token) }
  );
  return res;
}

export function createOrder(token, productId, qty, idemKey) {
  return http.post(
    `${BASE_URL}/api/v1/orders`,
    JSON.stringify({ items: [{ productId, qty }] }),
    {
      headers: headers(token, { "Idempotency-Key": idemKey }),
    }
  );
}

export function getProduct(token, id) {
  return http.get(`${BASE_URL}/api/v1/products/${id}`, {
    headers: headers(token),
  });
}

export function scrapeMetrics() {
  return http.get(`${BASE_URL}/metrics`);
}

/** Parse a Prometheus gauge/counter value by metric name prefix. */
export function metricValue(body, name) {
  if (!body) return null;
  const lines = String(body).split("\n");
  for (const line of lines) {
    if (!line || line.startsWith("#")) continue;
    if (line.startsWith(name + " ") || line.startsWith(name + "{")) {
      const parts = line.trim().split(/\s+/);
      const v = parseFloat(parts[parts.length - 1]);
      if (!Number.isNaN(v)) return v;
    }
  }
  return null;
}

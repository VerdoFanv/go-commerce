// Mixed traffic: mostly browse (catalog/list) + minority checkout.
// Closer to real storefront ratio than CRUD-only hammering.
import { check, sleep } from "k6";
import http from "k6/http";
import { Trend } from "k6/metrics";
import {
  BASE_URL,
  createOrder,
  createProduct,
  headers,
  register,
} from "./helpers.js";

const browseLatency = new Trend("bench_browse_duration", true);
const buyLatency = new Trend("bench_buy_duration", true);

export const options = {
  scenarios: {
    mixed: {
      executor: "constant-vus",
      vus: Number(__ENV.BENCH_VUS || 15),
      duration: __ENV.BENCH_DURATION || "90s",
    },
  },
  thresholds: {
    bench_browse_duration: ["p(95)<300"],
    bench_buy_duration: ["p(95)<600"],
    http_req_failed: ["rate<0.05"],
  },
};

export function setup() {
  const seller = register("mixed-seller");
  if (!seller.ok) throw new Error("seller register failed");
  const products = [];
  for (let i = 0; i < 8; i++) {
    const p = createProduct(seller.token, {
      name: `Mixed SKU ${i}`,
      price: 12000 + i * 100,
      stock: 2000,
    });
    if (p.status === 201) products.push(p.json("data.id"));
  }
  const buyers = [];
  for (let i = 0; i < 20; i++) {
    const b = register(`mixed-buyer-${i}`);
    if (b.ok) buyers.push(b);
  }
  return { products, buyers };
}

export default function (data) {
  const buyer = data.buyers[__VU % data.buyers.length];
  if (!buyer || data.products.length === 0) return;

  const roll = Math.random();
  if (roll < 0.8) {
    const t0 = Date.now();
    const res = http.get(`${BASE_URL}/api/v1/products/catalog?limit=20`, {
      headers: headers(buyer.token),
    });
    browseLatency.add(Date.now() - t0);
    check(res, { "catalog ok": (r) => r.status === 200 || r.status === 429 });
  } else {
    const pid = data.products[__ITER % data.products.length];
    const t0 = Date.now();
    const res = createOrder(
      buyer.token,
      pid,
      1,
      `mixed-${__VU}-${__ITER}-${Date.now()}`
    );
    buyLatency.add(Date.now() - t0);
    check(res, {
      "buy ok": (r) => r.status === 201 || r.status === 409 || r.status === 429,
    });
  }
  sleep(0.05);
}

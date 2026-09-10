// Smoke: health + auth + product + ONE real checkout (the commerce hot path).
// Fail hard on any unexpected status — this is the gate before heavier benches.
import { check, sleep } from "k6";
import http from "k6/http";
import {
  BASE_URL,
  createOrder,
  createProduct,
  getProduct,
  headers,
  register,
} from "./helpers.js";

export const options = {
  vus: 1,
  iterations: 3,
  thresholds: {
    http_req_failed: ["rate==0"],
    checks: ["rate>0.99"],
  },
};

export default function () {
  const health = http.get(`${BASE_URL}/health/ready`);
  check(health, { "ready: 200": (r) => r.status === 200 });

  const seller = register("bench-seller");
  check(seller, { "seller registered": (s) => s.ok });
  if (!seller.ok) return;

  const product = createProduct(seller.token, {
    name: `Smoke SKU ${Date.now()}`,
    price: 15000,
    stock: 5,
  });
  check(product, { "create product: 201": (r) => r.status === 201 });
  const productId = product.json("data.id");

  const buyer = register("bench-buyer");
  check(buyer, { "buyer registered": (s) => s.ok });
  if (!buyer.ok) return;

  const order = createOrder(
    buyer.token,
    productId,
    1,
    `smoke-${__ITER}-${Date.now()}`
  );
  check(order, { "create order: 201": (r) => r.status === 201 });
  const orderId = order.json("data.id");

  const got = http.get(`${BASE_URL}/api/v1/orders/${orderId}`, {
    headers: headers(buyer.token),
  });
  check(got, {
    "get order: 200": (r) => r.status === 200,
    "order pending_payment or paid": (r) => {
      const st = r.json("data.status");
      return st === "pending_payment" || st === "paid";
    },
  });

  const stockAfter = getProduct(seller.token, productId);
  check(stockAfter, {
    "stock still >= 0": (r) => r.status === 200 && r.json("data.stock") >= 0,
  });

  const catalog = http.get(`${BASE_URL}/api/v1/products/catalog?limit=10`, {
    headers: headers(buyer.token),
  });
  check(catalog, { "catalog: 200": (r) => r.status === 200 });

  sleep(0.2);
}

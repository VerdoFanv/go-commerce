// Sustained checkout — measures orders/sec and latency on POST /orders
// (stock hold + outbox TX), not product CRUD.
//
// Raise RATE_LIMIT_MAX on the API for this run (e.g. 10000) or you measure
// the Redis limiter, not Postgres/Kafka capacity. See docs/BENCHMARK.md.
import { check, sleep } from "k6";
import { Counter, Trend } from "k6/metrics";
import {
  BASE_URL,
  createOrder,
  createProduct,
  headers,
  register,
  scrapeMetrics,
  metricValue,
} from "./helpers.js";
import http from "k6/http";

const orderOK = new Counter("bench_orders_created");
const orderFail = new Counter("bench_orders_failed");
const orderLatency = new Trend("bench_order_duration", true);

const VUS = Number(__ENV.BENCH_VUS || 20);
const DURATION = __ENV.BENCH_DURATION || "2m";
const STOCK = Number(__ENV.BENCH_STOCK || 5000);
const THINK = Number(__ENV.BENCH_THINK || 0); // seconds; 0 = saturate

export const options = {
  scenarios: {
    checkout: {
      executor: "constant-vus",
      vus: VUS,
      duration: DURATION,
    },
  },
  thresholds: {
    // Default 800ms matches measured APU lab box; set BENCH_ORDER_P95=500 for aspirational SLI.
    bench_order_duration: [
      `p(95)<${Number(__ENV.BENCH_ORDER_P95 || 800)}`,
      `p(99)<${Number(__ENV.BENCH_ORDER_P99 || 1500)}`,
    ],
    // Allow some failures if stock depletes near end; primary pass is p95 + no 5xx storm.
    http_req_failed: ["rate<0.05"],
  },
};

export function setup() {
  const sellers = [];
  const products = [];
  // A few SKUs so we don't serialize on one row lock forever.
  for (let i = 0; i < 5; i++) {
    const seller = register(`chk-seller-${i}`);
    if (!seller.ok) throw new Error(`seller register failed: ${seller.status}`);
    const p = createProduct(seller.token, {
      name: `Bench SKU ${i} ${Date.now()}`,
      price: 25000,
      stock: STOCK,
    });
    if (p.status !== 201) throw new Error(`create product failed: ${p.status}`);
    sellers.push(seller);
    products.push({ id: p.json("data.id"), sellerToken: seller.token });
  }

  const buyers = [];
  for (let i = 0; i < Math.max(VUS, 10); i++) {
    const b = register(`chk-buyer-${i}`);
    if (b.ok) buyers.push(b);
  }
  if (buyers.length === 0) throw new Error("no buyers registered");

  const metricsBefore = scrapeMetrics();
  return {
    products,
    buyers,
    outboxBefore: metricValue(metricsBefore.body, "golangbe_outbox_pending"),
  };
}

export default function (data) {
  const buyer = data.buyers[__VU % data.buyers.length];
  const product = data.products[__ITER % data.products.length];
  const key = `chk-${__VU}-${__ITER}-${Date.now()}`;

  const start = Date.now();
  const res = createOrder(buyer.token, product.id, 1, key);
  orderLatency.add(Date.now() - start);

  if (res.status === 201) {
    orderOK.add(1);
    check(res, { "order 201": () => true });
  } else if (res.status === 409) {
    // stock exhausted — expected near end of long runs
    check(res, { "order conflict ok": () => true });
  } else {
    orderFail.add(1);
    check(res, {
      "order unexpected": () => false,
    });
  }

  // light read mix
  http.get(`${BASE_URL}/api/v1/products/catalog?limit=5`, {
    headers: headers(buyer.token),
  });

  if (THINK > 0) sleep(THINK);
}

export function teardown(data) {
  const metricsAfter = scrapeMetrics();
  const outbox = metricValue(metricsAfter.body, "golangbe_outbox_pending");
  console.log(
    JSON.stringify({
      scenario: "checkout",
      outbox_pending_before: data.outboxBefore,
      outbox_pending_after: outbox,
      note: "outbox should drain toward 0 within ~30s after traffic stops",
    })
  );
}

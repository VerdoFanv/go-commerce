// Oversell race: stock=S, N concurrent qty=1 buys.
// PASS: created <= S, final stock >= 0, created + conflict ≈ attempts (no 5xx).
import { check } from "k6";
import { Counter } from "k6/metrics";
import {
  createOrder,
  createProduct,
  getProduct,
  register,
} from "./helpers.js";

const created = new Counter("oversell_created");
const conflict = new Counter("oversell_conflict");
const other = new Counter("oversell_other");

const STOCK = Number(__ENV.OVERSELL_STOCK || 20);
const VUS = Number(__ENV.OVERSELL_VUS || 60); // VUS >> STOCK

export const options = {
  scenarios: {
    race: {
      executor: "shared-iterations",
      vus: VUS,
      iterations: VUS, // one attempt per VU
      maxDuration: "2m",
    },
  },
  thresholds: {
    oversell_created: [`count<=${STOCK}`],
    oversell_other: ["count==0"],
    checks: ["rate>0.99"],
  },
};

export function setup() {
  const seller = register("oversell-seller");
  if (!seller.ok) throw new Error(`seller: ${seller.status}`);
  const p = createProduct(seller.token, {
    name: `Oversell SKU ${Date.now()}`,
    price: 10000,
    stock: STOCK,
  });
  if (p.status !== 201) throw new Error(`product: ${p.status} ${p.body}`);
  const productId = p.json("data.id");

  const buyers = [];
  for (let i = 0; i < VUS; i++) {
    const b = register(`oversell-buyer-${i}`);
    if (!b.ok) throw new Error(`buyer ${i}: ${b.status}`);
    buyers.push(b);
  }
  return { sellerToken: seller.token, productId, buyers, stock: STOCK };
}

export default function (data) {
  // shared-iterations: each VU runs until iterations exhausted; pin one buyer per VU
  const buyer = data.buyers[(__VU - 1) % data.buyers.length];
  const key = `oversell-${__VU}-${Date.now()}-${Math.random()}`;
  const res = createOrder(buyer.token, data.productId, 1, key);

  if (res.status === 201) {
    created.add(1);
    check(res, { created: () => true });
  } else if (res.status === 409) {
    conflict.add(1);
    check(res, { conflict: () => true });
  } else {
    other.add(1);
    check(res, {
      [`unexpected ${res.status}`]: () => false,
    });
  }
}

export function teardown(data) {
  const res = getProduct(data.sellerToken, data.productId);
  const stock = res.json("data.stock");
  console.log(
    JSON.stringify({
      scenario: "oversell",
      initial_stock: data.stock,
      final_stock: stock,
      product_id: data.productId,
    })
  );
  if (stock == null || stock < 0) {
    throw new Error(`OVERSELL DETECTED: final stock=${stock}`);
  }
  // All successful holds should leave stock at initial - created (>= 0).
  if (stock > data.stock) {
    throw new Error(`stock increased unexpectedly: ${stock}`);
  }
}

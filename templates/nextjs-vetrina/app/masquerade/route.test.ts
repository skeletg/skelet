import { NextRequest } from "next/server";
import { expect, it } from "vitest";

import { GET } from "./route";

const jwt = (claims: object) => `h.${Buffer.from(JSON.stringify(claims)).toString("base64url")}.sig`;
const land = (token: string, headers: Record<string, string> = {}) =>
  GET(new NextRequest(`http://shop.test/masquerade?token=${encodeURIComponent(token)}`, { headers }));

it("installs a masquerade session and lands on the homepage of this host", () => {
  const token = jwt({ msq: true, customer_id: "c1" });
  const res = land(token, { "x-forwarded-host": "evil.example" });
  expect(res.status).toBe(303);
  expect(res.headers.get("location")).toBe("/");
  expect(res.cookies.get("storefront_access_token")?.value).toBe(token);
  expect(res.cookies.get("storefront_access_token")?.httpOnly).toBe(true);
  expect(res.cookies.get("storefront_masquerade")?.value).toBe("1");
});

it.each([
  ["a customer's own token", jwt({ customer_id: "c1" })],
  ["one claiming msq as a string", jwt({ msq: "true" })],
  ["garbage", "not-a-token"],
  ["nothing", ""],
])("installs nothing for %s", (_label, token) => {
  const res = land(token);
  expect(res.headers.get("location")).toBe("/");
  expect(res.cookies.get("storefront_access_token")).toBeUndefined();
  expect(res.cookies.get("storefront_masquerade")).toBeUndefined();
});

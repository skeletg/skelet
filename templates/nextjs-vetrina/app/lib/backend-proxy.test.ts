import { existsSync, readFileSync } from "node:fs";
import { createServer, type IncomingMessage, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { fileURLToPath } from "node:url";
import { afterAll, beforeAll, describe, expect, it } from "vitest";

import { PROXIED_PREFIXES, proxyToBackend } from "@skelet/storefront-core/backend-proxy";

type Seen = { method: string; url: string; headers: IncomingMessage["headers"]; body: string };

let server: Server;
let backend = "";
const seen: Seen[] = [];

beforeAll(async () => {
  server = createServer((req, res) => {
    let body = "";
    req.on("data", (c) => (body += c));
    req.on("end", () => {
      seen.push({ method: req.method ?? "", url: req.url ?? "", headers: req.headers, body });
      if (req.url?.startsWith("/e/c/")) {
        res.writeHead(302, { Location: "https://brand.example/landing" });
        res.end();
        return;
      }
      res.writeHead(201, { "Content-Type": "image/webp", "Cache-Control": "public, max-age=31536000" });
      res.end("bytes");
    });
  });
  await new Promise<void>((r) => server.listen(0, "127.0.0.1", r));
  backend = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
});

afterAll(() => new Promise<void>((r) => server.close(() => r())));

describe("proxyToBackend", () => {
  it("forwards method, path, query, body and the client's forwarding headers, and streams the answer back", async () => {
    const res = await proxyToBackend(
      new Request("https://shop.test/t/rec?x=1", {
        method: "POST",
        headers: { "x-forwarded-for": "203.0.113.9", "x-channel-id": "c1", "content-type": "text/plain" },
        body: "payload",
      }),
      backend,
    );
    const got = seen.at(-1)!;
    expect(got.method).toBe("POST");
    expect(got.url).toBe("/t/rec?x=1");
    expect(got.body).toBe("payload");
    expect(got.headers["x-forwarded-for"]).toBe("203.0.113.9");
    expect(got.headers["x-channel-id"]).toBe("c1");
    expect(res.status).toBe(201);
    expect(res.headers.get("content-type")).toBe("image/webp");
    expect(res.headers.get("cache-control")).toBe("public, max-age=31536000");
    expect(await res.text()).toBe("bytes");
  });

  it("hands an email click's redirect to the browser instead of following it", async () => {
    const res = await proxyToBackend(new Request("https://shop.test/e/c/tok"), backend);
    expect(res.status).toBe(302);
    expect(res.headers.get("location")).toBe("https://brand.example/landing");
  });

  it("refuses a path that leaves the backend's same-origin links once its dot segments resolve", async () => {
    const before = seen.length;
    for (const path of ["/t/%2e%2e/api/admin/graphql", "/media/t/%2E%2E/%2e%2e/metrics", "/api/front/graphql"]) {
      expect((await proxyToBackend(new Request(`https://shop.test${path}`), backend)).status, path).toBe(404);
    }
    expect(seen.length).toBe(before);
  });

  it("answers 502 when the backend is unreachable", async () => {
    const res = await proxyToBackend(new Request("https://shop.test/media/t/w96/a.jpg"), "http://127.0.0.1:1");
    expect(res.status).toBe(502);
  });
});

describe("backend paths", () => {
  const root = fileURLToPath(new URL("../..", import.meta.url));

  it("each prefix is served by a runtime route handler, since a next.config rewrite bakes the build's backend url", () => {
    for (const p of PROXIED_PREFIXES) {
      const file = `${root}app${p}[...path]/route.ts`;
      expect(existsSync(file), file).toBe(true);
      expect(readFileSync(file, "utf8")).toContain("backendRoute as GET");
    }
    expect(readFileSync(`${root}next.config.ts`, "utf8")).not.toMatch(/rewrites\s*\(/);
  });
});

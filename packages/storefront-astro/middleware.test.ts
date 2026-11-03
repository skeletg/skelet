import { beforeAll, describe, expect, it, vi } from "vitest";

const SHOP = "11111111-2222-4333-8444-555555555555";
const OTHER = "66666666-7777-4888-8999-aaaaaaaaaaaa";

type Mod = typeof import("./middleware");
let storefront: Mod["storefront"];
let isAdapterExit: Mod["isAdapterExit"];
let core: typeof import("@skelet/storefront-core/request");

beforeAll(async () => {
  vi.stubEnv("STOREFRONT_CHANNEL_ID", SHOP);
  vi.stubEnv("STOREFRONT_CHANNEL_HOSTS", `other.test=${OTHER}`);
  ({ storefront, isAdapterExit } = await import("./middleware"));
  core = await import("@skelet/storefront-core/request");
});

function context(host = "shop.test") {
  const jar = new Map<string, string>();
  return {
    jar,
    request: new Request(`http://${host}/x`, { headers: { host } }),
    currentLocale: "de",
    cookies: {
      get: (n: string) => (jar.has(n) ? { value: jar.get(n)! } : undefined),
      set: (n: string, v: string) => void jar.set(n, v),
      delete: (n: string) => void jar.delete(n),
    },
    redirect: (to: string, status: number) => new Response(null, { status, headers: { Location: to } }),
    rewrite: (to: string) => new Response(`rewrote to ${to}`, { status: 404 }),
  };
}

const run = (ctx: ReturnType<typeof context>, page: () => Promise<Response> | Response) =>
  (storefront as (c: unknown, n: () => Promise<Response>) => Promise<Response>)(ctx, async () => page());

describe("storefront middleware", () => {
  it("answers the core from the request it runs in - its host's channel, its locale, its cookies", async () => {
    const ctx = context("other.test");
    let seen: unknown[] = [];
    await run(ctx, async () => {
      const req = core.request();
      (await req.cookies()).set("a", "1");
      seen = [await req.channelId(), await req.locale(), (await req.headers()).get("host")];
      return new Response("ok");
    });
    expect(seen).toEqual([OTHER, "de", "other.test"]);
    expect(ctx.jar.get("a")).toBe("1");
    expect(() => core.request()).toThrow(/No storefront request in scope/);
  });

  it("turns the core's redirect into a 303 and its not-found into the 404 page", async () => {
    const moved = await run(context(), () => core.redirect("/account/login"));
    expect([moved.status, moved.headers.get("location")]).toEqual([303, "/account/login"]);
    expect(await (await run(context(), () => core.notFound())).text()).toBe("rewrote to /404");
  });

  it("lets its exits through a core catch, and anything else up to Astro", async () => {
    let exit: unknown;
    await run(context(), () => {
      try {
        core.redirect("/x");
      } catch (err) {
        exit = err;
      }
      return new Response("ok");
    });
    expect(isAdapterExit(exit)).toBe(true);
    expect(() => core.rethrowControlFlow(exit)).toThrow();
    expect(() => core.rethrowControlFlow(new Error("app"))).not.toThrow();
    await expect(run(context(), () => Promise.reject(new Error("boom")))).rejects.toThrow("boom");
  });

  it("keeps one request's memo from another's", async () => {
    let calls = 0;
    const read = core.perRequest(async () => ++calls);
    const twice = async () => {
      let got: number[] = [];
      await run(context(), async () => {
        got = [await read(), await read()];
        return new Response("ok");
      });
      return got;
    };
    expect(await twice()).toEqual([1, 1]);
    expect(await twice()).toEqual([2, 2]);
  });

  it("says a page varies by host when hosts pick the channel", async () => {
    const res = await run(context(), () => new Response("ok"));
    expect(res.headers.get("vary")).toBe("Host");
  });
});

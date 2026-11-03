import { afterEach, describe, expect, it, vi } from "vitest";

import { getChannelLocales } from "./backend-locales";

// The answer is cached per channel for the module's lifetime, so every test asks for its own channel

function answer(currentChannel: unknown): Response {
  return new Response(JSON.stringify({ data: { currentChannel } }), { status: 200 });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("getChannelLocales", () => {
  it("sends the generated ChannelLocales query for the asked channel", async () => {
    const fetchMock = vi.fn().mockResolvedValue(answer({ defaultLocale: "en", locales: [] }));
    vi.stubGlobal("fetch", fetchMock);

    await getChannelLocales("ch-query");

    const [, init] = fetchMock.mock.calls[0];
    expect(init.headers["X-Channel-Id"]).toBe("ch-query");
    expect(JSON.parse(init.body).query).toMatch(/^query ChannelLocales\b/);
  });

  it("maps codes and falls back to the code when a locale has no hreflang value", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        answer({
          defaultLocale: "en",
          locales: [
            { code: "en", hreflangValue: "en-GB" },
            { code: "sr", hreflangValue: null },
          ],
        }),
      ),
    );

    expect(await getChannelLocales("ch-map")).toEqual({
      codes: ["en", "sr"],
      defaultLocale: "en",
      hreflang: { en: "en-GB", sr: "sr" },
    });
  });

  it("asks nothing without a channel - the backend would answer for its default one", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    expect(await getChannelLocales("")).toEqual({ codes: [], defaultLocale: "", hreflang: {} });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("serves an empty set when the backend fails with nothing cached", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 502 })));

    expect(await getChannelLocales("ch-down")).toEqual({ codes: [], defaultLocale: "", hreflang: {} });
  });
});

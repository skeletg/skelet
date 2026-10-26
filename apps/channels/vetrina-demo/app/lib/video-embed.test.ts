import { afterEach, describe, expect, it, vi } from "vitest";

import { VIDEO_EMBED_ORIGINS, videoEmbedUrl } from "./video-embed";

describe("videoEmbedUrl", () => {
  it.each([
    ["https://www.youtube.com/watch?v=dQw4w9WgXcQ", "https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ"],
    ["https://m.youtube.com/watch?v=dQw4w9WgXcQ&t=10", "https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ"],
    ["https://youtu.be/dQw4w9WgXcQ", "https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ"],
    ["https://www.youtube.com/embed/dQw4w9WgXcQ", "https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ"],
    ["https://www.youtube.com/shorts/dQw4w9WgXcQ", "https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ"],
    ["https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ", "https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ"],
    ["https://vimeo.com/76979871", "https://player.vimeo.com/video/76979871"],
    ["https://player.vimeo.com/video/76979871", "https://player.vimeo.com/video/76979871"],
  ])("plays %s through %s", (raw, embed) => {
    expect(videoEmbedUrl(raw)).toBe(embed);
  });

  it.each([
    "https://evil.example/embed/x",
    "https://player.evil.example/video/1",
    "https://www.youtube.com/watch?v=%22onload%3D",
    "https://vimeo.com/channels/staffpicks",
    "javascript:alert(1)",
    "not a url",
  ])("refuses %s", (raw) => {
    expect(videoEmbedUrl(raw)).toBeNull();
  });

  it("only ever builds URLs on the origins the CSP frames", () => {
    for (const raw of ["https://youtu.be/dQw4w9WgXcQ", "https://vimeo.com/76979871"]) {
      expect(VIDEO_EMBED_ORIGINS).toContain(new URL(videoEmbedUrl(raw)!).origin);
    }
  });
});

describe("storefront CSP", () => {
  afterEach(() => vi.unstubAllEnvs());

  const csp = async (env: string, path = "/") => {
    vi.resetModules();
    vi.stubEnv("NODE_ENV", env);
    const config = (await import("@/next.config")).default;
    const rules = await config.headers!();
    const rule = path.startsWith("/cms/preview") ? rules[1] : rules[0];
    const headers = Object.fromEntries(rule.headers.map((h) => [h.key, h.value]));
    return { policy: headers["Content-Security-Policy"].split("; "), frameOptions: headers["X-Frame-Options"] };
  };

  it("frames only the two video players and plays https or same-origin video", async () => {
    const { policy, frameOptions } = await csp("production");
    expect(policy).toContain("frame-src https://www.youtube-nocookie.com https://player.vimeo.com");
    expect(policy).toContain("media-src 'self' blob: https:");
    // Who may frame the storefront is unchanged
    expect(policy).toContain("frame-ancestors 'self'");
    expect(frameOptions).toBe("SAMEORIGIN");
  });

  it("lets dev play plain-http local media", async () => {
    expect((await csp("development")).policy).toContain("media-src 'self' blob: https: http:");
  });

  it("keeps preview framing scoped to the dashboard", async () => {
    const { policy, frameOptions } = await csp("production", "/cms/preview/x");
    expect(policy.find((d) => d.startsWith("frame-ancestors"))).toMatch(/^frame-ancestors 'self' /);
    expect(policy).toContain("frame-src https://www.youtube-nocookie.com https://player.vimeo.com");
    expect(frameOptions).toBeUndefined();
  });
});

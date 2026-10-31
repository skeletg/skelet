import { getSiteUrl, requireRequestChannel } from "@/app/lib/channel/site";
import { getSeoConfig } from "@/app/lib/config/read";

// A route handler, not robots.ts, so a non-blank operator override is served verbatim - Next's robots.ts only emits the
// structured allow/disallow shape, which can't carry arbitrary directives (comments, Crawl-delay, per-bot blocks)
export const dynamic = "force-dynamic";

// The baseline served when the override is blank - keeps private surfaces out and points
// crawlers at the sitemap index
function managedDefault(base: string): string {
  return [
    "User-agent: *",
    "Allow: /",
    "Disallow: /account/",
    "Disallow: /cart",
    "Disallow: /checkout",
    "Disallow: /api/",
    "",
    `Sitemap: ${base}/sitemap-index.xml`,
    "",
  ].join("\n");
}

export async function GET() {
  await requireRequestChannel();
  const [{ robotsOverride }, base] = await Promise.all([getSeoConfig(), getSiteUrl()]);
  const body = robotsOverride ? robotsOverride + "\n" : managedDefault(base);
  return new Response(body, {
    headers: { "content-type": "text/plain; charset=utf-8" },
  });
}

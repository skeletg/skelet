import { getSeoConfig } from "@/app/lib/config/read";

const SITE_URL = (
  process.env.STOREFRONT_BASE_URL ?? "http://localhost:3000"
).replace(/\/$/, "");

// A route handler, not robots.ts, so a non-blank operator override is served verbatim - Next's robots.ts only emits the
// structured allow/disallow shape, which can't carry arbitrary directives (comments, Crawl-delay, per-bot blocks)
export const revalidate = 3600;

// The baseline served when the override is blank - keeps private surfaces out and points
// crawlers at the sitemap index
function managedDefault(): string {
  return [
    "User-agent: *",
    "Allow: /",
    "Disallow: /account/",
    "Disallow: /cart",
    "Disallow: /checkout",
    "Disallow: /api/",
    "",
    `Sitemap: ${SITE_URL}/sitemap-index.xml`,
    "",
  ].join("\n");
}

export async function GET() {
  const { robotsOverride } = await getSeoConfig();
  const body = robotsOverride ? robotsOverride + "\n" : managedDefault();
  return new Response(body, {
    headers: { "content-type": "text/plain; charset=utf-8" },
  });
}

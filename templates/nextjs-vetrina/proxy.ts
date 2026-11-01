import type { NextFetchEvent, NextRequest } from "next/server";

import { storefrontProxy } from "@skelet/storefront-next/proxy";

// Channel and locale routing, the session refresh, the visitor id, a held promo code and the crawl report are the
// package's - a fix to any of them arrives with a release rather than as a change to this file
export function proxy(req: NextRequest, event: NextFetchEvent) {
  return storefrontProxy(req, event);
}

// Next reads the matcher from this file as written, so it stays here: every page and route handler but static files,
// and the /<channel>/ tree the package's rewrite lands on
export const config = {
  matcher: [
    "/((?!_next/|.*\\.(?:ico|png|jpe?g|gif|svg|webp|avif|css|js|map|woff2?|ttf|otf)$).*)",
    "/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})/:path*",
  ],
};

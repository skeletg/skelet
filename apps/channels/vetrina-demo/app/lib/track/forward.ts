import "server-only";

import { BACKEND_INTERNAL_URL } from "@/app/lib/backend-url";
import { CHANNEL_HEADER } from "@/app/lib/channel";
import { shopperHeaders } from "@/app/lib/shopper-headers";

const MAX_BODY = 32 << 10; // the backend's own cap

// A browser event on its way to the backend's /api/front/track. More specific than the /t/[...path] passthrough, so it
// lands here: the browser's request carries cookies, not the consent and click-id headers the backend reads, and those
// are added the same way every server-to-backend call adds them. Always a 204 - a tracking hiccup is never the shopper's
export async function forwardTrack(req: Request): Promise<Response> {
  const channel = req.headers.get(CHANNEL_HEADER);
  const body = await req.text();
  if (channel && body.length <= MAX_BODY) {
    await fetch(`${BACKEND_INTERNAL_URL}/api/front/track`, {
      method: "POST",
      headers: { ...(await shopperHeaders()), "Content-Type": "application/json", "X-Channel-Id": channel },
      body,
      signal: AbortSignal.timeout(3000),
      cache: "no-store",
    }).catch(() => {});
  }
  return new Response(null, { status: 204 });
}

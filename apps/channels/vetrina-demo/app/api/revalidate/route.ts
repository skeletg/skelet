import { revalidatePath, revalidateTag } from "next/cache";
import { timingSafeEqual } from "node:crypto";

import { PINNED_CHANNEL_ID, WARM_CHANNEL_HEADER, isChannelId } from "@/app/lib/channel";
import { channelRoutePaths, isPagePath } from "@/app/lib/channel-routing";
import { getChannelLocales } from "@/app/lib/locale/backend-locales";
import { localesByChannel, warmAcrossChannels, type WarmFailure } from "@/app/lib/revalidate-fanout";
import { warmUntilRendered } from "@/app/lib/warm";

// Backend POSTs here on CMS publish/unpublish to bust Data Cache tags, and for on-demand rebuilds. tags/paths invalidate lazily; warm
// re-GETs each path now Auth is a constant-time compared X-Revalidate-Secret - never cache this handler
export const dynamic = "force-dynamic";

function secretOk(provided: string | null): boolean {
  const expected = process.env.STOREFRONT_REVALIDATE_SECRET ?? "";
  if (!expected || !provided) return false;
  const a = Buffer.from(provided);
  const b = Buffer.from(expected);
  if (a.length !== b.length) return false;
  return timingSafeEqual(a, b);
}

function stringList(v: unknown, max: number): string[] {
  if (!Array.isArray(v)) return [];
  return v.filter((x): x is string => typeof x === "string" && x.length > 0).slice(0, max);
}

export async function POST(req: Request) {
  if (!secretOk(req.headers.get("x-revalidate-secret"))) {
    return Response.json({ error: "unauthorized" }, { status: 401 });
  }
  let payload: { tags?: unknown; paths?: unknown; warm?: unknown; channels?: unknown };
  try {
    payload = (await req.json()) as { tags?: unknown; paths?: unknown; warm?: unknown; channels?: unknown };
  } catch {
    return Response.json({ error: "invalid json" }, { status: 400 });
  }
  const tags = stringList(payload.tags, 64);
  const paths = stringList(payload.paths, 64);
  const warm = stringList(payload.warm, 25);
  const named = stringList(payload.channels, 500).filter(isChannelId);
  const channels = named.length > 0 ? named : PINNED_CHANNEL_ID ? [PINNED_CHANNEL_ID] : [];
  // { expire: 0 } = invalidate immediately (the on-demand/webhook form; the bare
  // single-arg revalidateTag is deprecated in this Next version)
  for (const tag of tags) revalidateTag(tag, { expire: 0 });
  const normalized = paths.map((path) => (path.startsWith("/") ? path : `/${path}`));
  const locales = normalized.some(isPagePath) ? await localesByChannel(channels, getChannelLocales) : new Map<string, string[]>();
  for (const p of normalized) {
    if (!isPagePath(p)) {
      revalidatePath(p);
      continue;
    }
    for (const ch of channels) {
      for (const route of channelRoutePaths(p, ch, locales.get(ch) ?? [])) revalidatePath(route);
    }
  }

  // A purge lands only after its own request ends, so the backend sends the warm separately and each warm waits for the purge
  let warmed = 0;
  let failed: WarmFailure[] = [];
  if (warm.length > 0 && channels.length > 0) {
    const origin = new URL(req.url).origin;
    const secret = req.headers.get("x-revalidate-secret") ?? "";
    ({ warmed, failed } = await warmAcrossChannels(channels, warm, (ch, p) =>
      warmUntilRendered(() =>
        fetch(origin + (p.startsWith("/") ? p : `/${p}`), {
          cache: "no-store",
          headers: { [WARM_CHANNEL_HEADER]: ch, "x-revalidate-secret": secret },
          signal: AbortSignal.any([req.signal, AbortSignal.timeout(15_000)]),
        }),
      ),
    ));
  }
  return Response.json({ revalidated: true, tags: tags.length, paths: paths.length, warmed, failed });
}

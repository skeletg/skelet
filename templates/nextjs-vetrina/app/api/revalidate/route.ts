import { revalidatePath, revalidateTag } from "next/cache";

import { PINNED_CHANNEL_ID, WARM_CHANNEL_HEADER } from "@skelet/storefront-core/channel";
import { channelRoutePaths, isPagePath } from "@skelet/storefront-core/channel-routing";
import { getChannelLocales } from "@skelet/storefront-core/locale/backend-locales";
import { localesByChannel, warmAcrossChannels, type WarmFailure } from "@skelet/storefront-core/revalidate-fanout";
import { REVALIDATE_SECRET_HEADER, readRevalidate, warmURL } from "@skelet/storefront-core/revalidate-payload";
import { warmUntilRendered } from "@skelet/storefront-core/warm";

// Backend POSTs here on CMS publish/unpublish and for on-demand rebuilds - tags/paths invalidate lazily, warm re-GETs each path now
export const dynamic = "force-dynamic";

const WARM_TIMEOUT_MS = 15_000;

export async function POST(req: Request) {
  const payload = await readRevalidate(req);
  if (payload instanceof Response) return payload;
  const { tags, paths, warm } = payload;
  const channels = payload.channels.length > 0 ? payload.channels : PINNED_CHANNEL_ID ? [PINNED_CHANNEL_ID] : [];
  // { expire: 0 } = invalidate immediately (the on-demand/webhook form; the bare
  // single-arg revalidateTag is deprecated in this Next version)
  for (const tag of tags) revalidateTag(tag, { expire: 0 });
  const locales = paths.some(isPagePath) ? await localesByChannel(channels, getChannelLocales) : new Map<string, string[]>();
  for (const p of paths) {
    if (!isPagePath(p)) {
      revalidatePath(p);
      continue;
    }
    for (const ch of channels) {
      for (const route of channelRoutePaths(p, ch, locales.get(ch) ?? [])) revalidatePath(route);
    }
  }

  // A purge lands only after its own request ends, so the backend sends the warm separately and each warm waits for the
  // purge
  let warmed = 0;
  let failed: WarmFailure[] = [];
  if (warm.length > 0 && channels.length > 0) {
    const origin = new URL(req.url).origin;
    const secret = req.headers.get(REVALIDATE_SECRET_HEADER) ?? "";
    ({ warmed, failed } = await warmAcrossChannels(channels, warm, (ch, p) => {
      const url = warmURL(origin, p);
      if (!url) return Promise.resolve(false);
      return warmUntilRendered(() =>
        fetch(url, {
          cache: "no-store",
          redirect: "manual",
          headers: { [WARM_CHANNEL_HEADER]: ch, [REVALIDATE_SECRET_HEADER]: secret },
          signal: AbortSignal.any([req.signal, AbortSignal.timeout(WARM_TIMEOUT_MS)]),
        }),
      );
    }));
  }
  return Response.json({ revalidated: true, tags: tags.length, paths: paths.length, warmed, failed });
}

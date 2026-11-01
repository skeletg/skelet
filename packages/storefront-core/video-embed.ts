// The only video players the storefront frames - next.config's frame-src names exactly these, so an embed pointing anywhere
// else would be blocked by the page's own CSP
export const VIDEO_EMBED_ORIGINS = ["https://www.youtube-nocookie.com", "https://player.vimeo.com"];

// A YouTube or Vimeo link (watch, short, share, embed or player URL) as its player URL - YouTube always through its
// privacy-enhanced host, which sets no cookies until the visitor plays. Anything else, or an id that isn't one, is null
export function videoEmbedUrl(raw: string): string | null {
  let u: URL;
  try {
    u = new URL(raw);
  } catch {
    return null;
  }
  if (u.protocol !== "https:" && u.protocol !== "http:") return null;
  const host = u.hostname.replace(/^(www|m)\./, "");
  const seg = u.pathname.split("/").filter(Boolean);

  let youtube: string | null | undefined;
  if (host === "youtube.com" || host === "youtube-nocookie.com") {
    youtube = seg[0] === "embed" || seg[0] === "shorts" ? seg[1] : u.searchParams.get("v");
  } else if (host === "youtu.be") {
    youtube = seg[0];
  }
  if (youtube !== undefined) {
    return youtube && /^[\w-]{6,20}$/.test(youtube) ? `https://www.youtube-nocookie.com/embed/${youtube}` : null;
  }

  // shortcut: unlisted Vimeo links (vimeo.com/<id>/<hash>) lose their hash, so they won't play - pass it as ?h= if CMS editors need them
  const vimeo = host === "vimeo.com" ? seg[0] : host === "player.vimeo.com" && seg[0] === "video" ? seg[1] : undefined;
  if (vimeo && /^\d+$/.test(vimeo)) return `https://player.vimeo.com/video/${vimeo}`;
  return null;
}

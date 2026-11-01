// Sized image URLs via the backend's /media/t/w<width>/<key> transform

// Small fixed ladder - every distinct width is its own cached derivative, so an
// open-ended set trades bytes for cold-transform latency
const WIDTHS = [280, 560, 840, 1120] as const;

// A spec the backend already applied - it hands cart lines and saved items a w160
// derivative rather than the raw asset, so the key has to be recovered from those too
const TRANSFORM_PREFIX = /^media\/t\/w\d+(?:h\d+)?\//;

function keyFromUrl(url: string): string | null {
  try {
    const path = new URL(url, "http://placeholder.invalid").pathname;
    // Re-sizing an already-sized URL nested the prefix (/media/t/w96/media/t/w160/<key>),
    // which is a key that exists in no bucket - so resolve back to the original every time
    const key = path.replace(/^\/+/, "").replace(TRANSFORM_PREFIX, "");
    return key || null;
  } catch {
    return null;
  }
}

/**
 * Single sized URL for a known-width slot. `intrinsicWidth` (the source
 * upload's width, which the API returns alongside the URL) clamps the
 * request: asking for more pixels than the original has re-encodes it
 * LARGER than the file we were trying to shrink.
 */
export function mediaThumb(
  url: string,
  width: number,
  intrinsicWidth?: number | null,
): string {
  const key = keyFromUrl(url);
  if (!key) return url;
  // Never ask for more pixels than the source has - upscaling re-encodes it LARGER than
  // the file we set out to shrink
  const want = Math.min(Math.round(width), intrinsicWidth || Infinity);
  return `/media/t/w${Math.round(want)}/${key}`;
}

/**
 * srcset across the fixed width ladder, so a phone doesn't download the
 * desktop rendition. Pair with a `sizes` attribute describing the slot.
 */
export function mediaSrcSet(
  url: string,
  intrinsicWidth?: number | null,
): string | undefined {
  const key = keyFromUrl(url);
  if (!key) return undefined;
  const rungs = WIDTHS.filter((w) => !intrinsicWidth || w < intrinsicWidth).map(
    (w) => `/media/t/w${w}/${key} ${w}w`,
  );
  return rungs.length ? rungs.join(", ") : undefined;
}

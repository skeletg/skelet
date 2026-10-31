// One redaction rule for every path that leaves the browser for the analytics pipeline (Lens page views + session replay). Reset / verify
// links carry a live secret in a path segment, and those batches are stored and replayed by staff

const UUID =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const NUMERIC = /^\d+$/;
// 32 random bytes hex-encoded is the reset-token shape - 20 is well below it and well
// above any hex-looking word
const HEX_RUN = /^[0-9a-f]{20,}$/i;
const OPAQUE_CHARS = /^[A-Za-z0-9_-]{20,}$/;
const TOKEN = ":token";
const ID = ":id";

// A slug is hyphen/underscore-separated lowercase words - a token isn't. Checked before
// the entropy test so long product/blog slugs survive intact
function looksLikeSlug(seg: string): boolean {
  if (!/[-_]/.test(seg)) return false;
  return seg.split(/[-_]/).every((part) => /^[a-z0-9]{1,20}$/.test(part));
}

function isOpaqueToken(seg: string): boolean {
  if (seg.length < 20) return false;
  if (HEX_RUN.test(seg)) return true;
  if (!OPAQUE_CHARS.test(seg)) return false;
  if (looksLikeSlug(seg)) return false;
  const hasDigit = /\d/.test(seg);
  const hasUpper = /[A-Z]/.test(seg);
  const hasLower = /[a-z]/.test(seg);
  // Mixed character classes in one unbroken run - words don't do that
  return (hasDigit && (hasUpper || hasLower)) || (hasUpper && hasLower);
}

// redactPath swaps every identifying segment for a placeholder, so the result is a page
// KEY (groupable in reporting) rather than a URL
export function redactPath(path: string): string {
  const clean = (path || "/").split("?")[0].split("#")[0];
  const out = clean
    .split("/")
    .map((seg) => {
      if (!seg) return seg;
      if (UUID.test(seg) || NUMERIC.test(seg)) return ID;
      if (isOpaqueToken(seg)) return TOKEN;
      return seg;
    })
    .join("/");
  return out || "/";
}

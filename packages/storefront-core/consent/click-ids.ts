// Ad click ids, kept in server-set first-party cookies so a purchase sent server to server can carry the click it came
// from. No Next imports, so the proxy and shopperHeaders share it. Stored only with advertising consent: a click that lands
// before it is allowed is dropped, and withdrawing deletes every one of them on the next request

const DAY = 60 * 60 * 24;

type ClickId = {
  key: string; // the name core reads in X-Click-Ids
  param: string; // the landing-page query parameter
  cookie: string;
  maxAge: number;
  foreign?: boolean; // set by a platform's own pixel, never by us - read and forwarded, deleted on withdrawal, never minted
};

// SYNC: core keeps only the ids apps/backend/core/lib/auth/consent.go lists - skelet check compares them
export const CLICK_IDS: ClickId[] = [
  { key: "gclid", param: "gclid", cookie: "sk_gclid", maxAge: 90 * DAY },
  { key: "gbraid", param: "gbraid", cookie: "sk_gbraid", maxAge: 90 * DAY },
  { key: "wbraid", param: "wbraid", cookie: "sk_wbraid", maxAge: 90 * DAY },
  { key: "msclkid", param: "msclkid", cookie: "sk_msclkid", maxAge: 90 * DAY },
  { key: "ttclid", param: "ttclid", cookie: "sk_ttclid", maxAge: 90 * DAY },
  { key: "oppref", param: "oppref", cookie: "sk_oppref", maxAge: 90 * DAY }, // OpenAI's ad click
  { key: "obref", param: "", cookie: "__obref", maxAge: 0, foreign: true }, // OpenAI's pixel
  { key: "fbc", param: "fbclid", cookie: "_fbc", maxAge: 90 * DAY },
  { key: "fbp", param: "", cookie: "_fbp", maxAge: 90 * DAY },
];

// Click ids are opaque tokens - anything else is someone writing into our cookies through a link
const VALID = /^[\w.~-]{1,255}$/;

export type CookieWrite = {
  name: string;
  value: string; // "" deletes
  options: { path: string; httpOnly: boolean; sameSite: "lax"; secure: boolean; maxAge: number; domain?: string };
};

// The parent domain, so a click on www. still counts at checkout on the bare domain. STOREFRONT_COOKIE_DOMAIN
// names it outright, for a shop whose checkout lives on another subdomain; without it a leading www. is dropped, which
// needs no public-suffix list, and any other host keeps the cookie for itself and below
export function cookieDomain(host: string, configured = process.env.STOREFRONT_COOKIE_DOMAIN): string | undefined {
  if (configured) return configured.replace(/^\./, "");
  const name = host.replace(/:\d+$/, "").toLowerCase();
  if (!name.includes(".") || /^[\d.]+$/.test(name) || name.startsWith("[")) return undefined;
  return name.replace(/^www\./, "");
}

// The cookie writes one page request needs. A new click overwrites the stored one, no click keeps it, and _fbp is minted
// once per browser in Meta's fb.1.<ms>.<random> shape. Without ads consent every click id is deleted
export function clickCookieWrites(
  params: URLSearchParams,
  stored: (name: string) => string | undefined,
  ads: boolean,
  host: string,
  secure: boolean,
  now = Date.now(),
): CookieWrite[] {
  const domain = cookieDomain(host);
  const write = (c: ClickId, value: string): CookieWrite => ({
    name: c.cookie,
    value,
    options: { path: "/", httpOnly: true, sameSite: "lax", secure, maxAge: value ? c.maxAge : 0, ...(domain && { domain }) },
  });
  const out: CookieWrite[] = [];
  for (const c of CLICK_IDS) {
    const current = stored(c.cookie);
    if (!ads) {
      if (current) out.push(write(c, ""));
      continue;
    }
    if (c.foreign) continue;
    if (c.key === "fbp") {
      if (!current) out.push(write(c, `fb.1.${now}.${Math.floor(Math.random() * 1e10)}`));
      continue;
    }
    const clicked = params.get(c.param);
    if (!clicked || !VALID.test(clicked)) continue;
    if (c.key === "fbc") {
      // Meta wants the first-seen time kept while the fbclid stays the same
      if (!current?.endsWith(`.${clicked}`)) out.push(write(c, `fb.1.${now}.${clicked}`));
      continue;
    }
    out.push(write(c, clicked));
  }
  return out;
}

// What core receives in X-Click-Ids: a query string of the stored ids, or "" when there are none. Only with ads consent,
// since on the request that withdraws it the cookies are still here
export function clickIdsHeader(stored: (name: string) => string | undefined, ads: boolean): string {
  if (!ads) return "";
  const q = new URLSearchParams();
  for (const c of CLICK_IDS) {
    const v = stored(c.cookie);
    if (v && VALID.test(v)) q.set(c.key, v);
  }
  return q.toString();
}

// The analytics session: a first-party cookie holding the session's start time in seconds, which is what GA4 takes as
// session_id. It lives 30 minutes from the last page, like GA's own session, and exists only with analytics consent -
// withdrawing deletes it. No Next imports, so the edge proxy can load it

export const SESSION_COOKIE = "sk_sid";
export const SESSION_TTL = 30 * 60;

const VALID = /^\d{9,11}$/;

export function validSession(v: string | undefined | null): v is string {
  return !!v && VALID.test(v);
}

// What the proxy writes on a page request: the session to keep (re-set, which rolls its 30 minutes), a new one, "" to
// delete it, or undefined to leave the cookie alone
export function sessionWrite(current: string | undefined, analytics: boolean, nowMs: number): string | undefined {
  if (!analytics) return current ? "" : undefined;
  return validSession(current) ? current : String(Math.floor(nowMs / 1000));
}

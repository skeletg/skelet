// The dashboard's origin, decided ONCE - it was declared in three places that all have to agree:
// the CSP frame-ancestors for preview routes, and the ev.origin check in each preview client
//
// Fails CLOSED: wrong or unset means preview won't embed, rather than framing from anywhere
export const DASHBOARD_ORIGIN =
  process.env.NEXT_PUBLIC_DASHBOARD_ORIGIN ?? "http://localhost:5173";

// The admin GraphQL endpoint, decided ONCE - four modules each had this literal, and compose warns
// that ADMIN_API_SLUG and VITE_GRAPHQL_URL have to move together
//
// Relative on purpose: the vite proxy (API_PROXY) resolves it, so the same value works on the host
// and in the container
export const ADMIN_GRAPHQL_ENDPOINT = import.meta.env.VITE_GRAPHQL_URL ?? '/api/admin/graphql'

// SVG uploads POST here instead of taking the presigned route - the server has to see the bytes to
// sanitize them (see internal/wiring/media_svg_http.go). Derived from the GraphQL endpoint so the
// two can't drift when ADMIN_API_SLUG moves
export const ADMIN_MEDIA_SVG_ENDPOINT =
  ADMIN_GRAPHQL_ENDPOINT.replace(/\/graphql$/, "") + "/media/svg";

// The Signals API runs as its own service (cmd/signals), so it is a second origin - relative by default like the admin one, and the vite proxy resolves it in dev
export const SIGNALS_GRAPHQL_ENDPOINT =
  import.meta.env.VITE_SIGNALS_GRAPHQL_URL ?? '/api/signals/graphql'

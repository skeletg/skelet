// The admin GraphQL endpoint, decided ONCE - four modules each had this literal, and compose warns
// that ADMIN_API_SLUG and VITE_GRAPHQL_URL have to move together
//
// Relative on purpose: the vite proxy (API_PROXY) resolves it, so the same value works on the host
// and in the container
export const ADMIN_GRAPHQL_ENDPOINT = import.meta.env.VITE_GRAPHQL_URL ?? '/api/admin/graphql'

// The admin's plain HTTP routes sit at a fixed /api/admin on the API's origin, NOT under the GraphQL endpoint's slug - nothing serves /api/admin/<slug>/media/...
const ADMIN_HTTP_BASE = ADMIN_GRAPHQL_ENDPOINT.replace(/\/api\/admin\/.*$/, '') + '/api/admin'

// SVG uploads POST here instead of taking the presigned route - the server has to see the bytes to
// sanitize them (see internal/wiring/media_svg_http.go)
export const ADMIN_MEDIA_SVG_ENDPOINT = ADMIN_HTTP_BASE + '/media/svg'

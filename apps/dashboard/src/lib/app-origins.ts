// Sibling app origins, decided ONCE - the storefront's was read in four modules, two of them under
// a second name (VITE_STOREFRONT_URL), so setting one left the others pointing at localhost
//
// The preview panels' iframe src has to match the storefront's frame-ancestors CSP, which is why
// this is one value rather than per-caller
//
// Trailing slash stripped - SeoPanel did this and the two preview panels did not, so an origin
// ending in "/" built `//cms/preview/...` in the iframe src
export const STOREFRONT_ORIGIN = (
  (import.meta.env.VITE_STOREFRONT_ORIGIN as string | undefined) ?? 'http://localhost:3000'
).replace(/\/$/, '')

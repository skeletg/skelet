import type { NextConfig } from "next";
import createNextIntlPlugin from "next-intl/plugin";
import { BACKEND_INTERNAL_URL } from "@/app/lib/backend-url";
import { DASHBOARD_ORIGIN } from "@/app/lib/dashboard-origin";

// Proxies the backend's token-gated links the browser can't reach directly (app:8080 is
// in-network): /download/:token, /unsubscribe/:token, /e/* pixels and click redirects

// App-layer security headers so the storefront ships a CSP even off the prod edge - static, so it caches across all paths, and no HSTS, which is the edge's concern
// SYNC - narrower on purpose than core's securityheaders.go and the dashboard's nginx template: no gateway SDK runs here, so no host earns a slot

// Dev media is plain http on localhost which `https:` won't cover - prod media is https
const imgSrc =
  process.env.NODE_ENV === "production"
    ? "img-src 'self' data: blob: https:"
    : "img-src 'self' data: blob: https: http:";

// Same-origin only - a payment plugin's browser SDK is what earns a gateway host a slot here
const CSP_BASE = [
  "default-src 'self'",
  "script-src 'self' 'unsafe-inline' 'unsafe-eval'",
  "style-src 'self' 'unsafe-inline'",
  imgSrc,
  "font-src 'self' data:",
  "connect-src 'self'",
  "base-uri 'self'",
  "form-action 'self'",
];

// Common to every route, framing aside
const COMMON_HEADERS = [
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  {
    key: "Permissions-Policy",
    value: "camera=(), microphone=(), geolocation=(), payment=()",
  },
];

// Block cross-origin framing via CSP and legacy X-Frame-Options
const SECURITY_HEADERS = [
  { key: "Content-Security-Policy", value: [...CSP_BASE, "frame-ancestors 'self'"].join("; ") },
  { key: "X-Frame-Options", value: "SAMEORIGIN" },
  ...COMMON_HEADERS,
];

// Dashboard origin allowed to drive the preview iframe - must be set at build time when
// the dashboard has its own host. Fails CLOSED: wrong or unset means preview won't embed

// Preview routes are framed by the dashboard on a different origin, so they scope frame-ancestors instead of using X-Frame-Options, which
// can't name one The message listeners also check ev.origin, so a hostile page can neither frame nor post
const PREVIEW_HEADERS = [
  {
    key: "Content-Security-Policy",
    value: [...CSP_BASE, `frame-ancestors 'self' ${DASHBOARD_ORIGIN}`].join("; "),
  },
  ...COMMON_HEADERS,
];

const nextConfig: NextConfig = {
  // Standalone output so the prod image ships without full node_modules - see Dockerfile.prod
  output: "standalone",

  // Dev-only: next dev 403s /_next chunks requested from any origin but localhost, which broke
  // browsing over the machine's LAN IP. Private-range origins are safe to allow in dev
  allowedDevOrigins: ["192.168.*.*", "10.*.*.*", "172.16.*.*"],

  // Type errors fail the build - no ignoreBuildErrors bypass, keep it that way

  async headers() {
    return [
      { source: "/((?!cms/preview|blog/preview).*)", headers: SECURITY_HEADERS },
      { source: "/cms/preview/:path*", headers: PREVIEW_HEADERS },
      { source: "/blog/preview/:path*", headers: PREVIEW_HEADERS },
    ];
  },

  async rewrites() {
    return [
      {
        // Same-origin so <img src="/media/t/.."> works without exposing the backend host
        source: "/media/t/:path*",
        destination: `${BACKEND_INTERNAL_URL}/media/t/:path*`,
      },
      {
        source: "/download/:path*",
        destination: `${BACKEND_INTERNAL_URL}/download/:path*`,
      },
      {
        source: "/unsubscribe/:path*",
        destination: `${BACKEND_INTERNAL_URL}/unsubscribe/:path*`,
      },
      {
        source: "/e/:path*",
        destination: `${BACKEND_INTERNAL_URL}/e/:path*`,
      },
      {
        // Lens ingest + client config, proxied so the browser sees a same-origin call
        source: "/t/:path*",
        destination: `${BACKEND_INTERNAL_URL}/t/:path*`,
      },
    ];
  },
};

// Wires i18n/request.ts and lets the compiler generate next/root-params for the [locale] segment
const withNextIntl = createNextIntlPlugin();

export default withNextIntl(nextConfig);

import type { NextConfig } from "next";
import createNextIntlPlugin from "next-intl/plugin";
import { existsSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { DASHBOARD_ORIGIN } from "@skelet/storefront-core/dashboard-origin";
import { VIDEO_EMBED_ORIGINS } from "@skelet/storefront-core/video-embed";

// App-layer security headers so the storefront ships a CSP even off the prod edge - static, so it caches across all paths, and no HSTS, which is the edge's concern
// SYNC - narrower on purpose than core's securityheaders.go and the dashboard's nginx template: no gateway SDK runs here, so no host earns a slot

// Dev media is plain http on localhost which `https:` won't cover - prod media is https
const imgSrc =
  process.env.NODE_ENV === "production"
    ? "img-src 'self' data: blob: https:"
    : "img-src 'self' data: blob: https: http:";

// React's dev build evals for fast refresh and error overlays - a prod bundle never does, so it
// doesn't get the slot
const scriptSrc =
  process.env.NODE_ENV === "production"
    ? "script-src 'self' 'unsafe-inline'"
    : "script-src 'self' 'unsafe-inline' 'unsafe-eval'";

// Browser uploads go straight to object storage, a host of its own - UPLOAD_ORIGINS names it at build,
// and dev adds the local Garage and API ports
const connectSrc = [
  "connect-src 'self'",
  process.env.UPLOAD_ORIGINS?.trim(),
  process.env.NODE_ENV === "production" ? "" : "http://localhost:*",
]
  .filter(Boolean)
  .join(" ");

// Video files play from this origin, upload previews (blob:) or https media storage - dev media is plain http on localhost
const mediaSrc =
  process.env.NODE_ENV === "production"
    ? "media-src 'self' blob: https:"
    : "media-src 'self' blob: https: http:";

// Same-origin only - a payment plugin's browser SDK is what earns a gateway host a slot here. The one framed exception is the
// two video players video-embed.ts builds URLs for; who may frame the storefront is frame-ancestors, below
const CSP_BASE = [
  "default-src 'self'",
  scriptSrc,
  "style-src 'self' 'unsafe-inline'",
  imgSrc,
  "font-src 'self' data:",
  mediaSrc,
  `frame-src ${VIDEO_EMBED_ORIGINS.join(" ")}`,
  connectSrc,
  "base-uri 'self'",
  "form-action 'self'",
];

// Common to every route, framing aside
const COMMON_HEADERS = [
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  {
    key: "Permissions-Policy",
    value: "camera=(), microphone=(), geolocation=(self), payment=()",
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

// The pnpm workspace root, which holds the shared packages: Turbopack resolves from it and the standalone build traces
// from it - the two must match - so the server lands at .next/standalone/<this app's path from the root>/server.js.
// Found by walking up, since a template and a storefront made from it sit at different depths
function workspaceRootOf(dir: string): string {
  for (let d = dir; ; d = dirname(d)) {
    if (existsSync(join(d, "pnpm-workspace.yaml"))) return d;
    if (dirname(d) === d) return dir;
  }
}
const workspaceRoot = workspaceRootOf(dirname(fileURLToPath(import.meta.url)));

const nextConfig: NextConfig = {
  // Standalone output so the prod image ships without full node_modules - see Dockerfile.prod
  output: "standalone",
  turbopack: { root: workspaceRoot },
  outputFileTracingRoot: workspaceRoot,

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
};

// Wires i18n/request.ts and lets the compiler generate next/root-params for the [locale] segment
const withNextIntl = createNextIntlPlugin();

export default withNextIntl(nextConfig);

// Every /t/ endpoint the storefront calls. The browser reaches them through the same-origin proxy (backend-proxy.ts)
// with beaconFetch; server code goes straight to the backend with postBeacon or the crawl beacon. Named here rather
// than spelled at each call, so a renamed route is one edit
export const BEACON = {
  collect: "/t/collect",
  replay: "/t/rec",
  config: "/t/config",
  experiments: "/t/experiments",
  crawl: "/t/crawl",
  signupForms: "/t/forms",
  signupSubmit: "/t/forms/submit",
  backInStock: "/t/back-in-stock",
} as const;

export type BeaconPath = (typeof BEACON)[keyof typeof BEACON];

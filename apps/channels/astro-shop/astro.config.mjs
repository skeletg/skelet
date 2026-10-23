import node from "@astrojs/node";
import { defineConfig, memoryCache } from "astro/config";

import react from "@astrojs/react";
import tailwindcss from "@tailwindcss/vite";

// Every page renders on demand, and catalog pages then sit in the route cache until the backend
// purges them through /api/revalidate - static in practice, with no rebuild step
export default defineConfig({
  output: "server",
  adapter: node({ mode: "standalone" }),
  // No cached page reads its query string (checkout and order-placed do, uncached), so none is part of the key -
  // ?srsltid= and friends don't split the cache
  cache: { provider: memoryCache({ query: { include: [] } }) },
  server: { host: true, port: 3000 },

  // The whole stylesheet is a few KB - inlined, it costs no render-blocking request
  build: { inlineStylesheets: "always" },

  prefetch: { prefetchAll: true, defaultStrategy: "hover" },
  integrations: [react()],

  vite: {
    plugins: [tailwindcss()],
  },
});
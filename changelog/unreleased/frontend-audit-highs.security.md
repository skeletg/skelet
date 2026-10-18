- **Next.js 16.3.8**, fixing a server-side request forgery in image optimization, with sharp 0.35.5
  for its bundled librsvg.
- **No high or critical `pnpm audit` findings in either app.** Patched floors pin the flagged
  dependencies (shell-quote, fast-uri, undici, brace-expansion, busboy, source-map-js, proxy-addr,
  the MCP SDK and two GraphQL Tools packages) within their current major lines. `braces` has no fixed
  release and only build tooling reaches it, so it's recorded as a reviewed exception.

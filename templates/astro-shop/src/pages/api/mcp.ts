import type { APIRoute } from "astro";

import { proxyMcp } from "@skelet/storefront-core/mcp";

export const POST: APIRoute = ({ request }) => proxyMcp(request);
export const GET = POST;

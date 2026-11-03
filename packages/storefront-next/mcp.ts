import "./setup";

import { proxyMcp } from "@skelet/storefront-core/mcp";

// The /api/mcp route a template re-exports
export const POST = (req: Request) => proxyMcp(req);
export const GET = POST;

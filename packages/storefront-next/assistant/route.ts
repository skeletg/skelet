import "../setup";

import { getAssistantPage, postAssistant, putAssistantCart } from "@skelet/storefront-core/assistant/route";

// The /api/assistant route a template re-exports
export const POST = (req: Request) => postAssistant(req);
export const PUT = (req: Request) => putAssistantCart(req);
export const GET = (req: Request) => getAssistantPage(req);

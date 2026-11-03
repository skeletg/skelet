import type { APIRoute } from "astro";

// The compose healthcheck's probe - the process is up and serving, nothing more
export const GET: APIRoute = () => new Response("ok", { headers: { "Cache-Control": "no-store" } });

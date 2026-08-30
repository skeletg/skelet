// Liveness probe for the container healthcheck (see docker-compose.prod.yml)
import { NextResponse } from "next/server";

export const dynamic = "force-dynamic";

export function GET() {
  return NextResponse.json({ status: "ok" });
}

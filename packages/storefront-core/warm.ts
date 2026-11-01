import { warmedRedirect } from "./revalidate-payload";

// A purge lands a moment after its request returns, so a warm that still reads the old cached page (HIT) asks again
export async function warmUntilRendered(
  fetchPage: () => Promise<Response>,
  { tries = 20, delayMs = 50 }: { tries?: number; delayMs?: number } = {},
): Promise<boolean> {
  for (let i = 0; i < tries; i++) {
    const res = await fetchPage();
    if (warmedRedirect(res)) return true;
    if (!res.ok) return false;
    if (res.headers.get("x-nextjs-cache") !== "HIT") return true;
    if (i < tries - 1) await new Promise((r) => setTimeout(r, delayMs));
  }
  return true;
}

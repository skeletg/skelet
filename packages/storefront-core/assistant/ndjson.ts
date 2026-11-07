export async function* ndjson(body: ReadableStream<Uint8Array>): AsyncGenerator<Record<string, unknown>> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffered = "";
  let finished = false;
  try {
    for (;;) {
      const { value, done } = await reader.read();
      finished = done;
      buffered += decoder.decode(value, { stream: !done });
      const lines = buffered.split("\n");
      buffered = done ? "" : (lines.pop() ?? "");
      for (const line of lines) {
        const parsed = parse(line);
        if (parsed) yield parsed;
      }
      if (done) return;
    }
  } finally {
    // A reader that stopped early - it had its answer - lets go of the connection rather than leave it half read
    if (!finished) await reader.cancel().catch(() => {});
    reader.releaseLock();
  }
}

function parse(line: string): Record<string, unknown> | null {
  if (line.trim() === "") return null;
  try {
    const v: unknown = JSON.parse(line);
    return v && typeof v === "object" && !Array.isArray(v) ? (v as Record<string, unknown>) : null;
  } catch {
    return null;
  }
}

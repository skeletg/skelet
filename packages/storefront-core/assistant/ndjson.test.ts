import { describe, expect, it } from "vitest";

import { ndjson } from "./ndjson";

function chunked(chunks: Uint8Array[]) {
  let cancelled = false;
  let i = 0;
  const body = new ReadableStream<Uint8Array>({
    pull(controller) {
      if (i < chunks.length) controller.enqueue(chunks[i++]);
      else controller.close();
    },
    cancel() {
      cancelled = true;
    },
  });
  return { body, cancelled: () => cancelled };
}

const bytes = (s: string) => new TextEncoder().encode(s);

describe("ndjson", () => {
  it("joins a line split across chunks, a character split across them too, and a last line with no newline", async () => {
    const all = bytes('{"step":"one"}\n{"text":"Grüße"}\n{"answer":1}');
    const cut = all.indexOf(0xc3) + 1;
    const { body } = chunked([all.slice(0, 5), all.slice(5, cut), all.slice(cut)]);
    const lines = [];
    for await (const line of ndjson(body)) lines.push(line);
    expect(lines).toEqual([{ step: "one" }, { text: "Grüße" }, { answer: 1 }]);
  });

  it("skips what isn't a JSON object", async () => {
    const { body } = chunked([bytes('nope\n[1]\n"str"\n\n{"ok":true}\n')]);
    const lines = [];
    for await (const line of ndjson(body)) lines.push(line);
    expect(lines).toEqual([{ ok: true }]);
  });

  it("lets go of the stream when the reader stops early", async () => {
    const s = chunked([bytes('{"answer":1}\n'), bytes('{"more":2}\n')]);
    for await (const line of ndjson(s.body)) {
      expect(line).toEqual({ answer: 1 });
      break;
    }
    expect(s.cancelled()).toBe(true);
    expect(s.body.locked).toBe(false);
  });
});

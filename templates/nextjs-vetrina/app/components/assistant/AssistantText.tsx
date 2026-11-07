import type { ReactNode } from "react";

import { Link } from "@/app/components/LocaleLink";

// Resolved the way a browser would, so nothing a crafted question slips into a reply links off the store under its name
export function storePath(href: string, storeOrigins: string[]): string | null {
  const origins = storeOrigins.filter((o) => o !== "null");
  if (origins.length === 0) return null;
  let u: URL;
  try {
    u = new URL(href, `${origins[0]}/`);
  } catch {
    return null;
  }
  if ((u.protocol !== "https:" && u.protocol !== "http:") || !origins.includes(u.origin) || u.pathname.startsWith("//")) return null;
  return `${u.pathname}${u.search}${u.hash}`;
}

const TRAILING = /[.,;:!?'"]+$/;

const INLINE = /\*\*([^*]+)\*\*|\[([^\]]+)\]\(([^)\s]+)\)|(https?:\/\/[^\s)]+)/g;

function inline(text: string, origins: string[], key: string): ReactNode[] {
  const out: ReactNode[] = [];
  let last = 0;
  for (const m of text.matchAll(INLINE)) {
    if (m.index > last) out.push(text.slice(last, m.index));
    const k = `${key}-${m.index}`;
    if (m[1] !== undefined) {
      out.push(<strong key={k}>{m[1]}</strong>);
    } else {
      const bare = m[4]?.replace(TRAILING, "");
      const label = m[2] ?? bare!;
      const path = storePath(m[3] ?? bare!, origins);
      out.push(
        path ? (
          <Link key={k} href={path} className="underline underline-offset-2 hover:text-signal">
            {label}
          </Link>
        ) : (
          label
        ),
      );
      if (bare !== undefined && bare.length < m[4].length) out.push(m[4].slice(bare.length));
    }
    last = m.index + m[0].length;
  }
  if (last < text.length) out.push(text.slice(last));
  return out;
}

const BULLET = /^\s*[-*•]\s+/;
const NUMBERED = /^\s*\d+[.)]\s+/;

function blockParts(lines: string[], origins: string[], key: string): ReactNode[] {
  const parts: ReactNode[] = [];
  let i = 0;
  while (i < lines.length) {
    const marker = BULLET.test(lines[i]) ? BULLET : NUMBERED.test(lines[i]) ? NUMBERED : null;
    const start = i;
    if (marker) {
      while (i < lines.length && marker.test(lines[i])) i++;
      const items = lines.slice(start, i).map((l, j) => <li key={j}>{inline(l.replace(marker, ""), origins, `${key}-${start + j}`)}</li>);
      parts.push(
        marker === BULLET ? (
          <ul key={`${key}-${start}`} className="list-disc space-y-1 pl-5">
            {items}
          </ul>
        ) : (
          <ol key={`${key}-${start}`} className="list-decimal space-y-1 pl-5">
            {items}
          </ol>
        ),
      );
      continue;
    }
    while (i < lines.length && !BULLET.test(lines[i]) && !NUMBERED.test(lines[i])) i++;
    parts.push(
      <p key={`${key}-${start}`}>
        {lines.slice(start, i).map((l, j) => (
          <span key={j}>
            {j > 0 && <br />}
            {inline(l.replace(/^#+\s*/, ""), origins, `${key}-${start + j}`)}
          </span>
        ))}
      </p>,
    );
  }
  return parts;
}

// The assistant writes light markdown - paragraphs, lists, bold and links - and that's all this renders, as text
export function AssistantText({ text, storeOrigins }: { text: string; storeOrigins: string[] }) {
  return (
    <div className="space-y-2">
      {text.split(/\n{2,}/).flatMap((block, i) => blockParts(block.split("\n").filter((l) => l.trim() !== ""), storeOrigins, String(i)))}
    </div>
  );
}

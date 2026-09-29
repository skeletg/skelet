import { useTranslations } from "next-intl";

import type { Block } from "./blocks";
import { readString } from "./blocks";

function embedUrl(url: string): string | null {
  try {
    const u = new URL(url);
    const host = u.hostname.replace(/^www\./, "");
    if (host === "youtube.com" || host === "m.youtube.com") {
      const id = u.searchParams.get("v");
      return id ? `https://www.youtube-nocookie.com/embed/${id}` : null;
    }
    if (host === "youtu.be") {
      const id = u.pathname.slice(1);
      return id ? `https://www.youtube-nocookie.com/embed/${id}` : null;
    }
    if (host === "vimeo.com") {
      const id = u.pathname.split("/").filter(Boolean)[0];
      return id ? `https://player.vimeo.com/video/${id}` : null;
    }
    return null;
  } catch {
    return null;
  }
}

export function VideoBlock({ block }: { block: Block }) {
  const t = useTranslations("cms");
  const url = readString(block.props, "url");
  const heading = readString(block.props, "heading");
  if (!url) return null;
  const embed = embedUrl(url);
  const isFile = /\.(mp4|webm|ogg)(\?.*)?$/i.test(url);

  return (
    <section className="border-b border-border">
      <div className="mx-auto w-full max-w-4xl px-6 py-12">
        {heading && (
          <h2 className="mb-4 text-xl font-bold tracking-tight text-foreground">{heading}</h2>
        )}
        <div className="relative aspect-video overflow-hidden rounded-xl border border-border bg-black">
          {embed ? (
            <iframe
              src={embed}
              title={heading || t("video")}
              className="absolute inset-0 h-full w-full"
              allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
              allowFullScreen
            />
          ) : isFile ? (
            <video src={url} controls className="absolute inset-0 h-full w-full" />
          ) : (
            <div className="absolute inset-0 flex items-center justify-center text-sm text-white/70">
              {t("unsupportedVideo")}
            </div>
          )}
        </div>
      </div>
    </section>
  );
}

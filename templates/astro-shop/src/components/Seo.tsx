import type { Channel } from "../lib/channel";
import { jsonLd } from "../lib/seo";

// What a page says about itself - Base adds the channel and the canonical URL
export type SeoProps = {
  title?: string;
  description?: string;
  image?: string | null;
  type?: "website" | "product";
  // Cart, checkout and error pages - kept out of the index, links still followed
  noindex?: boolean;
  structured?: unknown[];
};

type Props = SeoProps & { channel: Channel; canonical: string };

export function Seo({ title, description, image, type = "website", noindex = false, structured = [], channel, canonical }: Props) {
  return (
    <>
      <title>{title ? `${title} · ${channel.name}` : channel.name}</title>
      {description && <meta name="description" content={description} />}
      <link rel="canonical" href={canonical} />
      <meta name="robots" content={noindex ? "noindex, follow" : "index, follow, max-image-preview:large"} />
      <meta property="og:site_name" content={channel.name} />
      <meta property="og:type" content={type} />
      <meta property="og:title" content={title ?? channel.name} />
      {description && <meta property="og:description" content={description} />}
      <meta property="og:url" content={canonical} />
      <meta property="og:locale" content={channel.defaultLocale.replace("-", "_")} />
      {image && <meta property="og:image" content={image} />}
      <meta name="twitter:card" content={image ? "summary_large_image" : "summary"} />
      {structured.map((data, i) => (
        <script key={i} type="application/ld+json" dangerouslySetInnerHTML={{ __html: jsonLd(data) }} />
      ))}
    </>
  );
}

// Server-rendered JSON-LD script tag

const ESCAPE_REGEX = /</g;

export function JsonLd({ data }: { data: unknown }) {
  const json = JSON.stringify(data).replace(ESCAPE_REGEX, "\\u003c");
  return (
    <script
      type="application/ld+json"
      // Crawlers read this; not rendered as DOM text
      dangerouslySetInnerHTML={{ __html: json }}
    />
  );
}

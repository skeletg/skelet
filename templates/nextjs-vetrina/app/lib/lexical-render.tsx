// Lexical -> React for the blog page and the dashboard preview iframe
// The document model - node types, format flags, parsing - is the package's lexical module; this is how vetrina draws it
//
// SYNC SENTINEL - node types must match
// apps/dashboard/src/features/catalog/lexical-blocks/*.tsx

import type { ReactNode } from "react";
import { useTranslations } from "next-intl";
import { videoEmbedUrl } from "@skelet/storefront-core/video-embed";
import {
  lexicalBlocks,
  IS_BOLD,
  IS_ITALIC,
  IS_STRIKETHROUGH,
  IS_UNDERLINE,
  IS_CODE,
  IS_SUBSCRIPT,
  IS_SUPERSCRIPT,
  IS_HIGHLIGHT,
  type Heading,
  type ListNode,
  type ListItem,
  type Text,
  type InlineNode,
  type ProductImage,
  type FAQBlock,
  type StructuredBlock,
  type ProsConsData,
  type ComparisonTableData,
  type TabsData,
  type TestimonialsData,
  type VideoEmbedData,
  type SpecsTableData,
  type ImageTextData,
  type BlockNode,
} from "@skelet/storefront-core/lexical";

/**
 * LexicalRichText renders a Lexical SerializedEditorState (or plain
 * string fallback) as React. Used by /blog/[slug] and the preview
 * iframe.
 */
export function LexicalRichText({ value }: { value: unknown }) {
  const blocks = lexicalBlocks(value);
  if (blocks.length === 0) return null;
  return (
    <>
      {blocks.map((block, i) => (
        <BlockRenderer key={i} node={block} />
      ))}
    </>
  );
}

function BlockRenderer({ node }: { node: BlockNode }) {
  switch (node.type) {
    case "paragraph": {
      const segments = partitionParagraphChildren(node.children);
      if (segments.length === 0) return null;
      if (segments.length === 1 && segments[0].kind === "inline") {
        return (
          <p className="leading-relaxed">
            <InlineChildren nodes={segments[0].nodes} />
          </p>
        );
      }
      return (
        <>
          {segments.map((seg, i) =>
            seg.kind === "inline" ? (
              seg.nodes.length === 0 ? null : (
                <p key={i} className="leading-relaxed">
                  <InlineChildren nodes={seg.nodes} />
                </p>
              )
            ) : (
              <BlockRenderer key={i} node={seg.block} />
            ),
          )}
        </>
      );
    }
    case "heading": {
      // Switch on tag at render time so the rendered element type is statically known
      // to React
      const cls = headingClass(node.tag);
      switch (node.tag) {
        case "h1":
          return (
            <h1 className={cls}>
              <InlineChildren nodes={node.children} />
            </h1>
          );
        case "h2":
          return (
            <h2 className={cls}>
              <InlineChildren nodes={node.children} />
            </h2>
          );
        case "h3":
          return (
            <h3 className={cls}>
              <InlineChildren nodes={node.children} />
            </h3>
          );
        case "h4":
          return (
            <h4 className={cls}>
              <InlineChildren nodes={node.children} />
            </h4>
          );
        case "h5":
          return (
            <h5 className={cls}>
              <InlineChildren nodes={node.children} />
            </h5>
          );
        case "h6":
          return (
            <h6 className={cls}>
              <InlineChildren nodes={node.children} />
            </h6>
          );
      }
      return null;
    }
    case "list":
      return <ListRenderer node={node} />;
    case "quote":
      return (
        <blockquote className="border-l-2 border-border pl-4 italic text-muted-foreground">
          <InlineChildren nodes={node.children} />
        </blockquote>
      );
    case "product-image":
      return <ProductImageBlock node={node} />;
    case "faq-block":
      return <FAQBlockRenderer node={node} />;
    case "structured-block":
      return <StructuredBlockRenderer node={node} />;
  }
}

function headingClass(tag: Heading["tag"]): string {
  switch (tag) {
    case "h1":
      return "mt-8 text-3xl font-semibold tracking-tight";
    case "h2":
      return "mt-8 text-2xl font-semibold tracking-tight";
    case "h3":
      return "mt-6 text-xl font-semibold tracking-tight";
    case "h4":
      return "mt-6 text-lg font-semibold";
    case "h5":
      return "mt-4 text-base font-semibold";
    case "h6":
      return "mt-4 text-sm font-semibold uppercase tracking-wide";
  }
}

function ListRenderer({ node }: { node: ListNode }) {
  if (node.tag === "ol") {
    return (
      <ol
        start={node.start}
        className="ml-6 list-decimal space-y-1 leading-relaxed"
      >
        {node.children.map((item, i) => (
          <ListItemRenderer key={i} node={item} />
        ))}
      </ol>
    );
  }
  return (
    <ul className="ml-6 list-disc space-y-1 leading-relaxed">
      {node.children.map((item, i) => (
        <ListItemRenderer key={i} node={item} />
      ))}
    </ul>
  );
}

function ListItemRenderer({ node }: { node: ListItem }) {
  return (
    <li>
      {node.children.map((child, i) => {
        if (isInlineNode(child)) {
          return <InlineRenderer key={i} node={child} />;
        }
        return <ListRenderer key={i} node={child} />;
      })}
    </li>
  );
}

function isInlineNode(n: InlineNode | ListNode): n is InlineNode {
  return n.type === "text" || n.type === "link" || n.type === "linebreak";
}

type ParagraphSegment =
  | { kind: "inline"; nodes: InlineNode[] }
  | { kind: "block"; block: BlockNode };

function partitionParagraphChildren(
  children: (InlineNode | BlockNode)[],
): ParagraphSegment[] {
  const out: ParagraphSegment[] = [];
  let buf: InlineNode[] = [];
  const flush = () => {
    if (buf.length > 0) {
      out.push({ kind: "inline", nodes: buf });
      buf = [];
    }
  };
  for (const child of children) {
    if (isParagraphInline(child)) {
      buf.push(child);
    } else {
      flush();
      out.push({ kind: "block", block: child });
    }
  }
  flush();
  return out;
}

function isParagraphInline(n: InlineNode | BlockNode): n is InlineNode {
  return n.type === "text" || n.type === "link" || n.type === "linebreak";
}

function InlineChildren({ nodes }: { nodes: InlineNode[] | undefined }) {
  if (!nodes) return null;
  return (
    <>
      {nodes.map((n, i) => (
        <InlineRenderer key={i} node={n} />
      ))}
    </>
  );
}

function InlineRenderer({ node }: { node: InlineNode }) {
  if (node.type === "linebreak") return <br />;
  if (node.type === "link") {
    return (
      <a
        href={node.url}
        title={node.title ?? undefined}
        rel={node.rel ?? "noopener noreferrer"}
        target={node.target ?? undefined}
        className="underline underline-offset-2 hover:text-foreground"
      >
        <InlineChildren nodes={node.children} />
      </a>
    );
  }
  return <FormattedText node={node} />;
}

function FormattedText({ node }: { node: Text }) {
  const f = node.format ?? 0;
  let el: ReactNode = node.text;
  if (f & IS_CODE) {
    el = (
      <code className="rounded bg-muted px-1 py-0.5 font-mono text-[0.95em]">
        {el}
      </code>
    );
  }
  if (f & IS_BOLD) el = <strong>{el}</strong>;
  if (f & IS_ITALIC) el = <em>{el}</em>;
  if (f & IS_UNDERLINE) el = <u>{el}</u>;
  if (f & IS_STRIKETHROUGH) el = <s>{el}</s>;
  if (f & IS_SUBSCRIPT) el = <sub>{el}</sub>;
  if (f & IS_SUPERSCRIPT) el = <sup>{el}</sup>;
  if (f & IS_HIGHLIGHT) {
    el = <mark className="bg-yellow-200 px-0.5">{el}</mark>;
  }
  return <>{el}</>;
}

function ProductImageBlock({ node }: { node: ProductImage }) {
  return (
    <figure className="my-4 overflow-hidden rounded-md border bg-muted/20">
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img
        src={node.src}
        alt={node.altText}
        width={node.width}
        height={node.height}
        className="h-auto max-w-full"
      />
      {node.altText ? (
        <figcaption className="border-t px-3 py-2 text-xs text-muted-foreground">
          {node.altText}
        </figcaption>
      ) : null}
    </figure>
  );
}

function FAQBlockRenderer({ node }: { node: FAQBlock }) {
  if (!node.items || node.items.length === 0) return null;
  return (
    <div className="my-6 space-y-3">
      {node.items.map((item, i) => (
        <details
          key={i}
          className="group rounded-md border border-border bg-card p-3"
        >
          <summary className="cursor-pointer font-medium">
            {item.question}
          </summary>
          <p className="mt-2 whitespace-pre-wrap text-sm leading-relaxed text-muted-foreground">
            {item.answer}
          </p>
        </details>
      ))}
    </div>
  );
}

function StructuredBlockRenderer({ node }: { node: StructuredBlock }) {
  switch (node.blockType) {
    case "pros-cons":
      return <ProsConsBlock data={node.data as ProsConsData} />;
    case "comparison-table":
      return <ComparisonTableBlock data={node.data as ComparisonTableData} />;
    case "tabs":
      return <TabsBlock data={node.data as TabsData} />;
    case "testimonials":
      return <TestimonialsBlock data={node.data as TestimonialsData} />;
    case "video-embed":
      return <VideoEmbedBlock data={node.data as VideoEmbedData} />;
    case "specs-table":
      return <SpecsTableBlock data={node.data as SpecsTableData} />;
    case "image-text":
      return <ImageTextBlock data={node.data as ImageTextData} />;
    default: {
      if (process.env.NODE_ENV !== "production") {
        console.warn(
          "[lexical-render] unknown structured blockType:",
          (node as { blockType?: string }).blockType,
        );
      }
      return null;
    }
  }
}

function ProsConsBlock({ data }: { data: ProsConsData }) {
  const t = useTranslations("cms");
  return (
    <div className="my-6 grid gap-4 sm:grid-cols-2">
      <div className="rounded-md border border-green-300 bg-green-50/50 p-4">
        <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-green-700">
          {t("pros")}
        </div>
        <ul className="ml-5 list-disc space-y-1 text-sm leading-relaxed">
          {data.pros.map((p, i) => (
            <li key={i}>{p}</li>
          ))}
        </ul>
      </div>
      <div className="rounded-md border border-red-300 bg-red-50/50 p-4">
        <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-red-700">
          {t("cons")}
        </div>
        <ul className="ml-5 list-disc space-y-1 text-sm leading-relaxed">
          {data.cons.map((p, i) => (
            <li key={i}>{p}</li>
          ))}
        </ul>
      </div>
    </div>
  );
}

function ComparisonTableBlock({ data }: { data: ComparisonTableData }) {
  return (
    <div className="my-6 overflow-x-auto rounded-md border">
      <table className="w-full border-collapse text-sm">
        <thead className="bg-muted/40">
          <tr>
            {data.columns.map((col, i) => (
              <th
                key={i}
                className="border-b border-border px-3 py-2 text-left font-semibold"
              >
                {col}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {data.rows.map((row, ri) => (
            <tr key={ri} className="odd:bg-background even:bg-muted/20">
              {row.map((cell, ci) => (
                <td key={ci} className="border-b border-border px-3 py-2">
                  {cell}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
function TabsBlock({ data }: { data: TabsData }) {
  if (!data.tabs || data.tabs.length === 0) return null;
  return (
    <div className="my-6 space-y-2">
      {data.tabs.map((tab, i) => (
        <details
          key={i}
          className="rounded-md border border-border bg-card p-3"
          open={i === 0}
        >
          <summary className="cursor-pointer font-medium">{tab.label}</summary>
          <p className="mt-2 whitespace-pre-wrap text-sm leading-relaxed text-muted-foreground">
            {tab.content}
          </p>
        </details>
      ))}
    </div>
  );
}

function TestimonialsBlock({ data }: { data: TestimonialsData }) {
  return (
    <div className="my-6 grid gap-4 sm:grid-cols-2">
      {data.items.map((t, i) => (
        <figure key={i} className="rounded-md border bg-muted/20 p-4">
          <blockquote className="text-sm italic leading-relaxed">
            “{t.quote}”
          </blockquote>
          <figcaption className="mt-3 text-xs text-muted-foreground">
            <span className="font-medium not-italic">{t.author}</span>
            {t.role ? ` · ${t.role}` : null}
          </figcaption>
        </figure>
      ))}
    </div>
  );
}

function VideoEmbedBlock({ data }: { data: VideoEmbedData }) {
  const t = useTranslations("cms");
  const embed = videoEmbedUrl(data.url);
  if (!embed) return null;
  return (
    <figure className="my-6">
      <div className="relative w-full overflow-hidden rounded-md border pt-[56.25%]">
        <iframe
          src={embed}
          title={data.title || t("embeddedVideo")}
          allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
          allowFullScreen
          className="absolute inset-0 h-full w-full"
        />
      </div>
      {data.title ? (
        <figcaption className="mt-2 text-xs text-muted-foreground">
          {data.title}
        </figcaption>
      ) : null}
    </figure>
  );
}
function SpecsTableBlock({ data }: { data: SpecsTableData }) {
  return (
    <dl className="my-6 grid grid-cols-[max-content_1fr] gap-x-6 gap-y-2 text-sm">
      {data.rows.map((r, i) => (
        <div key={i} className="contents">
          <dt className="font-medium text-muted-foreground">{r.label}</dt>
          <dd>{r.value}</dd>
        </div>
      ))}
    </dl>
  );
}

function ImageTextBlock({ data }: { data: ImageTextData }) {
  const imageFirst = data.layout === "image-left";
  return (
    <div className="my-6 grid gap-4 sm:grid-cols-2">
      <div className={imageFirst ? "order-1" : "order-2"}>
        {data.imageUrl ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={data.imageUrl}
            alt={data.imageAlt}
            className="h-auto w-full rounded-md border"
          />
        ) : null}
      </div>
      <div
        className={`${imageFirst ? "order-2" : "order-1"} flex flex-col justify-center`}
      >
        {data.title ? (
          <h3 className="text-xl font-semibold">{data.title}</h3>
        ) : null}
        <p className="mt-2 whitespace-pre-wrap text-sm leading-relaxed text-muted-foreground">
          {data.body}
        </p>
      </div>
    </div>
  );
}

import type { ReactNode } from "react";

import type { Block } from "./blocks";
import { HeroBlockServer } from "./HeroBlockServer";
import { CtaBannerBlock } from "./CtaBannerBlock";
import { HtmlBlock } from "./HtmlBlock";
import { ProductGridBlock } from "./ProductGridBlock";
import { CollectionGridBlock } from "./CollectionGridBlock";
import { CategoryGridBlock } from "./CategoryGridBlock";
import { BlogGridBlock } from "./BlogGridBlock";
import { SectionBlock } from "./SectionBlock";
import { ColumnsBlock } from "./ColumnsBlock";
import { RichTextBlock } from "./RichTextBlock";
import { AccordionBlock } from "./AccordionBlock";
import { TabsBlock } from "./TabsBlock";
import { VideoBlock } from "./VideoBlock";
import { GalleryBlock } from "./GalleryBlock";
import { DividerBlock } from "./DividerBlock";
import { QuoteBlock } from "./QuoteBlock";
import { StatsBlock } from "./StatsBlock";
import { CalloutBlock } from "./CalloutBlock";
import { ButtonsBlock } from "./ButtonsBlock";
import { MediaTextBlock } from "./MediaTextBlock";
import { FeatureGridBlock } from "./FeatureGridBlock";
import { LogosBlock } from "./LogosBlock";
import { TimelineBlock } from "./TimelineBlock";
import { FormBlock } from "./FormBlock";
import { PromoMosaicBlock } from "./PromoMosaicBlock";
import { ValueStripBlock } from "./ValueStripBlock";
import { TrackView } from "./TrackView";

// Renders a block Document via a type-keyed registry - unknown types render nothing, so
// a newer dashboard can author blocks this build doesn't know without blanking the page
// Dynamic *_grid blocks are async server components; static ones render synchronously
//
// SYNC SENTINEL - registry keys are a three-way contract with backend
// internal/blockkit BuiltinTypes and dashboard features/cms, checked by bin/contract-check
// ref is the one backend type with no renderer here - it's listed in that script's RENDERER_GAPS
type BlockRenderer = (props: { block: Block }) => ReactNode | Promise<ReactNode>;

const REGISTRY: Record<string, BlockRenderer> = {
  hero: HeroBlockServer,
  cta_banner: CtaBannerBlock,
  html: HtmlBlock,
  product_grid: ProductGridBlock,
  collection_grid: CollectionGridBlock,
  category_grid: CategoryGridBlock,
  blog_grid: BlogGridBlock,
  section: SectionBlock,
  columns: ColumnsBlock,
  rich_text: RichTextBlock,
  accordion: AccordionBlock,
  tabs: TabsBlock,
  video: VideoBlock,
  gallery: GalleryBlock,
  divider: DividerBlock,
  quote: QuoteBlock,
  stats: StatsBlock,
  callout: CalloutBlock,
  buttons: ButtonsBlock,
  media_text: MediaTextBlock,
  feature_grid: FeatureGridBlock,
  logos: LogosBlock,
  timeline: TimelineBlock,
  form: FormBlock,
  value_strip: ValueStripBlock,
  promo_mosaic: PromoMosaicBlock,
};
function deviceVisibilityClass(props: Record<string, unknown>): string {
  const vis = props?.visibility as { devices?: unknown } | undefined;
  const devices = Array.isArray(vis?.devices)
    ? (vis.devices as unknown[]).filter((d): d is string => typeof d === "string")
    : [];
  if (devices.length === 0) return "";
  const mobile = devices.includes("mobile");
  const desktop = devices.includes("desktop");
  if (mobile && !desktop) return "md:hidden"; // mobile only
  if (desktop && !mobile) return "hidden md:block"; // desktop only
  return ""; // both / neither → always
}

// Each block renders as an element, never a direct call - a renderer calling a hook (useTranslations) inside an
// awaited call is React's "Expected a suspended thenable", which hangs the whole page with no boundary above it
export function CmsBlocks({
  blocks,
  selectable = false,
}: {
  blocks: Block[];
  selectable?: boolean;
}) {
  const rendered = blocks.map((block) => {
    const Renderer = REGISTRY[block.type];
    if (!Renderer) return null;
    const node = <Renderer block={block} />;
    const viewEvent = typeof block.props?.viewEvent === "string" ? block.props.viewEvent : "";
    const inner = viewEvent ? <TrackView event={viewEvent}>{node}</TrackView> : node;
    // Per-block DEVICE visibility (the schedule/segment rules are applied server-side
    // before render; only the device hint reaches us)
    const deviceCls = deviceVisibilityClass(block.props);
    if (!selectable) {
      return (
        <div key={block.id} className={deviceCls || undefined}>
          {inner}
        </div>
      );
    }
    return (
      <div
        key={block.id}
        data-block-id={block.id}
        className={`group/cms relative cursor-pointer outline-offset-[-2px] transition-[outline] hover:outline hover:outline-2 hover:outline-sky-400 ${deviceCls}`}
      >
        {inner}
      </div>
    );
  });
  return <>{rendered}</>;
}

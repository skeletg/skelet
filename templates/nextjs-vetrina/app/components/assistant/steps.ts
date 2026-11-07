import type { AssistantStep } from "@skelet/storefront-core/assistant/types";

const STEPS = {
  search_products: ["steps.search", "steps.searchFor"],
  find_similar_products: ["steps.similar", "steps.similarTo"],
  get_product: ["steps.product", "steps.productNamed"],
  show_product_details: ["steps.writeUp", "steps.writeUpNamed"],
  get_deals: ["steps.deals", "steps.deals"],
  list_categories: ["steps.categories", "steps.categories"],
  search_store_pages: ["steps.pages", "steps.pagesAbout"],
  estimate_shipping: ["steps.delivery", "steps.deliveryTo"],
  get_cart: ["steps.cart", "steps.cart"],
  update_cart: ["steps.updateCart", "steps.updateCart"],
  start_checkout: ["steps.checkout", "steps.checkout"],
  show_products: ["steps.picking", "steps.picking"],
  show_results: ["steps.links", "steps.links"],
  show_page: ["steps.links", "steps.links"],
  get_current_page: ["steps.currentPage", "steps.currentPage"],
  update_listing: ["steps.listing", "steps.listing"],
  get_store_info: ["steps.store", "steps.store"],
  get_my_orders: ["steps.orders", "steps.orders"],
  get_my_order: ["steps.order", "steps.orderNamed"],
  start_return: ["steps.return", "steps.returnFor"],
} as const;

export type StepKey = (typeof STEPS)[keyof typeof STEPS][number];

function regionName(code: string, locale: string): string {
  try {
    return new Intl.DisplayNames([locale], { type: "region" }).of(code) ?? code;
  } catch {
    return code;
  }
}

export function stepLine(step: AssistantStep | null, locale: string): { key: StepKey; detail: string } | null {
  if (!step || !Object.hasOwn(STEPS, step.tool)) return null;
  const [plain, named] = STEPS[step.tool as keyof typeof STEPS];
  const detail = step.tool === "estimate_shipping" && /^[A-Z]{2}$/.test(step.detail) ? regionName(step.detail, locale) : step.detail;
  return detail ? { key: named, detail } : { key: plain, detail: "" };
}

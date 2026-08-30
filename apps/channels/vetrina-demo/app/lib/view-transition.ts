// Shared-element names for the PLP -> PDP morph. Both sides have to agree exactly or React finds no
// pair and the image just cross-fades, so the string is built here rather than spelled out twice
//
// A view-transition-name must be UNIQUE in the document: if two elements claim one, the browser
// skips the whole transition. That is why the name is opt-in per surface - the product grid claims
// it, and the compact rails (recently viewed, related, cart upsells) deliberately do not, since the
// same product can appear in a rail and the grid on one page
export function productImageTransitionName(productId: string): string {
  return `product-image-${productId}`;
}

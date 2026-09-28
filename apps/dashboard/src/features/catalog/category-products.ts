// How a product reads in the category's product list and picker - the resolved name, else the SKU,
// else the id, so a row is never blank
export function productLabel(p: { id: string; name?: string | null; sku?: string | null }): string {
  return p.name || p.sku || p.id
}

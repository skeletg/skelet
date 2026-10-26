export const templatePath = (id: string) => `/catalog/configurator-templates/${id}`

// "3 products · 1 product type" - empty when nothing uses it
export function usedByLabel(t: { productCount: number; productTypeCount: number }) {
  return [
    t.productCount ? `${t.productCount} product${t.productCount === 1 ? '' : 's'}` : null,
    t.productTypeCount ? `${t.productTypeCount} product type${t.productTypeCount === 1 ? '' : 's'}` : null,
  ]
    .filter(Boolean)
    .join(' · ')
}

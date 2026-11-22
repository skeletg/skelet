// Who set a line's price, as the backend names a hook ("plugin:member-prices", "webhook:quotes"), in words -
// empty when it names nobody
export function priceRefLabel(ref: string): string {
  const [kind, ...rest] = ref.split(':')
  const name = rest.join(':')
  if (!name) return ''
  if (kind === 'plugin') return `${name} plugin`
  if (kind === 'webhook') return `${name} webhook`
  return name
}

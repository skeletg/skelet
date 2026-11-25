// Add-ons are order lines bought with another - a protection plan with the item it covers (parentLineId). They are
// refunded on their own, so when their item comes back the dialogs suggest them and leave the choice to staff

type Line = { id: string; quantity: number; parentLineId?: string | null }

export function addOnsOf<T extends Line>(lines: readonly T[], lineId: string): T[] {
  return lines.filter((l) => l.parentLineId === lineId)
}

// How many of the add-on go with that many of its item - the ratio it was sold at, never more than there are
export function addOnQuantity(addOn: Line, item: Line, itemQuantity: number): number {
  if (!(item.quantity > 0) || !(itemQuantity > 0)) return 0
  const q = (addOn.quantity * itemQuantity) / item.quantity
  return Math.min(addOn.quantity, Math.round(q * 10000) / 10000)
}

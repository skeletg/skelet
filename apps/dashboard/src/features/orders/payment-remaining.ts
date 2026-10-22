// What a blank capture/refund amount stands for - the backend's guard sums every non-failed row
// (SumOrderPaymentByKindActive), so pending intents count here too
export function paymentRemaining(
  action: 'capture' | 'refund',
  txns: readonly { kind: string; amount: number; status: string }[],
): number {
  const sum = (kind: string) => txns.reduce((s, t) => (t.kind === kind && t.status !== 'failed' ? s + t.amount : s), 0)
  const remaining = action === 'capture' ? sum('authorize') - sum('capture') : sum('capture') - sum('refund')
  return Math.max(0, remaining)
}

import { CustomerGroupsDocument } from '@/features/customers/CustomerGroups.gql'
import { CountriesPickerDocument } from '@/features/countries/Countries.gql'
import { useState, type ReactNode } from 'react'
import { useLazyQuery, useQuery } from '@apollo/client/react'
import { PlusIcon, Trash2Icon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { SimulatePromotionDocument } from './Promotions.gql'
import { formatCents } from './promoFormat'

// "Why didn't rule X fire?" - runs the promotion engine against a synthetic
// cart and shows the full per-rule trace, without touching a real customer's
// cart - The engine evaluates rules in priority order, so the trace is the
// answer to both "which rule won" and "what stopped the others"

type Line = { productSku: string; quantity: string; unitPriceCents: string }

const EMPTY_LINE: Line = { productSku: '', quantity: '1', unitPriceCents: '' }

function toCents(dollars: string): number {
  const n = Number(dollars)
  return Number.isFinite(n) ? Math.round(n * 100) : 0
}

export function PromotionSimulatorDialog({ trigger }: { trigger: ReactNode }) {
  const countryOptions = useQuery(CountriesPickerDocument).data?.countries ?? []
  const groupOptions = useQuery(CustomerGroupsDocument).data?.customerGroups ?? []
  const [open, setOpen] = useState(false)
  const [subtotal, setSubtotal] = useState('100.00')
  const [countryCode, setCountryCode] = useState('')
  const [groupSlug, setGroupSlug] = useState('')
  const [codes, setCodes] = useState('')
  const [lines, setLines] = useState<Line[]>([{ ...EMPTY_LINE }])

  const [run, { data, loading, error }] = useLazyQuery(SimulatePromotionDocument, {
    fetchPolicy: 'no-cache',
  })

  const result = data?.simulatePromotion

  function simulate() {
    void run({
      variables: {
        input: {
          subtotalCents: toCents(subtotal),
          countryCode: countryCode.trim() || null,
          customerGroupSlug: groupSlug.trim() || null,
          appliedCodes: codes
            .split(',')
            .map((c) => c.trim())
            .filter(Boolean),
          lines: lines
            .filter((l) => l.productSku.trim())
            .map((l) => ({
              productSku: l.productSku.trim(),
              quantity: Number(l.quantity) || 1,
              unitPriceCents: toCents(l.unitPriceCents),
            })),
        },
      },
    })
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>{trigger}</DialogTrigger>
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>Simulate promotions</DialogTitle>
          <DialogDescription>
            Run every active rule against a made-up cart and see which fired, which didn&rsquo;t,
            and why. Nothing is saved and no real cart is touched.
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-4">
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <div className="grid gap-1.5">
              <Label htmlFor="sim-subtotal">Subtotal $</Label>
              <Input
                id="sim-subtotal"
                type="number"
                step="0.01"
                min="0"
                value={subtotal}
                onChange={(e) => setSubtotal(e.target.value)}
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="sim-country">Country</Label>
              <Select value={countryCode || '__none__'} onValueChange={(v) => setCountryCode(v === '__none__' ? '' : v)}>
                <SelectTrigger className="w-full" id="sim-country">
                  <SelectValue placeholder="Country" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="__none__">- Any -</SelectItem>
                  {countryOptions.map((c) => (
                    <SelectItem key={c.code} value={c.code}>
                      {c.emojiFlag ? `${c.emojiFlag} ` : ''}
                      {c.name}
                      <span className="ml-1 text-xs text-muted-foreground">({c.code})</span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="sim-group">Customer group</Label>
              <Select value={groupSlug || '__none__'} onValueChange={(v) => setGroupSlug(v === '__none__' ? '' : v)}>
                <SelectTrigger className="w-full" id="sim-group">
                  <SelectValue placeholder="Customer group" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="__none__">- Any -</SelectItem>
                  {groupOptions.map((g) => (
                    <SelectItem key={g.id} value={g.slug}>
                      {g.name || g.slug}
                      <span className="ml-1 font-mono text-xs text-muted-foreground">{g.slug}</span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="sim-codes">Codes</Label>
              <Input
                id="sim-codes"
                placeholder="SAVE10, FREESHIP"
                value={codes}
                onChange={(e) => setCodes(e.target.value)}
              />
            </div>
          </div>

          <div className="grid gap-2">
            <Label>Cart lines</Label>
            {lines.map((line, i) => (
              <div key={i} className="grid grid-cols-[1fr_5rem_7rem_auto] items-center gap-2">
                <Input
                  placeholder="SKU"
                  className="font-mono"
                  value={line.productSku}
                  onChange={(e) =>
                    setLines((prev) =>
                      prev.map((l, j) => (j === i ? { ...l, productSku: e.target.value } : l)),
                    )
                  }
                />
                <Input
                  type="number"
                  min="1"
                  step="any"
                  value={line.quantity}
                  onChange={(e) =>
                    setLines((prev) =>
                      prev.map((l, j) => (j === i ? { ...l, quantity: e.target.value } : l)),
                    )
                  }
                />
                <Input
                  type="number"
                  step="0.01"
                  min="0"
                  placeholder="unit $"
                  value={line.unitPriceCents}
                  onChange={(e) =>
                    setLines((prev) =>
                      prev.map((l, j) => (j === i ? { ...l, unitPriceCents: e.target.value } : l)),
                    )
                  }
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="size-8"
                  disabled={lines.length === 1}
                  onClick={() => setLines((prev) => prev.filter((_, j) => j !== i))}
                >
                  <Trash2Icon className="size-3.5" />
                </Button>
              </div>
            ))}
            <div>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setLines((prev) => [...prev, { ...EMPTY_LINE }])}
              >
                <PlusIcon className="size-3.5" /> Add line
              </Button>
            </div>
          </div>

          {error ? (
            <Alert variant="destructive">
              <AlertTitle>Simulation failed</AlertTitle>
              <AlertDescription>{error.message}</AlertDescription>
            </Alert>
          ) : null}

          {result ? (
            <div className="grid gap-3 rounded-md border p-4">
              <div className="flex flex-wrap items-center gap-4 text-sm">
                <span>
                  Discount{' '}
                  <strong className="tabular-nums">
                    {formatCents(result.subtotalDiscountCents)}
                  </strong>
                </span>
                {result.freeShipping ? <Badge>Free shipping</Badge> : null}
                <span className="text-muted-foreground">
                  {result.appliedRuleIds.length} rule
                  {result.appliedRuleIds.length === 1 ? '' : 's'} applied
                </span>
              </div>
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b text-left text-xs uppercase tracking-wider text-muted-foreground">
                      <th className="py-2 pr-3 font-medium">Rule</th>
                      <th className="py-2 pr-3 font-medium">Priority</th>
                      <th className="py-2 pr-3 font-medium">Result</th>
                      <th className="py-2 pr-3 font-medium">Discount</th>
                      <th className="py-2 font-medium">Stop</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y">
                    {result.trace.map((t) => (
                      <tr key={t.ruleId}>
                        <td className="py-2 pr-3 font-mono text-xs">{t.ruleSlug}</td>
                        <td className="py-2 pr-3 tabular-nums">{t.priority}</td>
                        <td className="py-2 pr-3">
                          {t.matched ? (
                            <Badge>matched</Badge>
                          ) : (
                            <span className="text-muted-foreground">
                              {t.skipReason || 'no match'}
                            </span>
                          )}
                        </td>
                        <td className="py-2 pr-3 tabular-nums">{formatCents(t.discountCents)}</td>
                        <td className="py-2 text-xs text-muted-foreground">{t.stopAfterApply}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                {result.trace.length === 0 ? (
                  <p className="py-3 text-sm text-muted-foreground">
                    No rules were evaluated - there are no active promotions to test against.
                  </p>
                ) : null}
              </div>
            </div>
          ) : null}
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => setOpen(false)}>
            Close
          </Button>
          <Button type="button" onClick={simulate} disabled={loading}>
            {loading ? 'Running…' : 'Run simulation'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

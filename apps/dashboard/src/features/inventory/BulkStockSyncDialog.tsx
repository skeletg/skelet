import { useState } from 'react'
import { useMutation } from '@apollo/client/react'
import { UploadIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { SyncProductInventoryBatchDocument } from './Inventory.gql'

type ParsedRow = { productSku: string; quantity: number }
type ParseError = { line: number; text: string; reason: string }

// parseRows reads "sku,quantity" lines - A leading header row is tolerated, blank lines are skipped, and every bad row
// is reported with its line number instead of failing the whole paste
function parseRows(csv: string): { rows: ParsedRow[]; errors: ParseError[] } {
  const rows: ParsedRow[] = []
  const errors: ParseError[] = []
  const lines = csv.split(/\r?\n/)
  lines.forEach((raw, i) => {
    const text = raw.trim()
    if (!text) return
    const cells = text.split(',').map((c) => c.trim())
    // tolerate the header spreadsheets add
    if (i === 0 && cells[0].toLowerCase() === 'sku') return
    const [sku, qty] = cells
    if (!sku) {
      errors.push({ line: i + 1, text, reason: 'missing sku' })
      return
    }
    const n = Number(qty)
    if (qty === undefined || qty === '' || !Number.isFinite(n) || n < 0) {
      errors.push({ line: i + 1, text, reason: `invalid quantity ${qty ?? ''}`.trim() })
      return
    }
    rows.push({ productSku: sku, quantity: n })
  })
  return { rows, errors }
}

// Bulk stock update over syncProductInventoryBatch - the endpoint ERP runs use - Partial success is the norm, so
// failures come back per row and are shown alongside the success count rather than thrown away
export function BulkStockSyncDialog({ onSynced }: { onSynced: () => void }) {
  const [open, setOpen] = useState(false)
  const [csv, setCsv] = useState('')
  const [source, setSource] = useState('dashboard')
  const [error, setError] = useState<string | null>(null)
  const [result, setResult] = useState<{
    succeeded: number
    failed: { productSku?: string | null; reason: string }[]
  } | null>(null)
  const [runSync, { loading }] = useMutation(SyncProductInventoryBatchDocument)

  const parsed = parseRows(csv)

  function reset() {
    setCsv('')
    setError(null)
    setResult(null)
  }

  async function submit() {
    setError(null)
    setResult(null)
    try {
      const { data } = await runSync({
        variables: {
          items: parsed.rows.map((r) => ({ ...r, source: source.trim() || null })),
        },
      })
      const res = data?.syncProductInventoryBatch
      if (res) {
        setResult({ succeeded: res.succeeded, failed: res.failed })
        onSynced()
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Sync failed.')
    }
  }

  return (
    <>
      <Button size="sm" variant="outline" onClick={() => setOpen(true)}>
        <UploadIcon className="size-4" />
        Bulk stock update
      </Button>

      <Dialog
        open={open}
        onOpenChange={(v) => {
          setOpen(v)
          if (!v) reset()
        }}
      >
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>Bulk stock update</DialogTitle>
            <DialogDescription>
              One <code className="font-mono">sku,quantity</code> per line - a
              header row is fine. Quantities are absolute counts, not deltas.
              Rows are independent: a bad SKU is reported without blocking the
              rest.
            </DialogDescription>
          </DialogHeader>

          <div className="grid gap-4">
            <Input
              type="file"
              accept=".csv,text/csv"
              onChange={async (e) => {
                const f = e.target.files?.[0]
                if (f) setCsv(await f.text())
              }}
            />
            <div className="grid gap-1.5">
              <Label htmlFor="bulk-stock-csv">Rows</Label>
              <Textarea
                id="bulk-stock-csv"
                value={csv}
                onChange={(e) => setCsv(e.target.value)}
                rows={8}
                placeholder={'sku,quantity\nTEE-CLS-001,25\nHOOD-CORE,12'}
                className="font-mono text-xs"
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="bulk-stock-source">Source label</Label>
              <Input
                id="bulk-stock-source"
                value={source}
                onChange={(e) => setSource(e.target.value)}
                placeholder="dashboard"
              />
              <p className="text-xs text-muted-foreground">
                Stamped on each item as the last sync source, so you can tell
                these apart from an ERP run.
              </p>
            </div>

            {parsed.errors.length > 0 && (
              <Alert variant="destructive">
                <AlertTitle>
                  {parsed.errors.length} line{parsed.errors.length === 1 ? '' : 's'} will be skipped
                </AlertTitle>
                <AlertDescription>
                  <ul className="grid gap-0.5 text-xs">
                    {parsed.errors.slice(0, 5).map((e) => (
                      <li key={e.line}>
                        Line {e.line}: {e.reason} - <span className="font-mono">{e.text}</span>
                      </li>
                    ))}
                    {parsed.errors.length > 5 && <li>…and {parsed.errors.length - 5} more</li>}
                  </ul>
                </AlertDescription>
              </Alert>
            )}

            {error && (
              <Alert variant="destructive">
                <AlertTitle>Sync failed</AlertTitle>
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}

            {result && (
              <Alert variant={result.failed.length > 0 ? 'destructive' : 'default'}>
                <AlertTitle>
                  Updated {result.succeeded} item{result.succeeded === 1 ? '' : 's'}
                  {result.failed.length > 0 ? `, ${result.failed.length} failed` : ''}
                </AlertTitle>
                {result.failed.length > 0 && (
                  <AlertDescription>
                    <ul className="grid gap-0.5 text-xs">
                      {result.failed.map((f, i) => (
                        <li key={`${f.productSku ?? i}`}>
                          <span className="font-mono">{f.productSku ?? '(no sku)'}</span>: {f.reason}
                        </li>
                      ))}
                    </ul>
                  </AlertDescription>
                )}
              </Alert>
            )}
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>
              Close
            </Button>
            <Button onClick={submit} disabled={loading || parsed.rows.length === 0}>
              {loading
                ? 'Updating…'
                : `Update ${parsed.rows.length} item${parsed.rows.length === 1 ? '' : 's'}`}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

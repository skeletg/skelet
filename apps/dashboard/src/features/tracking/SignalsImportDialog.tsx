import { useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { ImportSignalsDestinationsDocument, SignalsImportCandidatesDocument } from './Tracking.gql'

// Copies the old Signals GA4, Meta and webhook destinations here - previewed first, imported switched off and
// without their secrets, which Signals never hands over. Importing changes nothing on the Signals side
export function SignalsImportDialog({ channelId, open, onOpenChange, onImported }: { channelId: string; open: boolean; onOpenChange: (o: boolean) => void; onImported: () => void }) {
  const { data, loading, error } = useQuery(SignalsImportCandidatesDocument, { variables: { channelId }, skip: !open, fetchPolicy: 'network-only' })
  const [run, { loading: importing }] = useMutation(ImportSignalsDestinationsDocument)
  const [picked, setPicked] = useState<string[]>([])
  const [failure, setFailure] = useState<string | null>(null)
  const candidates = data?.signalsImportCandidates ?? []
  const importable = candidates.filter((c) => !c.problem && !c.importedAs)

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Import from Signals</DialogTitle>
          <DialogDescription>
            Each one arrives switched off and needs its secret entered again. The Signals one keeps running until you switch it off there, so
            turn one of the two off once this one works, or events go twice.
          </DialogDescription>
        </DialogHeader>
        {error && <p className="text-sm text-destructive">{error.message}</p>}
        {loading && !data && <p className="text-sm text-muted-foreground">Reading Signals…</p>}
        {data && candidates.length === 0 && <p className="text-sm text-muted-foreground">Signals has no event destinations on this channel.</p>}
        <ul className="grid max-h-[50vh] divide-y overflow-auto rounded-md border">
          {candidates.map((c) => {
            const id = `imp-${c.legacyId}`
            const can = !c.problem && !c.importedAs
            return (
              <li key={c.legacyId} className="grid gap-1 p-3 text-sm">
                <div className="flex items-center gap-2">
                  <Checkbox
                    id={id}
                    disabled={!can}
                    checked={picked.includes(c.legacyId)}
                    onCheckedChange={(on) => setPicked((p) => (on === true ? [...p, c.legacyId] : p.filter((x) => x !== c.legacyId)))}
                  />
                  <label htmlFor={id} className="font-medium">{c.name}</label>
                  <Badge variant="outline">{c.legacyKind}{c.targetKind ? ` → ${c.targetKind}` : ''}</Badge>
                  {c.importedAs && <Badge variant="success">Already imported</Badge>}
                  {!c.signalsActive && <Badge variant="outline">Off in Signals</Badge>}
                </div>
                {c.problem && <p className="text-destructive">{c.problem}</p>}
                {c.config.length > 0 && (
                  <p className="font-mono text-xs text-muted-foreground">{c.config.map((v) => `${v.key}=${v.value}`).join('  ')}</p>
                )}
                {c.droppedFilters.length > 0 && (
                  <p className="text-xs text-muted-foreground">
                    Its event filters don't carry over ({c.droppedFilters.join(', ')}) - it gets the platform's default shop events.
                  </p>
                )}
                {c.targetKind === 'webhook' && can && (
                  <p className="text-xs text-warning">The webhook format changes: one event per POST, signed with X-Webhook-Signature-256.</p>
                )}
              </li>
            )
          })}
        </ul>
        {failure && (
          <Alert variant="destructive">
            <AlertDescription>{failure}</AlertDescription>
          </Alert>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button
            disabled={picked.length === 0 || importing || importable.length === 0}
            onClick={async () => {
              setFailure(null)
              try {
                await run({ variables: { channelId, legacyIds: picked } })
                setPicked([])
                onImported()
                onOpenChange(false)
              } catch (e) {
                setFailure(e instanceof Error ? e.message : 'Import failed')
              }
            }}
          >
            {importing ? 'Importing…' : `Import ${picked.length || ''}`.trim()}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

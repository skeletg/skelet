import { Maximize2Icon, Minimize2Icon, XIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { BUILDERS, KINDS, ReportControls, builderOf } from '@/shared/analytics/kinds'
import type { ReportKind } from '@/shared/analytics/kinds/definition'
import type { DraftReport } from './boardDiff'
import { REPORT_SIZE_LABEL, type BoardReport, type ReportSize } from './report'

type Props = { r: DraftReport; channelId: string; focused: boolean; onChange: (patch: Partial<BoardReport>) => void; onFocus: () => void; onClose: () => void }

// EditPanel builds the tile being edited, beside the grid rather than over it, so the board stays usable and the tile
// shows each change as it's made - the changes are staged on the board like any other edit
export function EditPanel({ r, channelId, focused, onChange, onFocus, onClose }: Props) {
  return (
    <aside aria-label={`Edit ${r.title}`} className="sticky top-0 max-h-[calc(100vh-10rem)] w-[26rem] shrink-0 space-y-4 overflow-y-auto rounded-md border bg-background p-4">
      <div className="flex items-center justify-between gap-2">
        <h2 className="font-semibold">Edit report</h2>
        <div className="flex">
          <Button size="icon" variant="ghost" className="size-7" aria-label={focused ? 'Back to the board' : 'Expand the report'} onClick={onFocus}>
            {focused ? <Minimize2Icon className="size-4" /> : <Maximize2Icon className="size-4" />}
          </Button>
          <Button size="icon" variant="ghost" className="size-7" aria-label="Close the editor" onClick={onClose}>
            <XIcon className="size-4" />
          </Button>
        </div>
      </div>
      <div className="space-y-1">
        <Label htmlFor="tile-title">Title</Label>
        <Input id="tile-title" value={r.title} onChange={(e) => onChange({ title: e.target.value })} />
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-1">
          <Label htmlFor="tile-kind">Report</Label>
          <select
            id="tile-kind"
            value={builderOf(r.definition.kind)}
            onChange={(e) => {
              const { start, title } = KINDS[e.target.value as ReportKind]
              onChange({ title: title(start), definition: start })
            }}
            className="w-full rounded border bg-background px-2 py-1.5 text-sm"
          >
            {BUILDERS.map((b) => (
              <option key={b.kind} value={b.kind}>
                {b.label}
              </option>
            ))}
          </select>
        </div>
        <div className="space-y-1">
          <Label htmlFor="tile-size">Size</Label>
          <select id="tile-size" value={r.size} onChange={(e) => onChange({ size: e.target.value as ReportSize })} className="w-full rounded border bg-background px-2 py-1.5 text-sm">
            {(Object.keys(REPORT_SIZE_LABEL) as ReportSize[]).map((s) => (
              <option key={s} value={s}>
                {REPORT_SIZE_LABEL[s]}
              </option>
            ))}
          </select>
        </div>
      </div>
      <div className="border-t pt-4">
        <ReportControls channelId={channelId} value={r.definition} onChange={(definition) => onChange({ definition })} />
      </div>
      <p className="text-xs text-muted-foreground">Changes show on the tile straight away and are kept when you Save the board. Esc closes this panel.</p>
    </aside>
  )
}

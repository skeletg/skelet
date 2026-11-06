import { reportOf, type ReportDefinition } from './report'
import { Note } from './tiles'

// ReportTile runs one saved report through the tile of its kind
export function ReportTile({ channelId, definition, sinceDays }: { channelId: string; definition: ReportDefinition; sinceDays: number }) {
  const report = reportOf(definition)
  if (!report) return <Note>This report was saved by a newer version and can't be shown here.</Note>
  return <report.Tile channelId={channelId} d={definition} sinceDays={definition.sinceDays || sinceDays} />
}

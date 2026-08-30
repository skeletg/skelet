import { Badge } from '@/components/ui/badge'
import type { DataResourceFieldsFragment } from './Data.gql'

// The reference table an admin builds their file against. Rendered from the resource descriptor, so a new
// backend column documents itself here the moment it is declared
export function ColumnReference({ resource }: { resource: DataResourceFieldsFragment }) {
  return (
    <div className="overflow-hidden rounded-md border">
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead className="border-b bg-muted/40 text-left text-xs uppercase tracking-wide text-muted-foreground">
            <tr>
              <th className="px-3 py-2 font-medium">Column</th>
              <th className="px-3 py-2 font-medium">Type</th>
              <th className="px-3 py-2 font-medium">Notes</th>
              <th className="px-3 py-2 font-medium">Example</th>
            </tr>
          </thead>
          <tbody className="divide-y">
            {resource.fields.map((f) => (
              <tr key={f.name} className="align-top">
                <td className="px-3 py-2">
                  <code className="font-mono text-xs">{f.name}</code>
                  <div className="text-xs text-muted-foreground">{f.label}</div>
                </td>
                <td className="px-3 py-2">
                  <span className="text-xs text-muted-foreground">{f.type}</span>
                </td>
                <td className="px-3 py-2">
                  <div className="flex flex-wrap gap-1">
                    {f.identifier ? (
                      <Badge variant="secondary" className="text-[10px]">
                        upsert key
                      </Badge>
                    ) : null}
                    {f.required ? (
                      <Badge variant="outline" className="text-[10px]">
                        required to create
                      </Badge>
                    ) : null}
                    {f.readOnly ? (
                      <Badge variant="outline" className="text-[10px]">
                        export only
                      </Badge>
                    ) : null}
                    {f.enum.length > 0 ? (
                      <Badge variant="outline" className="text-[10px]">
                        {f.enum.join(' | ')}
                      </Badge>
                    ) : null}
                  </div>
                  {f.description ? (
                    <p className="mt-1 text-xs text-muted-foreground">{f.description}</p>
                  ) : null}
                </td>
                <td className="px-3 py-2">
                  {f.example ? (
                    <code className="font-mono text-xs text-muted-foreground">{f.example}</code>
                  ) : null}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {resource.dynamicPrefixes.length > 0 ? (
        <p className="border-t bg-muted/20 px-3 py-2 text-xs text-muted-foreground">
          Also accepts any column starting with{' '}
          {resource.dynamicPrefixes.map((p) => (
            <code key={p} className="mx-0.5 font-mono">
              {p}
            </code>
          ))}
          - an export fills these in from what the data actually uses.
        </p>
      ) : null}
    </div>
  )
}

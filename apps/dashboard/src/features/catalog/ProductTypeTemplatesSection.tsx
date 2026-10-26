import { useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { useMutation } from '@apollo/client/react'
import { ArrowDownIcon, ArrowUpIcon, LayersIcon, XIcon } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { DetailPageSection } from '@/components/layout/detail-page'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'
import { OptionTemplatesDocument, ProductTypeDocument, SetProductTypeOptionTemplatesDocument } from './Catalog.gql'
import { AddTemplateMenu } from './product-options/TemplateGroup'
import { TemplatesDialog } from './product-options/TemplatesDialog'
import { templatePath } from './product-options/template-path'

type TemplateRef = { id: string; name: string; description: string; optionCount: number; ruleCount: number }

// The templates a product type hands every simple and configurable product of its type - they show first, before
// what a product picks itself. Like attribute assignment, a change applies right away
export function ProductTypeTemplatesSection({
  productType,
  canManage,
}: {
  productType: { id: string; name: string; optionTemplates: readonly TemplateRef[] }
  canManage: boolean
}) {
  const { pathname } = useLocation()
  const templates = productType.optionTemplates
  const ids = templates.map((t) => t.id)
  const [setTemplates, { loading }] = useMutation(SetProductTypeOptionTemplatesDocument, {
    refetchQueries: [{ query: ProductTypeDocument, variables: { id: productType.id } }, { query: OptionTemplatesDocument }],
  })
  const [error, setError] = useState<string | null>(null)
  const [managing, setManaging] = useState(false)
  const [removing, setRemoving] = useState<TemplateRef | null>(null)

  const save = async (next: string[]) => {
    setError(null)
    try {
      await setTemplates({ variables: { productTypeId: productType.id, templateIds: next } })
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not change the templates.')
      throw e
    }
  }
  const swap = (i: number, j: number) => {
    const next = [...ids]
    ;[next[i], next[j]] = [next[j], next[i]]
    void save(next).catch(() => {})
  }

  return (
    <DetailPageSection
      title="Configurator templates"
      description="Every simple and configurable product of this type shows these templates' options first, before the templates and options it has itself, and follows their rules. Changes apply right away."
      trailing={
        canManage ? (
          <AddTemplateMenu usedIds={new Set(ids)} onAdd={(id) => void save([...ids, id]).catch(() => {})} onManage={() => setManaging(true)} />
        ) : null
      }
    >
      <div className="grid gap-3">
        {error ? (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}
        {templates.length === 0 ? (
          <p className="flex items-center gap-2 rounded-md border border-dashed px-4 py-6 text-sm text-muted-foreground">
            <LayersIcon className="size-4 shrink-0" />
            No templates - each product of this type carries only the templates and options it picks itself.
          </p>
        ) : (
          <ul className="divide-y rounded-md border" aria-label="Templates for this type">
            {templates.map((t, i) => (
              <li key={t.id} className="flex items-center gap-2 px-3 py-2.5 sm:px-4">
                {canManage ? (
                  <div className="flex flex-col">
                    <Button
                      type="button" variant="ghost" size="icon" className="size-7" disabled={i === 0 || loading}
                      aria-label={`Show ${t.name} earlier`} onClick={() => swap(i, i - 1)}
                    >
                      <ArrowUpIcon className="size-3.5" />
                    </Button>
                    <Button
                      type="button" variant="ghost" size="icon" className="size-7" disabled={i === templates.length - 1 || loading}
                      aria-label={`Show ${t.name} later`} onClick={() => swap(i, i + 1)}
                    >
                      <ArrowDownIcon className="size-3.5" />
                    </Button>
                  </div>
                ) : null}
                <LayersIcon className="size-4 shrink-0 text-muted-foreground" />
                <div className="grid min-w-0 flex-1">
                  <Link to={templatePath(t.id)} state={{ from: pathname }} className="truncate text-sm font-medium hover:underline">
                    {t.name}
                  </Link>
                  <span className="truncate text-xs text-muted-foreground">
                    {t.optionCount} option{t.optionCount === 1 ? '' : 's'} · {t.ruleCount} rule{t.ruleCount === 1 ? '' : 's'}
                    {t.description ? ` · ${t.description}` : ''}
                  </span>
                </div>
                {canManage ? (
                  <Button
                    type="button" variant="ghost" size="icon" className="size-8 text-muted-foreground hover:text-destructive"
                    aria-label={`Remove ${t.name}`} disabled={loading} onClick={() => setRemoving(t)}
                  >
                    <XIcon className="size-4" />
                  </Button>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </div>
      {canManage ? (
        <TemplatesDialog
          open={managing}
          onOpenChange={setManaging}
          usedIds={new Set(ids)}
          // The dialog says why a use failed - the section behind it stays quiet
          onUse={async (id) => {
            await setTemplates({ variables: { productTypeId: productType.id, templateIds: [...ids, id] } })
          }}
          canCreate
        />
      ) : null}
      <ConfirmDialog
        open={removing !== null}
        onOpenChange={(open) => !open && setRemoving(null)}
        title={`Remove ${removing?.name ?? ''} from ${productType.name}?`}
        description="Its options stop showing on every product of this type that didn't pick the template itself, right away. Refused while one of those products still reads one of its options with its own options or rules."
        confirmLabel="Remove"
        destructive
        onConfirm={async () => {
          if (!removing) return
          await save(ids.filter((id) => id !== removing.id)).catch(() => {})
          setRemoving(null)
        }}
      />
    </DetailPageSection>
  )
}

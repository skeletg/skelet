import { useMemo, useState } from 'react'
import { Link, useLocation, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { LayersIcon, PlusIcon, ShapesIcon, Trash2Icon, XIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { RestoredDraftBanner } from '@/shared/saving/RestoredDraftBanner'
import { useSectionForm } from '@/shared/saving/useSectionForm'
import { SaveRegistryContext, useSaveRegistry } from '@/shared/saving/saveRegistry'
import { usePermissions } from '@/shared/auth/usePermissions'
import { LoadError } from '@/shared/errors/LoadError'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { CurrenciesPickerDocument } from '@/features/currencies/Currencies.gql'
import {
  AssignOptionTemplateDocument,
  DeleteOptionTemplateDocument,
  OptionTemplateDocument,
  OptionTemplateProductsDocument,
  OptionTemplatesDocument,
  UnassignOptionTemplateDocument,
  UpdateOptionTemplateDocument,
  type OptionTemplateQuery,
} from './Catalog.gql'
import { ConfiguratorSection, type ConfiguratorSubject } from './ProductOptionsSection'
import { ProductPickerDialog } from './ProductPickerDialog'
import { usedByLabel } from './product-options/template-path'
import {
  OPTION_TEMPLATE_LABELS,
  optionTemplateDiff,
  optionTemplateSchema,
  toOptionTemplateInput,
  toOptionTemplateValues,
} from './schemas/optionTemplate'

type Template = NonNullable<OptionTemplateQuery['optionTemplate']>

const PAGE = 25
const LIST = '/catalog/configurator-templates'

// Templates open from a product's Configurator or a product type, and Back returns there
function backFrom(state: unknown): { to: string; label: string } {
  const from = (state as { from?: unknown } | null)?.from
  // A path of this app only - the router opens "//host" as another site
  if (typeof from !== 'string' || !from.startsWith('/') || from.startsWith('//')) return { to: LIST, label: 'Back to configurator templates' }
  if (from.startsWith('/products/')) return { to: from, label: 'Back to the product' }
  if (from.startsWith('/catalog/product-types/')) return { to: from, label: 'Back to the product type' }
  return { to: from, label: 'Back' }
}

export function OptionTemplateDetail({ templateId }: { templateId?: string } = {}) {
  const params = useParams<{ id?: string }>()
  const id = templateId ?? params.id ?? ''
  const navigate = useNavigate()
  const back = backFrom(useLocation().state)
  const { has, allowedChannelScopes } = usePermissions()
  // A template reaches products on every store, so only a store-wide catalog manager edits one
  const canManage = has('catalog.manage') && allowedChannelScopes('catalog.manage').allChannels
  const { data, loading, error, refetch } = useQuery(OptionTemplateDocument, { variables: { id }, skip: !id })
  const channelsQuery = useQuery(ChannelsDocument)
  const template = data?.optionTemplate

  const { contextValue, dirtySections, isDirty, saveAll } = useSaveRegistry()
  const [confirmingSave, setConfirmingSave] = useState(false)
  const [savingAll, setSavingAll] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const [deleteTemplate] = useMutation(DeleteOptionTemplateDocument, { refetchQueries: [{ query: OptionTemplatesDocument }] })

  const channels = useMemo(
    () => (channelsQuery.data?.channels ?? []).map((c) => ({ id: c.id, name: c.name, currencyCode: c.currencyCode })),
    [channelsQuery.data],
  )
  // The backend writes new options in the default store's language, like it does a product's
  const defaultLocale = useMemo(
    () => channelsQuery.data?.channels.find((c) => c.isDefault)?.defaultLocale || 'en',
    [channelsQuery.data],
  )
  const subject = useMemo<ConfiguratorSubject | null>(
    () =>
      template
        ? {
            owner: { kind: 'template', id: template.id },
            contentLocale: defaultLocale,
            currency: template.currencyCode,
            channels,
            productOptions: template.productOptions,
            optionRules: template.optionRules,
          }
        : null,
    [template, defaultLocale, channels],
  )

  if (loading && !template) return <p className="text-sm text-muted-foreground">Loading template…</p>
  if (error) {
    return <LoadError title="Couldn't load the template" message={error.message} onRetry={() => void refetch()} retrying={loading} />
  }
  if (!template || !subject) return <p className="text-sm text-muted-foreground">Template not found.</p>
  const inUse = template.productCount > 0 || template.productTypeCount > 0

  return (
    <SaveRegistryContext.Provider value={contextValue}>
      <DetailPageLayout>
        <DetailPageHeader
          backTo={back.to}
          backLabel={back.label}
          title={template.name}
          meta={
            <>
              <span>
                {template.optionCount} option{template.optionCount === 1 ? '' : 's'} · {template.ruleCount} rule
                {template.ruleCount === 1 ? '' : 's'}
              </span>
              <span>·</span>
              <span>{usedByLabel(template) ? `Used by ${usedByLabel(template)}` : 'Not used yet'}</span>
            </>
          }
          actions={
            canManage ? (
              <div className="flex items-center gap-2">
                <span title={inUse ? 'Remove it from every product and product type first' : undefined}>
                  <Button
                    type="button" size="sm" variant="outline" disabled={inUse}
                    onClick={() => setConfirmingDelete(true)}
                  >
                    <Trash2Icon className="size-4" />
                    Delete
                  </Button>
                </span>
                <Button
                  type="button" size="sm" disabled={!isDirty || savingAll}
                  onClick={() => {
                    setSaveError(null)
                    setConfirmingSave(true)
                  }}
                >
                  {savingAll ? 'Saving…' : isDirty ? 'Save changes' : 'No changes'}
                </Button>
              </div>
            ) : null
          }
        />
        <DetailPageBody>
          <GeneralSection key={template.id} template={template} canManage={canManage} />
          <DetailPageSection
            title="Options and rules"
            description="Every product using this template shows these options first, before its own, and follows these rules. An edit here reaches all of them."
          >
            <ConfiguratorSection subject={subject} canManage={canManage} />
          </DetailPageSection>
          <UsedBySection template={template} canManage={canManage} />
        </DetailPageBody>
      </DetailPageLayout>

      <SaveConfirmDialog
        open={confirmingSave}
        scopeLabel={`Configurator template · ${template.name}`}
        sections={dirtySections}
        saving={savingAll}
        error={saveError}
        onCancel={() => {
          if (!savingAll) {
            setConfirmingSave(false)
            setSaveError(null)
          }
        }}
        onConfirm={async () => {
          setSaveError(null)
          setSavingAll(true)
          try {
            await saveAll()
            setConfirmingSave(false)
          } catch (e) {
            setSaveError(e instanceof Error ? e.message : 'Save failed.')
          } finally {
            setSavingAll(false)
          }
        }}
      />
      <ConfirmDialog
        open={confirmingDelete}
        onOpenChange={setConfirmingDelete}
        title={`Delete ${template.name}?`}
        description="This permanently removes the template with its options and rules. No product or product type uses it."
        confirmLabel="Delete template"
        destructive
        onConfirm={async () => {
          await deleteTemplate({ variables: { id: template.id } })
          navigate(back.to, { replace: true })
        }}
      />
    </SaveRegistryContext.Provider>
  )
}

function GeneralSection({ template, canManage }: { template: Template; canManage: boolean }) {
  const [update] = useMutation(UpdateOptionTemplateDocument, {
    refetchQueries: [{ query: OptionTemplatesDocument }, { query: OptionTemplateDocument, variables: { id: template.id } }],
  })
  const currencies = useQuery(CurrenciesPickerDocument).data?.currencies ?? []
  const baseline = useMemo(() => toOptionTemplateValues(template), [template])
  const { form, draft } = useSectionForm({
    id: 'general',
    title: 'General',
    baseline,
    schema: optionTemplateSchema,
    diff: optionTemplateDiff,
    routeKey: `option-template:${template.id}`,
    label: `Configurator template · ${template.name}`,
    submit: async (value) => {
      await update({ variables: { id: template.id, input: toOptionTemplateInput(value) } })
    },
  })
  return (
    <DetailPageSection title="General" description="Only admins see these.">
      {draft.offer !== null ? (
        <div className="mb-4">
          <RestoredDraftBanner draft={draft} labels={OPTION_TEMPLATE_LABELS} />
        </div>
      ) : null}
      <fieldset disabled={!canManage} className="grid gap-4">
        <div className="grid grid-cols-1 items-start gap-4 sm:grid-cols-2">
          <form.AppField name="name">{(f) => <f.TextField id="tpl-name" label="Name" placeholder="Gift wrapping" />}</form.AppField>
          <form.AppField name="currencyCode">
            {(f) => (
              <f.SelectField
                id="tpl-currency"
                label="Currency"
                placeholder="Pick a currency"
                options={currencies.map((c) => ({
                  value: c.code,
                  label: (
                    <>
                      <span className="font-mono text-xs">{c.code}</span> · {c.name}
                    </>
                  ),
                }))}
                hint="A flat charge on its options and rules is in this currency - set per-store amounts for stores selling in another."
              />
            )}
          </form.AppField>
        </div>
        <form.AppField name="description">
          {(f) => <f.TextareaField id="tpl-description" label="Description" rows={2} placeholder="What it's for, which products should use it" />}
        </form.AppField>
      </fieldset>
    </DetailPageSection>
  )
}

function UsedBySection({ template, canManage }: { template: Template; canManage: boolean }) {
  const [page, setPage] = useState(0)
  const variables = { id: template.id, first: PAGE, offset: page * PAGE }
  const { data, loading } = useQuery(OptionTemplateProductsDocument, { variables })
  const refetchQueries = [
    { query: OptionTemplateProductsDocument, variables },
    { query: OptionTemplateDocument, variables: { id: template.id } },
    { query: OptionTemplatesDocument },
  ]
  const [assign] = useMutation(AssignOptionTemplateDocument, { refetchQueries })
  const [unassign] = useMutation(UnassignOptionTemplateDocument, { refetchQueries })
  const [picking, setPicking] = useState(false)
  const [removing, setRemoving] = useState<{ id: string; name: string } | null>(null)
  const products = data?.optionTemplate?.products ?? []
  const total = data?.optionTemplate?.productCount ?? template.productCount
  const pages = Math.max(1, Math.ceil(total / PAGE))

  return (
    <DetailPageSection
      title="Used by"
      description="Products showing this template's options - picked one by one, or handed over by their product type. Adding or removing a product applies right away."
      trailing={
        canManage ? (
          <Button type="button" size="sm" variant="outline" onClick={() => setPicking(true)}>
            <PlusIcon className="size-4" />
            Add products
          </Button>
        ) : null
      }
    >
      {template.productTypes.length > 0 ? (
        <div className="mb-3 grid gap-1.5">
          <p className="text-xs font-medium text-muted-foreground">Every simple and configurable product of</p>
          <ul className="divide-y rounded-md border" aria-label="Product types handing it over">
            {template.productTypes.map((pt) => (
              <li key={pt.id} className="flex items-center gap-3 px-4 py-2">
                <ShapesIcon className="size-4 shrink-0 text-muted-foreground" />
                <Link to={`/catalog/product-types/${pt.id}`} className="min-w-0 truncate text-sm font-medium hover:underline">
                  {pt.name}
                </Link>
                <span className="ml-auto text-xs text-muted-foreground">Change it on the type</span>
              </li>
            ))}
          </ul>
          {total > 0 ? <p className="pt-2 text-xs font-medium text-muted-foreground">And these products, picked one by one</p> : null}
        </div>
      ) : null}
      {total === 0 && !loading ? (
        template.productTypes.length > 0 ? null : (
          <p className="flex items-center gap-2 rounded-md border border-dashed px-4 py-6 text-sm text-muted-foreground">
            <LayersIcon className="size-4" />
            No product uses it yet. Add products here, or give it to a product type for all of its products.
          </p>
        )
      ) : (
        <div className="grid gap-2">
          <ul className="divide-y rounded-md border" aria-label="Products using the template">
            {products.map((p) => (
              <li key={p.id} className="flex items-center gap-3 px-4 py-2">
                {p.thumbnail?.url ? (
                  <img src={p.thumbnail.url} alt={p.thumbnail.altText ?? ''} className="size-8 rounded object-cover" />
                ) : (
                  <span className="size-8 rounded bg-muted" aria-hidden />
                )}
                <div className="grid min-w-0 flex-1">
                  <Link to={`/products/${p.id}`} className="truncate text-sm font-medium hover:underline">
                    {p.name || 'Untitled product'}
                  </Link>
                  <span className="font-mono text-xs text-muted-foreground">{p.sku}</span>
                </div>
                {canManage ? (
                  <Button
                    type="button" variant="ghost" size="icon" className="size-8 text-muted-foreground hover:text-destructive"
                    aria-label={`Remove ${p.name || p.sku}`} onClick={() => setRemoving({ id: p.id, name: p.name || p.sku })}
                  >
                    <XIcon className="size-4" />
                  </Button>
                ) : null}
              </li>
            ))}
          </ul>
          {pages > 1 ? (
            <div className="flex items-center justify-end gap-2 text-xs text-muted-foreground">
              <span>
                Page {page + 1} of {pages}
              </span>
              <Button type="button" size="sm" variant="outline" disabled={page === 0} onClick={() => setPage(page - 1)}>
                Previous
              </Button>
              <Button type="button" size="sm" variant="outline" disabled={page + 1 >= pages} onClick={() => setPage(page + 1)}>
                Next
              </Button>
            </div>
          ) : null}
        </div>
      )}
      {canManage ? (
        <ProductPickerDialog
          open={picking}
          onOpenChange={setPicking}
          title={`Use ${template.name} on products`}
          description="Its options show first on each picked product, before the product's own. Applies right away."
          kinds={['SIMPLE', 'CONFIGURABLE']}
          confirmLabel={(n) => (n === 0 ? 'Add products' : `Use on ${n} product${n === 1 ? '' : 's'}`)}
          onConfirm={async (picks) => {
            try {
              await assign({ variables: { templateId: template.id, productIds: picks.map((p) => p.product.id) } })
            } catch (e) {
              return { error: e instanceof Error ? e.message : 'Could not add the products.' }
            }
          }}
        />
      ) : null}
      <ConfirmDialog
        open={removing !== null}
        onOpenChange={(open) => !open && setRemoving(null)}
        title={`Remove ${template.name} from ${removing?.name ?? ''}?`}
        description="Its options stop showing on that product right away. Refused while the product's own options or rules still read one of them."
        confirmLabel="Remove"
        destructive
        onConfirm={async () => {
          if (!removing) return
          await unassign({ variables: { templateId: template.id, productIds: [removing.id] } })
          setRemoving(null)
        }}
      />
    </DetailPageSection>
  )
}

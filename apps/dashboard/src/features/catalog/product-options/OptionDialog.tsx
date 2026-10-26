import { useMemo, useState, type ReactNode } from 'react'
import { useMutation } from '@apollo/client/react'
import type { DocumentNode } from 'graphql'
import { ChevronDownIcon, LanguagesIcon } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { TranslationLocales } from '@/components/TranslationLocales'
import { cn } from '@/lib/utils'
import { ChannelSurchargeInputs } from '../ChannelSurchargeInputs'
import { DeleteProductOptionContentDocument } from '../Catalog.gql'
import { isTemp, parseVisibleIfGroup, type PendingOp, type ProductOption } from '../productOptionsStaging'
import { ChoicesEditor, PriceInput } from './ChoicesEditor'
import { ConditionEditor } from './ConditionEditor'
import { OptionPicture } from './OptionPicture'
import { draftFromOption, draftToOps, parsePrice, validateDraft, type DraftContext, type OptionDraft } from './option-draft'
import { INPUT_TYPES, TYPE_GROUPS, isChoice, isSingleChoice } from './option-types'

// What a write refetches - the product or template page's own query
export type RefetchQuery = { query: DocumentNode; variables?: Record<string, unknown> }

type DialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  // The option being edited, or null to create one
  option: ProductOption | null
  // Every option as staged, in order - what conditions can point at and what points back
  options: ProductOption[]
  // The product being edited, so a choice can't link back to it - empty on a template
  productId: string
  refetchQueries: RefetchQuery[]
  locale: string
  defaultLocale: string
  multilingual: boolean
  nextSortOrder: number
  ctx: DraftContext
  // What a choice is used by - conditions and rules that would point at nothing if it went
  usedBy: (valueId: string) => string[]
  onDone: (ops: PendingOp[]) => void
}

export function OptionDialog(props: DialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="flex max-h-[92vh] flex-col gap-0 p-0 sm:max-w-[min(94vw,80rem)]">
        {/* Remounted per open, so a cancelled draft never comes back */}
        {props.open ? <OptionDialogBody key={`${props.option?.id ?? 'new'}:${props.locale}`} {...props} /> : null}
      </DialogContent>
    </Dialog>
  )
}

function Block({ title, hint, children }: { title: string; hint?: ReactNode; children: ReactNode }) {
  return (
    <section className="grid gap-3">
      <div className="grid gap-0.5">
        <h3 className="text-sm font-semibold">{title}</h3>
        {hint ? <p className="text-xs text-muted-foreground">{hint}</p> : null}
      </div>
      {children}
    </section>
  )
}

function OptionDialogBody({ onOpenChange, option, options, productId, refetchQueries, locale, defaultLocale, multilingual, nextSortOrder, ctx, usedBy, onDone }: DialogProps) {
  // A new option is always written in the default language - the backend creates the first translation there
  const editLocale = option ? locale : defaultLocale
  const translating = !!option && locale !== defaultLocale
  // An unsaved option has no row to hang a translation on yet
  const frozen = translating && isTemp(option.id)
  const [initial] = useState(() => draftFromOption(option, editLocale, ctx))
  const [draft, setDraft] = useState<OptionDraft>(initial)
  const [attempted, setAttempted] = useState(false)
  const [deleteContent] = useMutation(DeleteProductOptionContentDocument, { refetchQueries })

  const set = (p: Partial<OptionDraft>) => setDraft((d) => ({ ...d, ...p }))
  const errors = attempted ? validateDraft(draft, ctx) : null
  const choice = isChoice(draft.inputType)
  const family = INPUT_TYPES[draft.inputType].family
  const cur = ctx.baseCurrency
  const nested = (parseVisibleIfGroup(option?.visibleIf)?.groups?.length ?? 0) > 0

  // Conditions point only at options listed above this one - the backend's order rule, which keeps them cycle-free
  const earlier = useMemo(() => {
    const at = option ? options.findIndex((o) => o.id === option.id) : options.length
    return options.slice(0, at < 0 ? options.length : at)
  }, [options, option])
  const lockedTitles = useMemo(
    () => new Set(translating ? draft.choices.filter((c) => !c.id || isTemp(c.id) || frozen).map((c) => c.key) : []),
    [draft.choices, translating, frozen],
  )

  function done() {
    if (validateDraft(draft, ctx)) {
      setAttempted(true)
      return
    }
    onDone(draftToOps({ draft, initial, option, locale: editLocale, contentLocale: defaultLocale, sortOrder: nextSortOrder, ctx }))
    onOpenChange(false)
  }

  const title = initial.title.trim() || 'Untitled option'
  const langTag = multilingual ? <span className="font-normal text-muted-foreground">({editLocale})</span> : null

  return (
    <>
      <DialogHeader className="border-b px-6 py-4">
        <DialogTitle>{option ? title : 'New option'}</DialogTitle>
        <DialogDescription>Something customers choose or fill in when they add this product to the cart.</DialogDescription>
      </DialogHeader>

      <div className="min-h-0 flex-1 overflow-y-auto px-6 py-5">
        <div className="grid gap-6">
          {translating ? (
            <Alert>
              <LanguagesIcon className="size-4" />
              <AlertDescription>
                {frozen
                  ? `This option isn't saved yet. Save the page first, then switch to ${locale} to translate it.`
                  : `Translating to ${locale}. Only names and help text change here - switch the language back to ${defaultLocale} to change prices, choices or conditions.`}
              </AlertDescription>
            </Alert>
          ) : null}

          <div className="grid gap-8 lg:grid-cols-[minmax(0,23rem)_minmax(0,1fr)] lg:gap-10">
            <div className="grid content-start gap-8">
              <Block title="Basics">
                <div className="grid gap-1.5">
                  <Label htmlFor="option-title">Name shown to customers {langTag}</Label>
                  <Input
                    id="option-title" value={draft.title} placeholder="e.g. Gift wrapping" disabled={frozen} aria-invalid={!!errors?.title || undefined}
                    onChange={(e) => set({ title: e.target.value })}
                  />
                  {errors?.title ? <p className="text-xs text-destructive">{errors.title}</p> : null}
                </div>
                <div className="grid gap-1.5">
                  <Label htmlFor="option-help">Help text {langTag}</Label>
                  <Textarea
                    id="option-help" rows={2} value={draft.helpText} disabled={frozen} placeholder="Optional - shown under the option, e.g. Up to 20 letters"
                    onChange={(e) => set({ helpText: e.target.value })}
                  />
                </div>
                <div className="flex items-start gap-3">
                  <OptionPicture label={draft.title.trim() || 'Option'} value={draft.image} disabled={translating} onChange={(image) => set({ image })} />
                  <div className="grid gap-0.5 pt-1">
                    <p className="text-sm font-medium">Picture</p>
                    <p className="text-xs text-muted-foreground">
                      Optional - shown beside the option on the product page, like where an engraving goes. Choices can each have one too.
                    </p>
                  </div>
                </div>
                <div className="flex items-start justify-between gap-4 rounded-md border p-3">
                  <div className="grid gap-0.5">
                    <Label htmlFor="option-required">Required</Label>
                    <p className="text-xs text-muted-foreground">Customers can't add the product to the cart without answering.</p>
                  </div>
                  <Switch id="option-required" checked={draft.isRequired} disabled={translating} onCheckedChange={(on) => set({ isRequired: on })} />
                </div>
              </Block>

              <Block
                title="How customers answer"
                hint={option && !translating ? 'You can switch between similar kinds. To go between choices and typed answers, add a new option instead.' : undefined}
              >
                {TYPE_GROUPS.map((g) => (
                  <div key={g.title} className="grid gap-2">
                    <p className="text-xs font-medium text-muted-foreground">{g.title}</p>
                    <div role="radiogroup" aria-label={g.title} className="grid gap-2 sm:grid-cols-2">
                      {g.types.map((t) => {
                        const info = INPUT_TYPES[t]
                        const selected = draft.inputType === t
                        const allowed = !translating && (!option || info.family === INPUT_TYPES[option.inputType].family)
                        return (
                          <button
                            key={t} type="button" role="radio" aria-checked={selected} disabled={!allowed && !selected}
                            onClick={() => {
                              if (!allowed) return
                              // Leaving a multi-pick kind keeps only the first pre-selected choice
                              const first = draft.choices.findIndex((c) => c.isDefault)
                              set({
                                inputType: t,
                                ...(isSingleChoice(t) ? { choices: draft.choices.map((c, i) => ({ ...c, isDefault: c.isDefault && i === first })) } : {}),
                              })
                            }}
                            className={cn(
                              'flex items-start gap-3 rounded-md border px-3 py-2.5 text-left transition-colors',
                              selected ? 'border-foreground/50 bg-muted' : 'hover:bg-muted/50',
                              !allowed && !selected && 'cursor-not-allowed opacity-50',
                            )}
                          >
                            <info.Icon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                            <span className="grid gap-0.5">
                              <span className="text-sm font-medium">{info.label}</span>
                              <span className="text-xs text-muted-foreground">{info.hint}</span>
                            </span>
                          </button>
                        )
                      })}
                    </div>
                  </div>
                ))}
                {family === 'text' ? (
                  <div className="grid gap-1.5 sm:max-w-xs">
                    <Label htmlFor="option-max-chars">Character limit</Label>
                    <Input id="option-max-chars" inputMode="numeric" placeholder="No limit" value={draft.maxChars} disabled={translating} aria-invalid={!!errors?.maxChars || undefined} onChange={(e) => set({ maxChars: e.target.value })} />
                    {errors?.maxChars ? <p className="text-xs text-destructive">{errors.maxChars}</p> : null}
                  </div>
                ) : null}
                {draft.inputType === 'FILE' ? (
                  <div className="grid gap-3 sm:grid-cols-2">
                    <div className="grid gap-1.5">
                      <Label htmlFor="option-max-mb">Largest file (MB)</Label>
                      <Input id="option-max-mb" inputMode="decimal" placeholder="No limit" value={draft.maxFileMb} disabled={translating} aria-invalid={!!errors?.maxFileMb || undefined} onChange={(e) => set({ maxFileMb: e.target.value })} />
                      {errors?.maxFileMb ? <p className="text-xs text-destructive">{errors.maxFileMb}</p> : null}
                    </div>
                    <div className="grid gap-1.5">
                      <Label htmlFor="option-extensions">Allowed file types</Label>
                      <Input id="option-extensions" placeholder="Any, or e.g. jpg, png, pdf" value={draft.fileExtensions} disabled={translating} onChange={(e) => set({ fileExtensions: e.target.value })} />
                    </div>
                  </div>
                ) : null}
              </Block>
            </div>

            <div className="grid content-start gap-8">
              {choice ? (
                <Block title="Choices" hint="The extra charge is added to the product's price when the choice is picked. Leave it empty for no charge.">
                  <ChoicesEditor
                    choices={draft.choices}
                    single={isSingleChoice(draft.inputType)}
                    productId={productId}
                    ctx={ctx}
                    translating={translating}
                    lockedTitles={lockedTitles}
                    usedBy={usedBy}
                    errors={errors?.choice ?? {}}
                    listError={errors?.choices}
                    onChange={(choices) => set({ choices })}
                  />
                </Block>
              ) : (
                <Block title="Extra charge" hint="Added to the product's price when the customer uses this option. Leave it empty for no charge.">
                  <div className="grid gap-1.5 sm:max-w-xs">
                    <PriceInput
                      label="Extra charge" value={draft.price} type={draft.priceType} currency={cur} disabled={translating} invalid={!!errors?.price}
                      onValue={(price) => set({ price })} onType={(priceType) => set({ priceType })}
                    />
                    {errors?.price ? <p className="text-xs text-destructive">{errors.price}</p> : null}
                  </div>
                  {draft.priceType === 'FIXED' && draft.price.trim() !== '' && ctx.channels.length > 0 ? (
                    <Collapsible defaultOpen={Object.values(initial.channelPrices).some((v) => v.trim() !== '') || ctx.channels.some((c) => c.currencyCode !== cur)}>
                      <CollapsibleTrigger className="group/stores flex items-center gap-1 text-sm font-medium">
                        <ChevronDownIcon className="size-4 transition-transform group-data-[state=closed]/stores:-rotate-90" />
                        Different charge per store
                      </CollapsibleTrigger>
                      <CollapsibleContent className="pt-2">
                        <ChannelSurchargeInputs
                          idPrefix="option-store" channels={ctx.channels} baseCurrency={cur} baseAmount={parsePrice(draft.price, 'FIXED', cur) ?? 0}
                          values={draft.channelPrices} disabled={translating}
                          onChange={(id, raw) => set({ channelPrices: { ...draft.channelPrices, [id]: raw } })}
                        />
                        {errors?.channels ? <p className="pt-1 text-xs text-destructive">{errors.channels}</p> : null}
                      </CollapsibleContent>
                    </Collapsible>
                  ) : null}
                </Block>
              )}

              <Block title="When to show it" hint="Hidden options are never required and never charged.">
                <ConditionEditor
                  earlier={earlier}
                  isNew={!option}
                  locale={locale}
                  logic={draft.logic}
                  rules={draft.rules}
                  nested={nested}
                  disabled={translating}
                  error={errors?.rules}
                  onChange={(p) => set(p)}
                />
              </Block>

              {translating && !frozen ? (
                <TranslationLocales
                  locale={locale}
                  locales={(option.content ?? []).map((c) => c.locale)}
                  entityLabel="option"
                  canManage
                  defaultLocaleLocked
                  onRemove={async () => {
                    await deleteContent({ variables: { optionId: option.id, locale } })
                  }}
                />
              ) : null}
            </div>
          </div>
        </div>
      </div>

      <DialogFooter className="mx-0 mb-0 flex-row items-center justify-end gap-2 px-6 py-4">
        {errors ? <p className="mr-auto text-sm text-destructive">Fix the highlighted fields first.</p> : null}
        <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
        <Button type="button" onClick={done} disabled={frozen}>{option ? 'Done' : 'Add option'}</Button>
      </DialogFooter>
    </>
  )
}

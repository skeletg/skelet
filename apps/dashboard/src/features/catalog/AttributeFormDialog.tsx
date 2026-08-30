import { useState, type FormEvent, type ReactNode } from 'react'
import { useMutation } from '@apollo/client/react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { slugify } from '@/lib/slug'
import type { AttributeInputType, AttributeReferenceTarget } from '@/graphql/schema.types'
import { INPUT_TYPE_LABEL } from './attributeInputTypes'
import {
  AttributeDocument,
  AttributesDocument,
  CreateAttributeDocument,
  UpdateAttributeDocument,
} from './Catalog.gql'

export type ExistingAttribute = {
  id: string
  slug: string
  name: string
  inputType: AttributeInputType
  relationshipTarget?: AttributeReferenceTarget | null
  unit: string
  valueRequired: boolean
  filterable: boolean
  searchable: boolean
  isPublic: boolean
  facetCollapsed: boolean
  isIndexable: boolean
  h1Position: 'BEFORE' | 'AFTER'
  sortOrder: number
}

const SCALAR_INPUT_TYPES: AttributeInputType[] = ['PLAIN_TEXT', 'NUMERIC', 'BOOLEAN', 'DATE', 'DATE_TIME']

function isScalarInputType(t: AttributeInputType): boolean {
  return SCALAR_INPUT_TYPES.includes(t)
}

const INPUT_TYPE_OPTIONS: AttributeInputType[] = [
  'PLAIN_TEXT',
  'NUMERIC',
  'BOOLEAN',
  'DATE',
  'DATE_TIME',
  'DROPDOWN',
  'MULTISELECT',
  'SWATCH',
  'REFERENCE',
]

// Entity types a REFERENCE attribute can point at (mirrors the backend
// AttributeReferenceTarget enum)
const REFERENCE_TARGET_OPTIONS: AttributeReferenceTarget[] = ['PRODUCT', 'CATEGORY', 'COLLECTION']

export function AttributeFormDialog({
  attribute,
  trigger,
  open,
  onOpenChange,
}: {
  attribute?: ExistingAttribute
  trigger?: ReactNode
  open?: boolean
  onOpenChange?: (open: boolean) => void
}) {
  const isEdit = !!attribute
  const [internalOpen, setInternalOpen] = useState(false)
  const actualOpen = open ?? internalOpen
  const setOpen = onOpenChange ?? setInternalOpen

  const [slug, setSlug] = useState('')
  const [slugTouched, setSlugTouched] = useState(false)
  const [name, setName] = useState('')
  const [inputType, setInputType] = useState<AttributeInputType>('PLAIN_TEXT')
  const [relationshipTarget, setRelationshipTarget] = useState<AttributeReferenceTarget>('PRODUCT')
  const [unit, setUnit] = useState('')
  const [valueRequired, setValueRequired] = useState(false)
  const [filterable, setFilterable] = useState(false)
  const [searchable, setSearchable] = useState(false)
  const [isPublic, setIsPublic] = useState(true)
  const [facetCollapsed, setFacetCollapsed] = useState(false)
  const [isIndexable, setIsIndexable] = useState(false)
  const [h1Position, setH1Position] = useState<'BEFORE' | 'AFTER'>('BEFORE')
  const [sortOrder, setSortOrder] = useState(0)
  // Default value is scalar-only here (selectable defaults need their AttributeValue rows, which don't exist at create
  // time - set them on the detail page after the attribute is saved)
  const [defaultValueRaw, setDefaultValueRaw] = useState('')
  const [defaultBool, setDefaultBool] = useState(false)
  const [hasDefault, setHasDefault] = useState(false)
  const [submitError, setSubmitError] = useState<string | null>(null)

  // Reset the form from the source attribute (or to blanks) when the dialog opens or the attribute changes while open - Adjusted during
  // render (React's "adjust state on prop change" pattern); the null sentinel covers dialogs that mount already open
  const [prevOpenState, setPrevOpenState] = useState<
    { open: boolean; attribute: ExistingAttribute | undefined } | null
  >(null)
  if (
    prevOpenState === null ||
    prevOpenState.open !== actualOpen ||
    prevOpenState.attribute !== attribute
  ) {
    setPrevOpenState({ open: actualOpen, attribute })
    if (actualOpen) {
      setSlug(attribute?.slug ?? '')
      setSlugTouched(false)
      setName(attribute?.name ?? '')
      setInputType(attribute?.inputType ?? 'PLAIN_TEXT')
      setRelationshipTarget(attribute?.relationshipTarget ?? 'PRODUCT')
      setUnit(attribute?.unit ?? '')
      setValueRequired(attribute?.valueRequired ?? false)
      setFilterable(attribute?.filterable ?? false)
      setSearchable(attribute?.searchable ?? false)
      setIsPublic(attribute?.isPublic ?? true)
      setFacetCollapsed(attribute?.facetCollapsed ?? false)
      setIsIndexable(attribute?.isIndexable ?? false)
      setH1Position(attribute?.h1Position ?? 'BEFORE')
      setSortOrder(attribute?.sortOrder ?? 0)
      setDefaultValueRaw('')
      setDefaultBool(false)
      setHasDefault(false)
      setSubmitError(null)
    }
  }

  // Drop the default whenever input type changes - the prior value's shape
  // probably no longer fits, and silently coercing surprises people
  const [prevInputType, setPrevInputType] = useState(inputType)
  if (prevInputType !== inputType) {
    setPrevInputType(inputType)
    setHasDefault(false)
    setDefaultValueRaw('')
    setDefaultBool(false)
  }

  // Required ⇒ default invariant: when an admin checks "value required" on
  // a scalar attribute, force the default toggle on so they can't ship the
  // form without setting one - Selectables can't satisfy the invariant at
  // create time (their value IDs don't exist yet), so we block submit and
  // explain in the UI rather than force-flip anything
  const requiresDefault = !isEdit && valueRequired
  // Declared after the input-type reset above so that when both fire in
  // one render the force wins
  const [prevForce, setPrevForce] = useState({ requiresDefault, inputType })
  if (prevForce.requiresDefault !== requiresDefault || prevForce.inputType !== inputType) {
    setPrevForce({ requiresDefault, inputType })
    if (requiresDefault && isScalarInputType(inputType)) {
      setHasDefault(true)
    }
  }

  function onNameChange(next: string) {
    setName(next)
    if (!isEdit && !slugTouched) setSlug(slugify(next))
  }

  const refetchQueries = isEdit && attribute
    ? [
        { query: AttributesDocument },
        { query: AttributeDocument, variables: { id: attribute.id } },
      ]
    : [{ query: AttributesDocument }]

  const [createAttribute, { loading: creating }] = useMutation(CreateAttributeDocument, {
    refetchQueries,
  })
  const [updateAttribute, { loading: updating }] = useMutation(UpdateAttributeDocument, {
    refetchQueries,
  })
  const loading = creating || updating

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setSubmitError(null)
    try {
      if (isEdit && attribute) {
        await updateAttribute({
          variables: {
            input: {
              id: attribute.id,
              name,
              unit,
              valueRequired,
              filterable,
              searchable,
              isPublic,
              facetCollapsed,
              isIndexable,
              h1Position,
              sortOrder,
            },
          },
        })
      } else {
        if (valueRequired && !isScalarInputType(inputType)) {
          setSubmitError(
            'Required selectable attributes need a default - create this attribute with "Value required" off, add values, then turn required on alongside the default on the detail page.',
          )
          return
        }
        if (valueRequired && !hasDefault) {
          setSubmitError('Required attributes must have a default value.')
          return
        }
        let defaultValue: unknown = undefined
        if (hasDefault && isScalarInputType(inputType)) {
          if (inputType === 'BOOLEAN') {
            defaultValue = defaultBool
          } else if (inputType === 'NUMERIC') {
            const n = Number(defaultValueRaw)
            if (!Number.isFinite(n)) {
              setSubmitError('Default value must be a number.')
              return
            }
            defaultValue = n
          } else {
            if (!defaultValueRaw.trim()) {
              setSubmitError('Default value cannot be empty.')
              return
            }
            defaultValue = defaultValueRaw.trim()
          }
        }
        await createAttribute({
          variables: {
            input: {
              slug,
              name,
              inputType,
              relationshipTarget: inputType === 'REFERENCE' ? relationshipTarget : undefined,
              unit: unit || undefined,
              valueRequired,
              filterable,
              searchable,
              isPublic,
              facetCollapsed,
              isIndexable,
              h1Position,
              sortOrder,
              defaultValue,
            },
          },
        })
      }
      setOpen(false)
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'Something went wrong.')
    }
  }

  return (
    <Dialog open={actualOpen} onOpenChange={setOpen}>
      {trigger && <DialogTrigger asChild>{trigger}</DialogTrigger>}
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{isEdit ? `Edit ${attribute?.name}` : 'New attribute'}</DialogTitle>
          <DialogDescription>
            {isEdit
              ? 'Slug and input type are immutable - changing input type would orphan existing values.'
              : 'Attributes are reusable facets (Color, Size, Material) that product types can assign.'}
          </DialogDescription>
        </DialogHeader>

        <form id="attribute-form" onSubmit={onSubmit} className="grid gap-4">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div className="grid gap-2">
              <Label htmlFor="attr-name">Name</Label>
              <Input
                id="attr-name"
                required
                placeholder="Color"
                value={name}
                onChange={(e) => onNameChange(e.target.value)}
              />
              <p aria-hidden="true" className="invisible min-h-4 text-xs text-muted-foreground">
                Helper text placeholder
              </p>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="attr-slug">Slug</Label>
              <Input
                id="attr-slug"
                required
                disabled={isEdit}
                placeholder="color"
                value={slug}
                onChange={(e) => {
                  setSlug(e.target.value)
                  setSlugTouched(true)
                }}
              />
              {isEdit ? (
                <p className="text-xs text-muted-foreground">Slug is immutable.</p>
              ) : (
                <p className="text-xs text-muted-foreground">Auto-filled from name - edit to override.</p>
              )}
            </div>
          </div>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div className="grid gap-2">
              <Label htmlFor="attr-type">Input type</Label>
              <Select
                value={inputType}
                onValueChange={(v) => setInputType(v as AttributeInputType)}
                disabled={isEdit}
              >
                <SelectTrigger id="attr-type">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {INPUT_TYPE_OPTIONS.map((t) => (
                    <SelectItem key={t} value={t}>
                      {INPUT_TYPE_LABEL[t]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {isEdit ? (
                <p className="text-xs text-muted-foreground">Input type is immutable.</p>
              ) : (
                <p aria-hidden="true" className="invisible min-h-4 text-xs text-muted-foreground">
                  Helper text placeholder
                </p>
              )}
            </div>
            {inputType === 'REFERENCE' && (
              <div className="grid gap-2">
                <Label htmlFor="attr-ref-target">Reference target</Label>
                <Select
                  value={relationshipTarget}
                  onValueChange={(v) => setRelationshipTarget(v as AttributeReferenceTarget)}
                  disabled={isEdit}
                >
                  <SelectTrigger id="attr-ref-target">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {REFERENCE_TARGET_OPTIONS.map((t) => (
                      <SelectItem key={t} value={t}>
                        {t.charAt(0) + t.slice(1).toLowerCase()}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">
                  Values link to a {relationshipTarget.toLowerCase()}; set them per-product.
                </p>
              </div>
            )}
            <div className="grid gap-2">
              <Label htmlFor="attr-unit">Unit</Label>
              <Input
                id="attr-unit"
                placeholder="cm, kg, etc."
                value={unit}
                onChange={(e) => setUnit(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">Optional display suffix for numeric values.</p>
            </div>
          </div>

          <div className="grid gap-3 rounded-md border p-4">
            <SwitchRow
              id="attr-required"
              label="Value required"
              description="Products of types assigned to this attribute must provide a value."
              checked={valueRequired}
              onCheckedChange={setValueRequired}
            />
            <SwitchRow
              id="attr-filterable"
              label="Filterable"
              description="Show this attribute in channel filter facets."
              checked={filterable}
              onCheckedChange={setFilterable}
            />
            <SwitchRow
              id="attr-facet-collapsed"
              label="Collapse facet by default"
              description={
                filterable
                  ? 'Channels render this filter collapsed on first load. Users can still open it.'
                  : 'Only applies when "Filterable" is on.'
              }
              checked={facetCollapsed}
              onCheckedChange={setFacetCollapsed}
            />
            <SwitchRow
              id="attr-indexable"
              label="Indexable facet (master switch)"
              description="Required for any filter value on this attribute to be indexable. ANDs with the per-value isIndexable flag."
              checked={isIndexable}
              onCheckedChange={setIsIndexable}
            />
            <div className="flex items-start justify-between gap-4">
              <div className="grid gap-0.5">
                <Label htmlFor="attr-h1-position" className="text-sm">H1 position</Label>
                <p className="text-xs text-muted-foreground">
                  Where the value appears in the listing page H1 when
                  indexable. BEFORE → "Olive / Apparel"; AFTER → "Apparel / Olive".
                </p>
              </div>
              <select
                id="attr-h1-position"
                className="h-8 rounded-md border bg-background px-2 text-sm"
                value={h1Position}
                onChange={(e) => setH1Position(e.target.value as 'BEFORE' | 'AFTER')}
              >
                <option value="BEFORE">Before</option>
                <option value="AFTER">After</option>
              </select>
            </div>
            <SwitchRow
              id="attr-searchable"
              label="Searchable"
              description="Include this attribute's value in full-text product search."
              checked={searchable}
              onCheckedChange={setSearchable}
            />
            <SwitchRow
              id="attr-visible"
              label="Visible to customers"
              description="Display this attribute on the product detail page."
              checked={isPublic}
              onCheckedChange={setIsPublic}
            />
          </div>

          <div className="grid gap-2">
            <Label htmlFor="attr-sort">Sort order</Label>
            <Input
              id="attr-sort"
              type="number"
              inputMode="numeric"
              value={sortOrder}
              onChange={(e) => setSortOrder(Number(e.target.value) || 0)}
            />
            <p className="text-xs text-muted-foreground">
              Lower numbers appear first when listing attributes.
            </p>
          </div>

          {!isEdit && isScalarInputType(inputType) && (
            <div className="grid gap-3 rounded-md border p-4">
              <div className="flex items-start justify-between gap-4">
                <div className="grid gap-0.5">
                  <Label htmlFor="attr-default-toggle" className="text-sm">
                    Default value{requiresDefault && ' (required)'}
                  </Label>
                  <p className="text-xs text-muted-foreground">
                    {requiresDefault
                      ? 'Required attributes must have a default - new products auto-fill from it so "required" actually holds.'
                      : 'Auto-applied to new products of types that include this attribute. Admin can still override per product.'}
                  </p>
                </div>
                <Switch
                  id="attr-default-toggle"
                  checked={hasDefault}
                  onCheckedChange={setHasDefault}
                  disabled={requiresDefault}
                />
              </div>
              {hasDefault && (
                <div className="grid gap-2">
                  {inputType === 'BOOLEAN' ? (
                    <div className="flex items-center gap-3">
                      <Switch
                        id="attr-default-bool"
                        checked={defaultBool}
                        onCheckedChange={setDefaultBool}
                      />
                      <Label htmlFor="attr-default-bool" className="text-sm">
                        {defaultBool ? 'True' : 'False'}
                      </Label>
                    </div>
                  ) : (
                    <Input
                      id="attr-default-value"
                      type={
                        inputType === 'NUMERIC' ? 'number'
                          : inputType === 'DATE' ? 'date'
                            : inputType === 'DATE_TIME' ? 'datetime-local'
                              : 'text'
                      }
                      step={inputType === 'NUMERIC' ? 'any' : undefined}
                      value={defaultValueRaw}
                      onChange={(e) => setDefaultValueRaw(e.target.value)}
                    />
                  )}
                </div>
              )}
            </div>
          )}
          {!isEdit && !isScalarInputType(inputType) && (
            <Alert>
              <AlertTitle>Default value for selectable attributes</AlertTitle>
              <AlertDescription>
                {valueRequired
                  ? 'Selectable attributes can\'t be required at create time - their values don\'t exist yet. Create with "Value required" off, add values on the detail page, then enable required alongside the default.'
                  : 'After creating the attribute, add its values on the detail page and set the default there.'}
              </AlertDescription>
            </Alert>
          )}

          {submitError && (
            <Alert variant="destructive">
              <AlertTitle>Save failed</AlertTitle>
              <AlertDescription>{submitError}</AlertDescription>
            </Alert>
          )}
        </form>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => setOpen(false)}>
            Cancel
          </Button>
          <Button type="submit" form="attribute-form" disabled={loading}>
            {loading ? 'Saving…' : isEdit ? 'Save changes' : 'Create attribute'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function SwitchRow({
  id,
  label,
  description,
  checked,
  onCheckedChange,
}: {
  id: string
  label: string
  description: string
  checked: boolean
  onCheckedChange: (v: boolean) => void
}) {
  return (
    <div className="flex items-start justify-between gap-4">
      <div className="grid gap-0.5">
        <Label htmlFor={id} className="text-sm">
          {label}
        </Label>
        <p className="text-xs text-muted-foreground">{description}</p>
      </div>
      <Switch id={id} checked={checked} onCheckedChange={onCheckedChange} />
    </div>
  )
}

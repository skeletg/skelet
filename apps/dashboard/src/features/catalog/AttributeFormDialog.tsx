import { useState, type ReactNode } from 'react'
import { useMutation } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { FormDialog } from '@/components/form/FormDialog'
import { FormDialogBody } from '@/components/form/FormDialogBody'
import { FieldError } from '@/components/form/fields'
import { useDialogForm } from '@/components/form/useDialogForm'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { slugify } from '@/lib/slug'
import type { AttributeInputType } from '@/graphql/schema.types'
import { INPUT_TYPE_LABEL } from './attributeInputTypes'
import { AttributeDocument, AttributesDocument, CreateAttributeDocument, UpdateAttributeDocument } from './Catalog.gql'
import {
  INPUT_TYPE_OPTIONS,
  REFERENCE_TARGET_OPTIONS,
  createAttributeSchema,
  editAttributeSchema,
  isScalarInputType,
  toAttributeCreateInput,
  toAttributeUpdateInput,
  toAttributeValues,
  type ExistingAttribute,
} from './schemas/attribute'

export type { ExistingAttribute }

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
  return (
    <FormDialog open={open} onOpenChange={onOpenChange} trigger={trigger} className="sm:max-w-xl">
      {(close) => <AttributeForm attribute={attribute} onClose={close} />}
    </FormDialog>
  )
}

function AttributeForm({ attribute, onClose }: { attribute?: ExistingAttribute; onClose: () => void }) {
  const isEdit = !!attribute
  const [slugTouched, setSlugTouched] = useState(false)
  const refetchQueries = attribute
    ? [{ query: AttributesDocument }, { query: AttributeDocument, variables: { id: attribute.id } }]
    : [{ query: AttributesDocument }]
  const [createAttribute] = useMutation(CreateAttributeDocument, { refetchQueries })
  const [updateAttribute] = useMutation(UpdateAttributeDocument, { refetchQueries })

  const dialog = useDialogForm({
    baseline: toAttributeValues(attribute),
    schema: isEdit ? editAttributeSchema : createAttributeSchema,
    submit: async (v) => {
      if (attribute) await updateAttribute({ variables: { input: toAttributeUpdateInput(v, attribute.id) } })
      else await createAttribute({ variables: { input: toAttributeCreateInput(v) } })
    },
    onSaved: onClose,
  })
  const { form } = dialog
  const inputType = useStore(form.store, (s) => s.values.inputType) as AttributeInputType
  const valueRequired = useStore(form.store, (s) => s.values.valueRequired)
  const hasDefault = useStore(form.store, (s) => s.values.hasDefault)
  const filterable = useStore(form.store, (s) => s.values.filterable)
  const relationshipTarget = useStore(form.store, (s) => s.values.relationshipTarget)
  // A required scalar attribute can't be saved without its default, so the toggle is held on
  const requiresDefault = !isEdit && valueRequired
  const scalar = isScalarInputType(inputType)

  return (
    <FormDialogBody
      dialog={dialog}
      title={attribute ? `Edit ${attribute.name}` : 'New attribute'}
      description={
        attribute
          ? 'Slug and input type are immutable - changing input type would orphan existing values.'
          : 'Attributes are reusable facets (Color, Size, Material) that product types can assign.'
      }
      submitLabel={attribute ? 'Save changes' : 'Create attribute'}
      onCancel={onClose}
    >
      <div className="grid grid-cols-1 items-start gap-4 sm:grid-cols-2">
        <form.AppField
          name="name"
          listeners={{
            onChange: ({ value }) => {
              if (!isEdit && !slugTouched) form.setFieldValue('slug', slugify(value))
            },
          }}
        >
          {(f) => <f.TextField id="attr-name" label="Name" placeholder="Color" />}
        </form.AppField>
        <form.AppField name="slug">
          {(f) => (
            <f.TextField
              id="attr-slug"
              label="Slug"
              placeholder="color"
              disabled={isEdit}
              onValueChange={() => setSlugTouched(true)}
              hint={isEdit ? 'Slug is immutable.' : 'Auto-filled from name - edit to override.'}
            />
          )}
        </form.AppField>
      </div>

      <div className="grid grid-cols-1 items-start gap-4 sm:grid-cols-2">
        <form.AppField
          name="inputType"
          listeners={{
            // The prior default's shape probably no longer fits - drop it rather than coerce it
            onChange: ({ value }) => {
              form.setFieldValue('defaultRaw', '')
              form.setFieldValue('defaultBool', false)
              form.setFieldValue('hasDefault', form.state.values.valueRequired && isScalarInputType(value as AttributeInputType))
            },
          }}
        >
          {(f) => (
            <f.SelectField
              id="attr-type"
              label="Input type"
              disabled={isEdit}
              options={INPUT_TYPE_OPTIONS.map((t) => ({ value: t, label: INPUT_TYPE_LABEL[t] }))}
              hint={isEdit ? 'Input type is immutable.' : undefined}
            />
          )}
        </form.AppField>
        {inputType === 'REFERENCE' && (
          <form.AppField name="relationshipTarget">
            {(f) => (
              <f.SelectField
                id="attr-ref-target"
                label="Reference target"
                disabled={isEdit}
                options={REFERENCE_TARGET_OPTIONS.map((t) => ({ value: t, label: t.charAt(0) + t.slice(1).toLowerCase() }))}
                hint={`Values link to a ${relationshipTarget.toLowerCase()}; set them per-product.`}
              />
            )}
          </form.AppField>
        )}
        <form.AppField name="unit">
          {(f) => <f.TextField id="attr-unit" label="Unit" placeholder="cm, kg, etc." hint="Optional display suffix for numeric values." />}
        </form.AppField>
      </div>

      <div className="grid gap-3 rounded-md border p-4">
        <form.AppField
          name="valueRequired"
          listeners={{
            onChange: ({ value }) => {
              if (value && !isEdit && isScalarInputType(form.state.values.inputType as AttributeInputType)) {
                form.setFieldValue('hasDefault', true)
              }
            },
          }}
        >
          {(f) => (
            <f.SwitchField
              bordered={false}
              id="attr-required"
              label="Value required"
              description="Products of types assigned to this attribute must provide a value."
            />
          )}
        </form.AppField>
        <form.AppField name="filterable">
          {(f) => (
            <f.SwitchField bordered={false} id="attr-filterable" label="Filterable" description="Show this attribute in channel filter facets." />
          )}
        </form.AppField>
        <form.AppField name="facetCollapsed">
          {(f) => (
            <f.SwitchField
              bordered={false}
              id="attr-facet-collapsed"
              label="Collapse facet by default"
              description={
                filterable
                  ? 'Channels render this filter collapsed on first load. Users can still open it.'
                  : 'Only applies when "Filterable" is on.'
              }
            />
          )}
        </form.AppField>
        <form.AppField name="isIndexable">
          {(f) => (
            <f.SwitchField
              bordered={false}
              id="attr-indexable"
              label="Indexable facet (master switch)"
              description="Required for any filter value on this attribute to be indexable. ANDs with the per-value isIndexable flag."
            />
          )}
        </form.AppField>
        <form.Field name="h1Position">
          {(f) => (
            <div className="flex items-start justify-between gap-4">
              <div className="grid gap-0.5">
                <Label htmlFor="attr-h1-position" className="text-sm">
                  H1 position
                </Label>
                <p className="text-xs text-muted-foreground">
                  Where the value appears in the listing page H1 when indexable. BEFORE → "Olive / Apparel"; AFTER → "Apparel /
                  Olive".
                </p>
              </div>
              <select
                id="attr-h1-position"
                className="h-8 rounded-md border bg-background px-2 text-sm"
                value={f.state.value}
                onChange={(e) => f.handleChange(e.target.value as 'BEFORE' | 'AFTER')}
              >
                <option value="BEFORE">Before</option>
                <option value="AFTER">After</option>
              </select>
            </div>
          )}
        </form.Field>
        <form.AppField name="searchable">
          {(f) => (
            <f.SwitchField
              bordered={false}
              id="attr-searchable"
              label="Searchable"
              description="Include this attribute's value in full-text product search."
            />
          )}
        </form.AppField>
        <form.AppField name="isPublic">
          {(f) => (
            <f.SwitchField
              bordered={false}
              id="attr-visible"
              label="Visible to customers"
              description="Display this attribute on the product detail page."
            />
          )}
        </form.AppField>
      </div>

      <form.AppField name="sortOrder">
        {(f) => <f.NumberField id="attr-sort" label="Sort order" hint="Lower numbers appear first when listing attributes." />}
      </form.AppField>

      {!isEdit && scalar && (
        <div className="grid gap-3 rounded-md border p-4">
          <form.AppField name="hasDefault">
            {(f) => (
              <f.SwitchField
                bordered={false}
                id="attr-default-toggle"
                label={`Default value${requiresDefault ? ' (required)' : ''}`}
                description={
                  requiresDefault
                    ? 'Required attributes must have a default - new products auto-fill from it so "required" actually holds.'
                    : 'Auto-applied to new products of types that include this attribute. Admin can still override per product.'
                }
                disabled={requiresDefault}
              />
            )}
          </form.AppField>
          {hasDefault &&
            (inputType === 'BOOLEAN' ? (
              <form.Field name="defaultBool">
                {(f) => (
                  <div className="flex items-center gap-3">
                    <Switch id="attr-default-bool" checked={f.state.value} onCheckedChange={f.handleChange} />
                    <Label htmlFor="attr-default-bool" className="text-sm">
                      {f.state.value ? 'True' : 'False'}
                    </Label>
                  </div>
                )}
              </form.Field>
            ) : (
              <form.Field name="defaultRaw">
                {(f) => (
                  <div className="grid gap-1.5">
                    <Input
                      id="attr-default-value"
                      aria-label="Default value"
                      type={inputType === 'NUMERIC' ? 'number' : inputType === 'DATE' ? 'date' : inputType === 'DATE_TIME' ? 'datetime-local' : 'text'}
                      step={inputType === 'NUMERIC' ? 'any' : undefined}
                      value={f.state.value}
                      onChange={(e) => f.handleChange(e.target.value)}
                      onBlur={f.handleBlur}
                    />
                    <FieldError field={f} />
                  </div>
                )}
              </form.Field>
            ))}
        </div>
      )}
      {!isEdit && !scalar && (
        <Alert>
          <AlertTitle>Default value for selectable attributes</AlertTitle>
          <AlertDescription>
            {valueRequired
              ? 'Selectable attributes can\'t be required at create time - their values don\'t exist yet. Create with "Value required" off, add values on the detail page, then enable required alongside the default.'
              : 'After creating the attribute, add its values on the detail page and set the default there.'}
          </AlertDescription>
        </Alert>
      )}
    </FormDialogBody>
  )
}

import { useMemo, useState, type ReactNode } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { usePageTitle } from '@/lib/page-title'
import { MailCheckIcon, MailXIcon, PlusIcon, StarIcon, Trash2Icon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { AddressFields } from '@/components/form/AddressFields'
import { useDialogForm } from '@/components/form/useDialogForm'
import { DetailPageSection } from '@/components/layout/detail-page'
import { LoadError } from '@/shared/errors/LoadError'
import { EntityChannelForbidden, EntityWrongChannel } from '@/shared/errors/ChannelScopeError'
import { isForbiddenError } from '@/shared/errors/isForbiddenError'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { toAddressInput, toAddressValues } from '@/shared/schemas/address'
import { GdprActions } from './GdprActions'
import { MasqueradeButton } from '@/features/customers/MasqueradeButton'
import { LoyaltySection } from './LoyaltySection'
import { StoreCreditSection } from './StoreCreditSection'
import { useConfigScope } from '@/lib/config-scope'
import {
  AdminAddAddressDocument,
  AdminDeleteAddressDocument,
  AdminSetCustomerConsentDocument,
  AdminUpdateCustomerDocument,
  CustomerDocument,
  DeleteCustomerDocument,
  SetCustomerGroupDocument,
  SetCustomerTaxStatusDocument,
  SetCustomerVatIdDocument,
  type CustomerQuery,
} from './Customers.gql'
import { CustomerGroupsDocument } from './CustomerGroups.gql'
import { useCustomerAccess } from './useCustomerAccess'
import { PluginSections } from '@/plugins/detail-sections'
import {
  CUSTOMER_LABELS,
  customerChanges,
  customerDiff,
  customerSchema,
  newAddressSchema,
  toCustomerValues,
  type CustomerValues,
} from './schemas/customer'

type Customer = NonNullable<CustomerQuery['customer']>

export function CustomerDetail() {
  const { id = '' } = useParams()
  const { scope, selectedChannel } = useConfigScope()

  const { data, loading, error, refetch } = useQuery(CustomerDocument, {
    variables: { id },
    skip: !id,
    notifyOnNetworkStatusChange: true,
  })
  const customer = data?.customer
  usePageTitle(customer ? `${customer.firstName} ${customer.lastName}` : undefined)

  if (loading && !customer) return <p className="text-sm text-muted-foreground">Loading customer…</p>

  if (error) {
    // Channel-scoped roles get `forbidden` from the API when the customer belongs to a channel outside their role
    // scopes - show the dedicated permissions page rather than a retryable error
    if (isForbiddenError(error)) {
      return (
        <EntityChannelForbidden entityLabel="customer" permission="customers.view" backTo="/customers" backLabel="Back to customers" />
      )
    }
    return <LoadError title="Couldn't load customer" message={error.message} onRetry={() => void refetch()} retrying={loading} />
  }

  if (!customer) return <p className="text-sm text-muted-foreground">Customer not found.</p>

  // Header channel scope gate - Customers are per-channel accounts; when the header is scoped to a
  // channel this customer doesn't belong to, refuse to render the detail instead of silently showing
  // (and letting the operator edit) another channel's customer - Mirrors the server-side rule for
  // channel-scoped roles - this is the UX equivalent for global admins driving the header switcher
  if (scope.mode === 'CHANNEL' && selectedChannel && customer.channelId !== selectedChannel.id) {
    return (
      <EntityWrongChannel
        entityLabel="customer"
        entityChannelId={customer.channelId}
        permission="customers.view"
        backTo="/customers"
        backLabel="Back to customers"
      />
    )
  }

  return <CustomerForm customer={customer} />
}

// Profile, notes, group, tax, consents and metadata are one form behind the header Save; each part
// goes through its own mutation, only when it changed. Addresses, store credit and loyalty act on
// their own
function CustomerForm({ customer }: { customer: Customer }) {
  const navigate = useNavigate()
  const { canManage } = useCustomerAccess()
  const id = customer.id

  // Only this customer - dragging the whole customer table along each time is what hangs the tab
  const refetch = [{ query: CustomerDocument, variables: { id } }]
  const [updateCustomer, updateState] = useMutation(AdminUpdateCustomerDocument, { refetchQueries: refetch })
  const [setGroup] = useMutation(SetCustomerGroupDocument, { refetchQueries: refetch })
  const [setTaxStatus] = useMutation(SetCustomerTaxStatusDocument, { refetchQueries: refetch })
  const [setVatId] = useMutation(SetCustomerVatIdDocument, { refetchQueries: refetch })
  const [setConsent] = useMutation(AdminSetCustomerConsentDocument, { refetchQueries: refetch })
  const [addAddress] = useMutation(AdminAddAddressDocument, { refetchQueries: refetch })
  const [deleteAddress] = useMutation(AdminDeleteAddressDocument, { refetchQueries: refetch })
  const [deleteCustomer, deleteState] = useMutation(DeleteCustomerDocument)

  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const groupsQuery = useQuery(CustomerGroupsDocument)
  const groups = useMemo(() => groupsQuery.data?.customerGroups ?? [], [groupsQuery.data])

  const baseline = useMemo(() => toCustomerValues(customer), [customer])
  // Group NAMES in the diff (ids are an implementation detail); the id while the list loads
  const diff = useMemo(
    () =>
      customerDiff({
        group: (groupId) => groups.find((g) => g.id === groupId)?.name ?? groupId,
        consent: (kind) => customer.consents.find((c) => c.kind === kind)?.definition.name ?? kind,
      }),
    [groups, customer.consents],
  )
  const name = `${customer.firstName} ${customer.lastName}`

  const record = useRecordForm({
    id: 'customer',
    title: 'Customer',
    label: `Customer · ${name}`,
    isEdit: true,
    baseline,
    schema: customerSchema,
    diff,
    persist: { routeKey: `customer:${id}` },
    submit: async (v) => {
      const changes = customerChanges(baseline, v)
      if (changes.update) await updateCustomer({ variables: { id, input: changes.update } })
      if (changes.groupId) await setGroup({ variables: { customerId: id, groupId: changes.groupId } })
      if (changes.taxStatus) await setTaxStatus({ variables: { customerId: id, ...changes.taxStatus } })
      if (changes.vatId !== null) await setVatId({ variables: { customerId: id, vatId: changes.vatId } })
      for (const c of changes.consents) {
        await setConsent({ variables: { customerId: id, kind: c.kind, optedIn: c.optedIn } })
      }
    },
  })
  const { form } = record

  async function onDelete() {
    setDeleteError(null)
    try {
      await deleteCustomer({ variables: { id } })
      navigate('/customers', { replace: true })
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  return (
    <RecordFormPage
      record={record}
      backTo="/customers"
      backLabel="Back to customers"
      title={name}
      badges={
        <>
          {customer.isActive ? (
            <Badge variant="secondary">active</Badge>
          ) : (
            <Badge variant="outline" className="text-muted-foreground">
              inactive
            </Badge>
          )}
          <Badge variant="secondary">{customer.group.name}</Badge>
        </>
      }
      meta={
        <>
          <span className="inline-flex items-center gap-1.5">
            {customer.email}
            {customer.isEmailVerified ? (
              <MailCheckIcon className="size-3.5" aria-label="Verified" />
            ) : (
              <MailXIcon className="size-3.5" aria-label="Unverified" />
            )}
          </span>
          <span>·</span>
          <span>Joined {new Date(customer.createdAt).toLocaleString()}</span>
          <span>·</span>
          <span>Updated {new Date(customer.updatedAt).toLocaleString()}</span>
        </>
      }
      actions={
        <>
          <MasqueradeButton customerId={customer.id} />
          <GdprActions customerId={customer.id} customerName={name} />
        </>
      }
      createLabel="Save changes"
      labels={CUSTOMER_LABELS}
      hideSave={!canManage}
      after={
        <>
          <DetailPageSection
            title="Store credit"
            description="Credit spendable at checkout as a payment tender. Balance is restored automatically if a paying order is cancelled."
          >
            <StoreCreditSection customerId={id} />
          </DetailPageSection>

          <DetailPageSection
            title="Loyalty points"
            description="Points earned on paid orders at the configured rate; customers convert them into store credit. Points are revoked automatically if the earning order is cancelled."
          >
            <LoyaltySection customerId={id} />
          </DetailPageSection>

          <DetailPageSection title="Addresses" description="Manage addresses and set billing/shipping defaults.">
            <AddressesSection
              addresses={customer.addresses}
              defaultBillingId={customer.defaultBillingAddress?.id}
              defaultShippingId={customer.defaultShippingAddress?.id}
              setDefaultBilling={(addressId) => updateCustomer({ variables: { id, input: { defaultBillingAddressId: addressId } } })}
              setDefaultShipping={(addressId) => updateCustomer({ variables: { id, input: { defaultShippingAddressId: addressId } } })}
              onAddAddress={(input) => addAddress({ variables: { customerId: id, input } })}
              onDeleteAddress={(addressId) => deleteAddress({ variables: { id: addressId } })}
              saving={updateState.loading}
              canManage={canManage}
            />
          </DetailPageSection>

          <DetailPageSection
            title={<span className="text-destructive">Danger zone</span>}
            description="Deleting a customer cascades their addresses and carts. Customers with orders cannot be deleted - the backend rejects those to preserve order history."
          >
            <div>
              <Button
                variant="destructive"
                disabled={!canManage}
                onClick={() => {
                  setDeleteError(null)
                  setConfirmingDelete(true)
                }}
              >
                <Trash2Icon className="size-4" />
                Delete customer
              </Button>
            </div>
          </DetailPageSection>
          <PluginSections target="customer" entity={customer} mode="edit" />
        </>
      }
    >
      <DetailPageSection title="Profile & account" description="Name, contact details, and sign-in state.">
        <EditGate locked={!canManage}>
          <div className="grid gap-4">
            <div className="grid grid-cols-1 items-start gap-4 sm:grid-cols-2">
              <form.AppField name="firstName">{(f) => <f.TextField id="cd-first" label="First name" />}</form.AppField>
              <form.AppField name="lastName">{(f) => <f.TextField id="cd-last" label="Last name" />}</form.AppField>
            </div>
            <form.AppField name="email">{(f) => <f.TextField id="cd-email" label="Email" type="email" />}</form.AppField>
            <form.AppField name="phone">{(f) => <f.TextField id="cd-phone" label="Phone" type="tel" />}</form.AppField>
            <div className="grid gap-2">
              <form.AppField name="isActive">
                {(f) => <f.CheckboxField id="cd-active" label="Active - inactive customers can't sign in" />}
              </form.AppField>
              <form.AppField name="isEmailVerified">{(f) => <f.CheckboxField id="cd-verified" label="Email verified" />}</form.AppField>
            </div>
          </div>
        </EditGate>
      </DetailPageSection>

      <DetailPageSection
        title="Notes"
        description="Internal note visible only to admins. Use this for context that doesn't fit structured fields - payment quirks, support history, anything operators need at hand."
      >
        <EditGate locked={!canManage}>
          <form.AppField name="note">
            {(f) => <f.TextareaField aria-label="Note" rows={4} placeholder="Add an internal note about this customer…" />}
          </form.AppField>
        </EditGate>
      </DetailPageSection>

      <DetailPageSection title="Group" description="Assign the customer to a customer group.">
        <EditGate locked={!canManage}>
          {groupsQuery.loading ? (
            <p className="text-sm text-muted-foreground">Loading groups…</p>
          ) : (
            <form.Field name="groupId">
              {(f) => (
                <div className="grid gap-2" role="radiogroup" aria-label="Customer group">
                  {groups.map((g) => (
                    <label
                      key={g.id}
                      className="flex cursor-pointer items-center gap-2 rounded border bg-card px-3 py-2 hover:bg-muted/50 has-checked:border-foreground/40 has-checked:bg-muted"
                    >
                      <input
                        type="radio"
                        name="customer-group"
                        value={g.id}
                        checked={f.state.value === g.id}
                        onChange={(e) => f.handleChange(e.target.value)}
                        className="size-4"
                      />
                      <div className="flex-1">
                        <div className="text-sm font-medium">{g.name}</div>
                        <code className="text-xs text-muted-foreground">{g.slug}</code>
                      </div>
                      {g.isDefault && <span className="text-xs text-muted-foreground">default</span>}
                    </label>
                  ))}
                </div>
              )}
            </form.Field>
          )}
        </EditGate>
      </DetailPageSection>

      <DetailPageSection
        title="Tax"
        description="Exemption zeroes core tax on every order recompute (an active tax plugin stays authoritative). VAT IDs are format-checked on save and verified against VIES when the service is reachable."
      >
        <EditGate locked={!canManage}>
          <div className="grid gap-4">
            <form.AppField name="taxExempt">
              {(f) => <f.CheckboxField id="cd-exempt" label="Tax exempt - orders for this customer are charged zero tax" />}
            </form.AppField>
            <form.AppField name="taxExemptionNote">
              {(f) => <f.TextField id="cd-exempt-note" label="Exemption note" placeholder="Certificate number, reason…" />}
            </form.AppField>
            <div className="grid gap-2">
              <div className="flex items-end gap-2">
                <form.AppField name="vatId">
                  {(f) => <f.TextField id="cd-vat" label="VAT ID" placeholder="DE123456789" className="max-w-xs" />}
                </form.AppField>
                {customer.vatId && (
                  <Badge variant="outline" className={`mb-2 ${VAT_STATUS_STYLE[customer.vatIdStatus] ?? ''}`}>
                    {customer.vatIdStatus}
                  </Badge>
                )}
                {customer.vatIdCheckedAt && (
                  <span className="mb-2.5 text-xs text-muted-foreground">
                    checked {new Date(customer.vatIdCheckedAt).toLocaleDateString()}
                  </span>
                )}
              </div>
              <p className="text-xs text-muted-foreground">
                Format-checked on save; "unavailable" means VIES couldn't be reached - the ID is kept for a later re-check. Empty
                clears the ID.
              </p>
            </div>
          </div>
        </EditGate>
      </DetailPageSection>

      <DetailPageSection
        title="Communication preferences"
        description={
          'Marketing and transactional opt-ins. Saving here records source = "admin" in the consent audit trail. Required consents (e.g. transactional email) cannot be turned off.'
        }
      >
        <EditGate locked={!canManage}>
          <ConsentsList form={form} consents={customer.consents} />
        </EditGate>
      </DetailPageSection>

      <DetailPageSection title="Metadata" description="Arbitrary key-value pairs.">
        <EditGate locked={!canManage}>
          <form.AppField name="metadata">{(f) => <f.MetadataField />}</form.AppField>
        </EditGate>
      </DetailPageSection>

      <AlertDialog
        open={confirmingDelete}
        onOpenChange={(v) => {
          if (!v) {
            setConfirmingDelete(false)
            setDeleteError(null)
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete {name}?</AlertDialogTitle>
            <AlertDialogDescription>Addresses and carts will be removed. Customers with orders can't be deleted.</AlertDialogDescription>
          </AlertDialogHeader>
          {deleteError && (
            <Alert variant="destructive">
              <AlertTitle>Delete failed</AlertTitle>
              <AlertDescription>{deleteError}</AlertDescription>
            </Alert>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteState.loading}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={deleteState.loading}
              onClick={(e) => {
                e.preventDefault()
                void onDelete()
              }}
            >
              {deleteState.loading ? 'Deleting…' : 'Delete customer'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </RecordFormPage>
  )
}

// ---------- Sections ----------

type CustomerFormApi = ReturnType<typeof useRecordForm<CustomerValues>>['form']

// Read-only gate for the form's sections - a customers.view agent reads the page instead of filling
// in forms whose Save comes back FORBIDDEN - display:contents keeps the section layout
function EditGate({ locked, children }: { locked: boolean; children: ReactNode }) {
  return (
    <fieldset disabled={locked} className="contents">
      {children}
    </fieldset>
  )
}

const VAT_STATUS_STYLE: Record<string, string> = {
  valid: 'bg-success-soft text-success border-success/30',
  invalid: 'bg-destructive-soft text-destructive border-destructive/30',
  unavailable: 'bg-warning-soft text-warning border-warning/30',
  unverified: 'bg-muted text-muted-foreground border-border',
}

function ConsentsList({ form, consents }: { form: CustomerFormApi; consents: Customer['consents'] }) {
  const values = useStore(form.store, (s) => s.values.consents)
  if (consents.length === 0) return <p className="text-sm text-muted-foreground">No consent kinds registered.</p>
  return (
    <div className="grid gap-3">
      {consents.map((c) => (
        <div key={c.kind} className="flex items-start justify-between gap-3 rounded-md border bg-card px-3 py-2">
          <div className="grid gap-1">
            <div className="flex flex-wrap items-center gap-2 text-sm font-medium">
              {c.definition.name}
              <Badge variant="outline" className="text-[10px] uppercase">
                {c.definition.channel}
              </Badge>
              <Badge variant="secondary" className="text-[10px] uppercase">
                {c.definition.purpose}
              </Badge>
              {c.definition.required && (
                <Badge variant="outline" className="text-[10px] uppercase">
                  required
                </Badge>
              )}
            </div>
            {c.definition.description && <p className="text-xs text-muted-foreground">{c.definition.description}</p>}
            <p className="text-xs text-muted-foreground">
              {c.isDefault
                ? `Using registry default (${c.definition.defaultOptIn ? 'opted in' : 'opted out'}).`
                : `Source: ${c.source}${c.updatedAt ? ` · ${new Date(c.updatedAt).toLocaleString()}` : ''}`}
            </p>
          </div>
          <Switch
            checked={values[c.kind] ?? c.optedIn}
            disabled={c.definition.required}
            onCheckedChange={(next) => form.setFieldValue('consents', (prev) => ({ ...prev, [c.kind]: next }))}
            aria-label={`Toggle ${c.definition.name}`}
          />
        </div>
      ))}
    </div>
  )
}

type AddressView = Customer['addresses'][number]

// Entity CRUD section: addresses are added/deleted through an explicit row form / destructive
// buttons, and "set default" is an entity-level action - all immediate by convention, not staged
function AddressesSection({
  addresses,
  defaultBillingId,
  defaultShippingId,
  setDefaultBilling,
  setDefaultShipping,
  onAddAddress,
  onDeleteAddress,
  saving,
  canManage,
}: {
  addresses: AddressView[]
  defaultBillingId?: string
  defaultShippingId?: string
  setDefaultBilling: (id: string) => Promise<unknown>
  setDefaultShipping: (id: string) => Promise<unknown>
  onAddAddress: (input: ReturnType<typeof toAddressInput>) => Promise<unknown>
  onDeleteAddress: (id: string) => Promise<unknown>
  saving: boolean
  canManage: boolean
}) {
  const [showForm, setShowForm] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  async function onSetDefault(type: 'billing' | 'shipping', id: string) {
    setErr(null)
    try {
      if (type === 'billing') await setDefaultBilling(id)
      else await setDefaultShipping(id)
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Failed to set default.')
    }
  }

  async function onDelete(id: string) {
    setErr(null)
    try {
      await onDeleteAddress(id)
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Failed to delete address.')
    }
  }

  return (
    <div className="grid gap-4">
      {!showForm && canManage && (
        <div className="flex justify-end">
          <Button size="sm" variant="outline" onClick={() => setShowForm(true)}>
            <PlusIcon className="size-4" />
            Add address
          </Button>
        </div>
      )}

      {/* Mounted only while open, so each open starts blank */}
      {showForm && <AddAddressForm onAdd={onAddAddress} onClose={() => setShowForm(false)} />}

      {addresses.length === 0 && !showForm ? (
        <p className="text-sm text-muted-foreground">No addresses on file.</p>
      ) : (
        <div className="grid gap-3 md:grid-cols-2">
          {addresses.map((a) => (
            <div key={a.id} className="rounded border p-3 text-sm">
              <div className="mb-1 flex items-center gap-1.5">
                <span className="font-medium">
                  {a.firstName} {a.lastName}
                </span>
                {a.id === defaultBillingId && (
                  <Badge variant="outline" className="gap-1 font-normal">
                    <StarIcon className="size-3" /> billing
                  </Badge>
                )}
                {a.id === defaultShippingId && (
                  <Badge variant="outline" className="gap-1 font-normal">
                    <StarIcon className="size-3" /> shipping
                  </Badge>
                )}
              </div>
              {a.company && <div className="text-muted-foreground">{a.company}</div>}
              <div>{a.street1}</div>
              {a.street2 && <div>{a.street2}</div>}
              <div>
                {a.city}
                {a.state ? `, ${a.state}` : ''} {a.postalCode}
              </div>
              <div className="text-muted-foreground">{a.countryCode}</div>
              {a.phone && <div className="text-muted-foreground">{a.phone}</div>}
              <div className={`mt-2 flex flex-wrap gap-1.5 ${canManage ? '' : 'hidden'}`}>
                {a.id !== defaultBillingId && (
                  <Button size="sm" variant="outline" className="h-7 text-xs" disabled={saving} onClick={() => onSetDefault('billing', a.id)}>
                    Set billing default
                  </Button>
                )}
                {a.id !== defaultShippingId && (
                  <Button size="sm" variant="outline" className="h-7 text-xs" disabled={saving} onClick={() => onSetDefault('shipping', a.id)}>
                    Set shipping default
                  </Button>
                )}
                <Button
                  size="sm"
                  variant="ghost"
                  className="h-7 text-xs text-destructive hover:text-destructive"
                  disabled={saving}
                  onClick={() => onDelete(a.id)}
                >
                  <Trash2Icon className="size-3" />
                  Delete
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}

      {err && (
        <Alert variant="destructive">
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>{err}</AlertDescription>
        </Alert>
      )}
    </div>
  )
}

// The add-address row - an explicit CRUD form with its own Add, which the convention allows
function AddAddressForm({ onAdd, onClose }: { onAdd: (input: ReturnType<typeof toAddressInput>) => Promise<unknown>; onClose: () => void }) {
  const [baseline] = useState(() => ({ address: toAddressValues() }))
  const { form, error, onSubmit } = useDialogForm({
    baseline,
    schema: newAddressSchema,
    submit: async (v) => {
      await onAdd(toAddressInput(v.address))
    },
    onSaved: onClose,
  })
  const submitting = useStore(form.store, (s) => s.isSubmitting)

  return (
    <form onSubmit={onSubmit} className="grid gap-3 rounded border p-4">
      <AddressFields form={form} fields="address" idPrefix="addr" />
      {error && (
        <Alert variant="destructive">
          <AlertTitle>Couldn't add the address</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <div className="flex justify-end gap-2">
        <Button type="button" variant="outline" onClick={onClose}>
          Cancel
        </Button>
        <Button type="submit" disabled={submitting}>
          {submitting ? 'Adding…' : 'Add address'}
        </Button>
      </div>
    </form>
  )
}

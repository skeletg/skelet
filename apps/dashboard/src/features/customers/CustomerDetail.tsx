import { CountriesPickerDocument } from '@/features/countries/Countries.gql'
import { useCallback, useMemo, useState, type FormEvent, type ReactNode } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { usePageTitle } from '@/lib/page-title'
import {
  MailCheckIcon,
  MailXIcon,
  PlusIcon,
  StarIcon,
  Trash2Icon,
  XIcon,
} from 'lucide-react'
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
import { Checkbox } from '@/components/ui/checkbox'
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
import { Textarea } from '@/components/ui/textarea'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { LoadError } from '@/shared/errors/LoadError'
import { EntityChannelForbidden, EntityWrongChannel } from '@/shared/errors/ChannelScopeError'
import { isForbiddenError } from '@/shared/errors/isForbiddenError'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { GdprActions } from './GdprActions'
import { MasqueradeButton } from '@/features/customers/MasqueradeButton'
import { LoyaltySection } from './LoyaltySection'
import { StoreCreditSection } from './StoreCreditSection'
import { useConfigScope } from '@/lib/config-scope'
import {
  SaveRegistryContext,
  useSaveRegistry,
  useSaveSection,
  type SectionDiffEntry,
} from '@/shared/saving/saveRegistry'
import { added, changed, fieldDiff, removed } from '@/shared/saving/diff'
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
} from './Customers.gql'
import { CustomerGroupsDocument } from './CustomerGroups.gql'
import { useCustomerAccess } from './useCustomerAccess'
import { PluginSections } from '@/plugins/detail-sections'

export function CustomerDetail() {
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const { scope, selectedChannel } = useConfigScope()
  const { canManage } = useCustomerAccess()

  const { data, loading, error, refetch: refetchCustomer } = useQuery(CustomerDocument, {
    variables: { id },
    skip: !id,
    notifyOnNetworkStatusChange: true,
  })
  const customer = data?.customer
  usePageTitle(customer ? `${customer.firstName} ${customer.lastName}` : undefined)

  // Only this customer - a staged save fires one mutation per section, and dragging the whole
  // customer table along each time is what hangs the tab
  const refetch = [{ query: CustomerDocument, variables: { id } }]
  const [updateCustomer, updateState] = useMutation(AdminUpdateCustomerDocument, { refetchQueries: refetch })
  const [setGroup] = useMutation(SetCustomerGroupDocument, { refetchQueries: refetch })
  const [setTaxStatus] = useMutation(SetCustomerTaxStatusDocument, { refetchQueries: refetch })
  const [setVatId] = useMutation(SetCustomerVatIdDocument, { refetchQueries: refetch })
  const [addAddress] = useMutation(AdminAddAddressDocument, { refetchQueries: refetch })
  const [deleteAddress] = useMutation(AdminDeleteAddressDocument, { refetchQueries: refetch })
  const [deleteCustomer, deleteState] = useMutation(DeleteCustomerDocument)

  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  // Page save registry: profile, notes, group, consents, and metadata stage their edits and the header Save applies them through the
  // SaveConfirmDialog diff modal - the dashboard-wide convention (see docs/conventions.md) - Address CRUD stays immediate (entity rows)
  const { contextValue: saveContextValue, dirtySections, isDirty, saveAll } = useSaveRegistry()
  const [confirmingSave, setConfirmingSave] = useState(false)
  const [savingAll, setSavingAll] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  async function onConfirmSave() {
    setSaveError(null)
    setSavingAll(true)
    try {
      await saveAll()
      setConfirmingSave(false)
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Save failed.')
    } finally {
      setSavingAll(false)
    }
  }

  async function onDelete() {
    setDeleteError(null)
    try {
      await deleteCustomer({ variables: { id } })
      navigate('/customers', { replace: true })
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  if (loading) return <p className="text-sm text-muted-foreground">Loading customer…</p>

  if (error) {
    // Channel-scoped roles get `forbidden` from the API when the customer belongs to a channel outside their role
    // scopes - show the dedicated permissions page rather than a retryable error
    if (isForbiddenError(error)) {
      return (
        <EntityChannelForbidden
          entityLabel="customer"
          permission="customers.view"
          backTo="/customers"
          backLabel="Back to customers"
        />
      )
    }
    return (
      <LoadError
        title="Couldn't load customer"
        message={error.message}
        onRetry={() => void refetchCustomer()}
        retrying={loading}
      />
    )
  }

  if (!customer) return <p className="text-sm text-muted-foreground">Customer not found.</p>

  // Header channel scope gate - Customers are per-channel accounts; when
  // the header is scoped to a channel this customer doesn't belong to,
  // refuse to render the detail instead of silently showing (and
  // letting the operator edit) another channel's customer - Mirrors the
  // server-side rule for channel-scoped roles - this is the UX
  // equivalent for global admins driving the header switcher
  if (
    scope.mode === 'CHANNEL' &&
    selectedChannel &&
    customer.channelId !== selectedChannel.id
  ) {
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

  return (
    <SaveRegistryContext.Provider value={saveContextValue}>
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/customers"
        backLabel="Back to customers"
        title={`${customer.firstName} ${customer.lastName}`}
        badges={
          <>
            {customer.isActive ? (
              <Badge variant="secondary">active</Badge>
            ) : (
              <Badge variant="outline" className="text-muted-foreground">inactive</Badge>
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
          <div className="flex items-center gap-2">
            <MasqueradeButton customerId={customer.id} />
            <GdprActions customerId={customer.id} customerName={`${customer.firstName} ${customer.lastName}`} />
            {canManage && (
              <Button
                type="button"
                size="sm"
                disabled={!isDirty || savingAll}
                onClick={() => { setSaveError(null); setConfirmingSave(true) }}
              >
                {savingAll ? 'Saving…' : isDirty ? 'Save changes' : 'No changes'}
              </Button>
            )}
          </div>
        }
      />

      <DetailPageBody>
        <DetailPageSection
          title="Profile & account"
          description="Name, contact details, and sign-in state."
        >
          <EditGate locked={!canManage}>
            <ProfileAndAccountSection
              customer={customer}
              save={(input) => updateCustomer({ variables: { id, input } })}
            />
          </EditGate>
        </DetailPageSection>

        <DetailPageSection
          title="Notes"
          description="Internal note visible only to admins. Use this for context that doesn't fit structured fields - payment quirks, support history, anything operators need at hand."
        >
          <EditGate locked={!canManage}>
            <NotesSection
              note={customer.note ?? ''}
              save={(note) => updateCustomer({ variables: { id, input: { note } } })}
            />
          </EditGate>
        </DetailPageSection>

        <DetailPageSection
          title="Group"
          description="Assign the customer to a customer group."
        >
          <EditGate locked={!canManage}>
            <GroupSection
              customerGroupId={customer.group.id}
              save={(groupId) => setGroup({ variables: { customerId: id, groupId } })}
            />
          </EditGate>
        </DetailPageSection>

        <DetailPageSection
          title="Tax"
          description="Exemption zeroes core tax on every order recompute (an active tax plugin stays authoritative). VAT IDs are format-checked on save and verified against VIES when the service is reachable."
        >
          <EditGate locked={!canManage}>
            <TaxSection
              customer={customer}
              saveTaxStatus={(taxExempt, note) =>
                setTaxStatus({ variables: { customerId: id, taxExempt, note: note || null } })
              }
              saveVatId={(vatId) => setVatId({ variables: { customerId: id, vatId } })}
            />
          </EditGate>
        </DetailPageSection>

        <DetailPageSection
          title="Communication preferences"
          description={'Marketing and transactional opt-ins. Saving here records source = "admin" in the consent audit trail. Required consents (e.g. transactional email) cannot be turned off.'}
        >
          <EditGate locked={!canManage}>
            <CommunicationPreferencesSection
              customerId={id}
              consents={customer.consents}
              refetchQueries={refetch}
            />
          </EditGate>
        </DetailPageSection>

        <DetailPageSection
          title="Metadata"
          description="Arbitrary key-value pairs. null = no change, empty list = clear all."
        >
          <EditGate locked={!canManage}>
            <MetadataSection
              metadata={customer.metadata}
              save={(metadata) => updateCustomer({ variables: { id, input: { metadata } } })}
            />
          </EditGate>
        </DetailPageSection>

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

        <DetailPageSection
          title="Addresses"
          description="Manage addresses and set billing/shipping defaults."
        >
          <AddressesSection
            addresses={customer.addresses}
            defaultBillingId={customer.defaultBillingAddress?.id}
            defaultShippingId={customer.defaultShippingAddress?.id}
            setDefaultBilling={(addressId) =>
              updateCustomer({ variables: { id, input: { defaultBillingAddressId: addressId } } })
            }
            setDefaultShipping={(addressId) =>
              updateCustomer({ variables: { id, input: { defaultShippingAddressId: addressId } } })
            }
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
              onClick={() => { setDeleteError(null); setConfirmingDelete(true) }}
            >
              <Trash2Icon className="size-4" />
              Delete customer
            </Button>
          </div>
        </DetailPageSection>
        <PluginSections target="customer" entity={customer} mode="edit" />
      </DetailPageBody>

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
            <AlertDialogTitle>
              Delete {customer.firstName} {customer.lastName}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              Addresses and carts will be removed. Customers with orders can't be deleted.
            </AlertDialogDescription>
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
              onClick={(e) => { e.preventDefault(); onDelete() }}
            >
              {deleteState.loading ? 'Deleting…' : 'Delete customer'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Review-before-save modal: lists every staged change across the
          page's sections (profile, notes, group, consents, metadata) */}
      <SaveConfirmDialog
        open={confirmingSave}
        scopeLabel={`Customer · ${customer.firstName} ${customer.lastName}`}
        sections={dirtySections}
        saving={savingAll}
        error={saveError}
        onCancel={() => { if (!savingAll) { setConfirmingSave(false); setSaveError(null) } }}
        onConfirm={onConfirmSave}
      />
    </DetailPageLayout>
    </SaveRegistryContext.Provider>
  )
}

// ---------- Section cards ----------

type MetadataEntry = { key: string; value: string }

type CustomerView = {
  email: string
  firstName: string
  lastName: string
  phone?: string | null
  isActive: boolean
  isEmailVerified: boolean
  metadata: MetadataEntry[]
}

type UpdateInput = {
  email?: string
  firstName?: string
  lastName?: string
  phone?: string | null
  isActive?: boolean
  isEmailVerified?: boolean
  metadata?: MetadataEntry[] | null
  defaultBillingAddressId?: string | null
  defaultShippingAddressId?: string | null
}

// Read-only gate for the staged-edit sections - a customers.view agent reads the page instead of
// filling in forms whose Save comes back FORBIDDEN - display:contents keeps the section layout,
// and it wraps section bodies rather than the whole page so DetailPageBody's dividers survive
function EditGate({ locked, children }: { locked: boolean; children: ReactNode }) {
  return (
    <fieldset disabled={locked} className="contents">
      {children}
    </fieldset>
  )
}

function PendingHint({ show }: { show: boolean }) {
  if (!show) return null
  return (
    <p className="text-xs text-muted-foreground">
      Pending changes - applied with the page Save button.
    </p>
  )
}

// Keeps long values (notes, metadata blobs) readable in the diff modal
function clip(s: string, max = 80) {
  return s.length > max ? `${s.slice(0, max - 1)}…` : s
}

function ProfileAndAccountSection({
  customer,
  save,
}: {
  customer: CustomerView
  save: (input: UpdateInput) => Promise<unknown>
}) {
  const [firstName, setFirstName] = useState(customer.firstName)
  const [lastName, setLastName] = useState(customer.lastName)
  const [email, setEmail] = useState(customer.email)
  const [phone, setPhone] = useState(customer.phone ?? '')
  const [isActive, setIsActive] = useState(customer.isActive)
  const [isEmailVerified, setIsEmailVerified] = useState(customer.isEmailVerified)

  // Re-hydrate the draft when the server payload changes (post-save refetch) - the adjust-state-during-render pattern
  // from react.dev "You Might Not Need an Effect"
  const [prevCustomer, setPrevCustomer] = useState(customer)
  if (prevCustomer !== customer) {
    setPrevCustomer(customer)
    setFirstName(customer.firstName)
    setLastName(customer.lastName)
    setEmail(customer.email)
    setPhone(customer.phone ?? '')
    setIsActive(customer.isActive)
    setIsEmailVerified(customer.isEmailVerified)
  }

  // fieldDiff normalizes through display(), so a stray trailing space in Phone never marks
  // the page dirty or renders as `"555" -> "555 "`
  const diff = useMemo<SectionDiffEntry[]>(
    () =>
      fieldDiff(
        customer,
        { ...customer, firstName, lastName, email, phone, isActive, isEmailVerified },
        [
          { label: 'first name', value: (c) => c.firstName },
          { label: 'last name', value: (c) => c.lastName },
          { label: 'email', value: (c) => c.email },
          { label: 'phone', value: (c) => c.phone ?? '' },
          { label: 'active', value: (c) => c.isActive },
          { label: 'email verified', value: (c) => c.isEmailVerified },
        ],
      ),
    [customer, firstName, lastName, email, phone, isActive, isEmailVerified],
  )

  useSaveSection({
    id: 'profile',
    title: 'Profile & account',
    diff,
    save: async () => {
      if (!firstName.trim() || !lastName.trim()) throw new Error('First and last name are required.')
      if (!email.trim()) throw new Error('Email is required.')
      // A <form> would enforce type="email" at submit; staged saves
      // bypass form validation, so re-check the shape here
      if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim())) throw new Error('Email address is not valid.')
      // Send what was validated - the confirm dialog shows the trimmed value too
      await save({
        firstName: firstName.trim(),
        lastName: lastName.trim(),
        email: email.trim(),
        phone: phone.trim() || null,
        isActive,
        isEmailVerified,
      })
    },
  })

  return (
    <div className="grid gap-4">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div className="grid gap-2">
          <Label htmlFor="cd-first">First name</Label>
          <Input id="cd-first" value={firstName} onChange={(e) => setFirstName(e.target.value)} required />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="cd-last">Last name</Label>
          <Input id="cd-last" value={lastName} onChange={(e) => setLastName(e.target.value)} required />
        </div>
      </div>
      <div className="grid gap-2">
        <Label htmlFor="cd-email">Email</Label>
        <Input id="cd-email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
      </div>
      <div className="grid gap-2">
        <Label htmlFor="cd-phone">Phone</Label>
        <Input id="cd-phone" type="tel" value={phone} onChange={(e) => setPhone(e.target.value)} />
      </div>
      <div className="grid gap-2">
        <div className="flex items-center gap-2">
          <Checkbox id="cd-active" checked={isActive} onCheckedChange={(v) => setIsActive(!!v)} />
          <Label htmlFor="cd-active" className="cursor-pointer">Active - inactive customers can't sign in</Label>
        </div>
        <div className="flex items-center gap-2">
          <Checkbox id="cd-verified" checked={isEmailVerified} onCheckedChange={(v) => setIsEmailVerified(!!v)} />
          <Label htmlFor="cd-verified" className="cursor-pointer">Email verified</Label>
        </div>
      </div>
      <PendingHint show={diff.length > 0} />
    </div>
  )
}

const VAT_STATUS_STYLE: Record<string, string> = {
  valid: 'bg-emerald-500/10 text-emerald-600 border-emerald-500/30',
  invalid: 'bg-red-500/10 text-red-600 border-red-500/30',
  unavailable: 'bg-amber-500/10 text-amber-600 border-amber-500/30',
  unverified: 'bg-muted text-muted-foreground border-border',
}

function TaxSection({
  customer,
  saveTaxStatus,
  saveVatId,
}: {
  customer: {
    taxExempt: boolean
    taxExemptionNote: string
    vatId: string
    vatIdStatus: string
    vatIdCheckedAt?: string | null
  }
  saveTaxStatus: (taxExempt: boolean, note: string) => Promise<unknown>
  saveVatId: (vatId: string) => Promise<unknown>
}) {
  const [exempt, setExempt] = useState(customer.taxExempt)
  const [note, setNote] = useState(customer.taxExemptionNote)
  const [vat, setVat] = useState(customer.vatId)

  const [prev, setPrev] = useState(customer)
  if (prev !== customer) {
    setPrev(customer)
    setExempt(customer.taxExempt)
    setNote(customer.taxExemptionNote)
    setVat(customer.vatId)
  }

  const diff = useMemo<SectionDiffEntry[]>(
    () =>
      [
        changed('tax exempt', customer.taxExempt, exempt),
        changed('exemption note', customer.taxExemptionNote, note),
        changed('VAT ID', customer.vatId || '(none)', vat.trim() || '(cleared)'),
      ].filter((e): e is SectionDiffEntry => e !== null),
    [customer, exempt, note, vat],
  )

  useSaveSection({
    id: 'tax',
    title: 'Tax',
    diff,
    save: async () => {
      if (exempt !== customer.taxExempt || note.trim() !== customer.taxExemptionNote.trim()) {
        await saveTaxStatus(exempt, note.trim())
      }
      if (vat.trim() !== customer.vatId) {
        await saveVatId(vat.trim())
      }
    },
  })

  return (
    <div className="grid gap-4">
      <div className="flex items-center gap-2">
        <Checkbox id="cd-exempt" checked={exempt} onCheckedChange={(v) => setExempt(!!v)} />
        <Label htmlFor="cd-exempt" className="cursor-pointer">
          Tax exempt - orders for this customer are charged zero tax
        </Label>
      </div>
      <div className="grid gap-2">
        <Label htmlFor="cd-exempt-note">Exemption note</Label>
        <Input
          id="cd-exempt-note"
          placeholder="Certificate number, reason…"
          value={note}
          onChange={(e) => setNote(e.target.value)}
        />
      </div>
      <div className="grid gap-2">
        <Label htmlFor="cd-vat">VAT ID</Label>
        <div className="flex items-center gap-2">
          <Input
            id="cd-vat"
            placeholder="DE123456789"
            className="max-w-xs font-mono"
            value={vat}
            onChange={(e) => setVat(e.target.value)}
          />
          {customer.vatId && (
            <Badge variant="outline" className={VAT_STATUS_STYLE[customer.vatIdStatus] ?? ''}>
              {customer.vatIdStatus}
            </Badge>
          )}
          {customer.vatIdCheckedAt && (
            <span className="text-xs text-muted-foreground">
              checked {new Date(customer.vatIdCheckedAt).toLocaleDateString()}
            </span>
          )}
        </div>
        <p className="text-xs text-muted-foreground">
          Format-checked on save; "unavailable" means VIES couldn't be reached - the ID is kept for a later
          re-check. Empty clears the ID.
        </p>
      </div>
      <PendingHint show={diff.length > 0} />
    </div>
  )
}

// Group assignment rides the dedicated setCustomerGroup op (by group id) -
// it validates the target group exists instead of resolving a slug
function GroupSection({
  customerGroupId,
  save,
}: {
  customerGroupId: string
  save: (groupId: string) => Promise<unknown>
}) {
  const { data, loading } = useQuery(CustomerGroupsDocument)
  const groups = useMemo(() => data?.customerGroups ?? [], [data])
  const [selected, setSelected] = useState(customerGroupId)

  // Reset the draft when the saved group changes (adjust state during render)
  const [prevGroupId, setPrevGroupId] = useState(customerGroupId)
  if (prevGroupId !== customerGroupId) {
    setPrevGroupId(customerGroupId)
    setSelected(customerGroupId)
  }

  // Show group NAMES in the diff (ids are an implementation detail);
  // fall back to the id while the list is still loading
  const groupLabel = useCallback(
    (groupId: string) => groups.find((g) => g.id === groupId)?.name ?? groupId,
    [groups],
  )
  const diff = useMemo<SectionDiffEntry[]>(() => {
    const entry = changed('group', groupLabel(customerGroupId), groupLabel(selected))
    return entry ? [entry] : []
  }, [selected, customerGroupId, groupLabel])

  useSaveSection({
    id: 'group',
    title: 'Group',
    diff,
    save: async () => {
      await save(selected)
    },
  })

  return (
    <div className="grid gap-4">
      {loading ? (
        <p className="text-sm text-muted-foreground">Loading groups…</p>
      ) : (
        <div className="grid gap-2">
          {groups.map((g) => (
            <label
              key={g.id}
              className="flex cursor-pointer items-center gap-2 rounded border bg-card px-3 py-2 hover:bg-accent/50 has-[:checked]:border-primary has-[:checked]:bg-accent"
            >
              <input
                type="radio"
                name="customer-group"
                value={g.id}
                checked={selected === g.id}
                onChange={(e) => setSelected(e.target.value)}
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
      <PendingHint show={diff.length > 0} />
    </div>
  )
}

type AddressView = {
  id: string
  firstName: string
  lastName: string
  company?: string | null
  street1: string
  street2?: string | null
  city: string
  state?: string | null
  postalCode: string
  countryCode: string
  phone?: string | null
}

function NotesSection({
  note,
  save,
}: {
  note: string
  save: (note: string) => Promise<unknown>
}) {
  const [value, setValue] = useState(note)

  // Reset the draft when the saved note changes (adjust state during render)
  const [prevNote, setPrevNote] = useState(note)
  if (prevNote !== note) {
    setPrevNote(note)
    setValue(note)
  }

  const diff = useMemo<SectionDiffEntry[]>(() => {
    const entry = changed('note', clip(note), clip(value))
    return entry ? [entry] : []
  }, [value, note])

  useSaveSection({
    id: 'notes',
    title: 'Notes',
    diff,
    save: async () => {
      await save(value)
    },
  })

  return (
    <div className="grid gap-4">
      <Textarea
        value={value}
        rows={4}
        placeholder="Add an internal note about this customer…"
        onChange={(e) => setValue(e.target.value)}
      />
      <PendingHint show={diff.length > 0} />
    </div>
  )
}

type ConsentView = {
  kind: string
  optedIn: boolean
  source: string
  updatedAt?: string | null
  isDefault: boolean
  definition: {
    slug: string
    name: string
    description: string
    purpose: string
    channel: string
    required: boolean
    defaultOptIn: boolean
  }
}

function CommunicationPreferencesSection({
  customerId,
  consents,
  refetchQueries,
}: {
  customerId: string
  consents: ConsentView[]
  refetchQueries: { query: typeof CustomerDocument; variables?: { id: string } }[]
}) {
  const [setConsent] = useMutation(AdminSetCustomerConsentDocument, { refetchQueries })
  // Staged toggles: kind → desired opt-in - No reset effect needed - after save + refetch the server values match the
  // draft, so the diff clears on its own
  const [draft, setDraft] = useState<Record<string, boolean>>({})

  const effective = (c: ConsentView) => draft[c.kind] ?? c.optedIn

  const diff = useMemo<SectionDiffEntry[]>(
    () =>
      consents
        .filter((c) => c.kind in draft && draft[c.kind] !== c.optedIn)
        .map((c) =>
          changed(
            c.definition.name,
            c.optedIn ? 'opted in' : 'opted out',
            draft[c.kind] ? 'opted in' : 'opted out',
          ),
        )
        .filter((e): e is SectionDiffEntry => e !== null),
    [consents, draft],
  )

  useSaveSection({
    id: 'consents',
    title: 'Communication preferences',
    diff,
    save: async () => {
      // One mutation per changed kind so each lands as its own
      // source="admin" entry in the consent audit trail
      for (const c of consents) {
        if (c.kind in draft && draft[c.kind] !== c.optedIn) {
          await setConsent({ variables: { customerId, kind: c.kind, optedIn: draft[c.kind] } })
        }
      }
    },
  })

  return (
    <div className="grid gap-3">
      {consents.length === 0 ? (
        <p className="text-sm text-muted-foreground">No consent kinds registered.</p>
      ) : (
        consents.map((c) => (
          <div
            key={c.kind}
            className="flex items-start justify-between gap-3 rounded-md border bg-card px-3 py-2"
          >
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
                  <Badge variant="outline" className="text-[10px] uppercase">required</Badge>
                )}
              </div>
              {c.definition.description && (
                <p className="text-xs text-muted-foreground">{c.definition.description}</p>
              )}
              <p className="text-xs text-muted-foreground">
                {c.isDefault
                  ? `Using registry default (${c.definition.defaultOptIn ? 'opted in' : 'opted out'}).`
                  : `Source: ${c.source}${c.updatedAt ? ` · ${new Date(c.updatedAt).toLocaleString()}` : ''}`}
              </p>
            </div>
            <Switch
              checked={effective(c)}
              disabled={c.definition.required}
              onCheckedChange={(next) => setDraft((prev) => ({ ...prev, [c.kind]: next }))}
              aria-label={`Toggle ${c.definition.name}`}
            />
          </div>
        ))
      )}
      <PendingHint show={diff.length > 0} />
    </div>
  )
}

function MetadataSection({
  metadata,
  save,
}: {
  metadata: MetadataEntry[]
  save: (metadata: MetadataEntry[]) => Promise<unknown>
}) {
  const [rows, setRows] = useState<MetadataEntry[]>(metadata)

  // Reset the draft when the saved metadata changes (adjust state during render)
  const [prevMetadata, setPrevMetadata] = useState(metadata)
  if (prevMetadata !== metadata) {
    setPrevMetadata(metadata)
    setRows(metadata)
  }

  function addRow() {
    setRows((prev) => [...prev, { key: '', value: '' }])
  }

  function removeRow(i: number) {
    setRows((prev) => prev.filter((_, idx) => idx !== i))
  }

  function setKey(i: number, key: string) {
    setRows((prev) => prev.map((r, idx) => (idx === i ? { ...r, key } : r)))
  }

  function setValue(i: number, value: string) {
    setRows((prev) => prev.map((r, idx) => (idx === i ? { ...r, value } : r)))
  }

  // Key-based diff: blank-key rows still being typed are excluded so the
  // section only reads dirty once they hold a real entry
  const diff = useMemo<SectionDiffEntry[]>(() => {
    const before = new Map(metadata.map((m) => [m.key, m.value]))
    const after = new Map(rows.filter((r) => r.key.trim()).map((r) => [r.key, r.value]))
    const out: SectionDiffEntry[] = []
    for (const [k, v] of after) {
      const prev = before.get(k)
      if (prev === undefined) {
        out.push(added(k, clip(v)))
        continue
      }
      const entry = changed(k, clip(prev), clip(v))
      if (entry) out.push(entry)
    }
    for (const [k, v] of before) {
      if (!after.has(k)) out.push(removed(k, clip(v)))
    }
    return out
  }, [rows, metadata])

  useSaveSection({
    id: 'metadata',
    title: 'Metadata',
    diff,
    save: async () => {
      const dupes = rows.map((r) => r.key).filter((k, i, arr) => k && arr.indexOf(k) !== i)
      if (dupes.length > 0) throw new Error(`Duplicate key: "${dupes[0]}"`)
      if (rows.some((r) => !r.key.trim())) throw new Error('All keys must be non-empty.')
      await save(rows)
    },
  })

  return (
    <div className="grid gap-4">
      {rows.length > 0 && (
        <div className="grid gap-2">
          {rows.map((row, i) => (
            <div key={i} className="flex items-center gap-2">
              <Input
                placeholder="key"
                value={row.key}
                onChange={(e) => setKey(i, e.target.value)}
                className="font-mono text-xs"
              />
              <Input
                placeholder="value"
                value={row.value}
                onChange={(e) => setValue(i, e.target.value)}
                className="font-mono text-xs"
              />
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="shrink-0"
                onClick={() => removeRow(i)}
              >
                <XIcon className="size-4" />
              </Button>
            </div>
          ))}
        </div>
      )}
      <Button type="button" variant="outline" size="sm" className="w-fit" onClick={addRow}>
        <PlusIcon className="size-4" />
        Add entry
      </Button>
      <PendingHint show={diff.length > 0} />
    </div>
  )
}

type AddressFormState = {
  firstName: string
  lastName: string
  company: string
  street1: string
  street2: string
  city: string
  state: string
  postalCode: string
  countryCode: string
  phone: string
}

const emptyAddress = (): AddressFormState => ({
  firstName: '', lastName: '', company: '', street1: '', street2: '',
  city: '', state: '', postalCode: '', countryCode: '', phone: '',
})

// Entity CRUD section: addresses are added/deleted through an explicit row form / destructive buttons, and "set
// default" is an entity-level action - all immediate by convention, not staged
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
  onAddAddress: (input: AddressFormState) => Promise<unknown>
  onDeleteAddress: (id: string) => Promise<unknown>
  saving: boolean
  canManage: boolean
}) {
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState<AddressFormState>(emptyAddress())
  const countryOptions = useQuery(CountriesPickerDocument).data?.countries ?? []
  const [err, setErr] = useState<string | null>(null)
  const [adding, setAdding] = useState(false)

  function set(field: keyof AddressFormState) {
    return (e: React.ChangeEvent<HTMLInputElement>) =>
      setForm((prev) => ({ ...prev, [field]: e.target.value }))
  }

  async function onSubmitAdd(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setErr(null)
    setAdding(true)
    try {
      await onAddAddress({
        ...form,
        company: form.company || '',
        street2: form.street2 || '',
        state: form.state || '',
        phone: form.phone || '',
      })
      setForm(emptyAddress())
      setShowForm(false)
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Failed to add address.')
    } finally {
      setAdding(false)
    }
  }

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

      {showForm && (
        <form onSubmit={onSubmitAdd} className="rounded border p-4 grid gap-3">
          <div className="grid grid-cols-2 gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="addr-first">First name *</Label>
              <Input id="addr-first" value={form.firstName} onChange={set('firstName')} required />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="addr-last">Last name *</Label>
              <Input id="addr-last" value={form.lastName} onChange={set('lastName')} required />
            </div>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="addr-company">Company</Label>
            <Input id="addr-company" value={form.company} onChange={set('company')} />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="addr-street1">Street line 1 *</Label>
            <Input id="addr-street1" value={form.street1} onChange={set('street1')} required />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="addr-street2">Street line 2</Label>
            <Input id="addr-street2" value={form.street2} onChange={set('street2')} />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="addr-city">City *</Label>
              <Input id="addr-city" value={form.city} onChange={set('city')} required />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="addr-state">State / region</Label>
              <Input id="addr-state" value={form.state} onChange={set('state')} />
            </div>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="addr-postal">Postal code *</Label>
              <Input id="addr-postal" value={form.postalCode} onChange={set('postalCode')} required />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="addr-country">Country *</Label>
              <Select
                value={form.countryCode || '__none__'}
                onValueChange={(v) => setForm((f) => ({ ...f, countryCode: v === '__none__' ? '' : v }))}
              >
                <SelectTrigger id="addr-country">
                  <SelectValue placeholder="Country" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="__none__">- None -</SelectItem>
                  {countryOptions.map((c) => (
                    <SelectItem key={c.code} value={c.code}>
                      {c.emojiFlag ? `${c.emojiFlag} ` : ''}
                      {c.name}
                      <span className="ml-1 text-xs text-muted-foreground">({c.code})</span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="addr-phone">Phone</Label>
            <Input id="addr-phone" type="tel" value={form.phone} onChange={set('phone')} />
          </div>
          <div className="flex gap-2 justify-end">
            <Button type="button" variant="outline" onClick={() => { setShowForm(false); setForm(emptyAddress()) }}>
              Cancel
            </Button>
            <Button type="submit" disabled={adding}>
              {adding ? 'Adding…' : 'Add address'}
            </Button>
          </div>
        </form>
      )}

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
                  <Button size="sm" variant="outline" className="h-7 text-xs" disabled={saving}
                    onClick={() => onSetDefault('billing', a.id)}>
                    Set billing default
                  </Button>
                )}
                {a.id !== defaultShippingId && (
                  <Button size="sm" variant="outline" className="h-7 text-xs" disabled={saving}
                    onClick={() => onSetDefault('shipping', a.id)}>
                    Set shipping default
                  </Button>
                )}
                <Button size="sm" variant="ghost" className="h-7 text-xs text-destructive hover:text-destructive"
                  disabled={saving} onClick={() => onDelete(a.id)}>
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


import { useRef } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { StoreIcon } from 'lucide-react'
import { DetailPageSection } from '@/components/layout/detail-page'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { useConfigScope } from '@/lib/config-scope'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { PASSWORD_HINT } from '@/shared/schemas/fields'
import { AdminCreateCustomerDocument } from './Customers.gql'
import { CustomerGroupsDocument } from './CustomerGroups.gql'
import { emptyNewCustomer, newCustomerSchema, toNewCustomerInput } from './schemas/newCustomer'

export function CustomerCreate() {
  const navigate = useNavigate()
  const { effectiveChannelId, selectedChannel } = useConfigScope()
  const groupsQuery = useQuery(CustomerGroupsDocument)
  const groups = groupsQuery.data?.customerGroups ?? []
  const defaultGroup = groups.find((g) => g.isDefault)

  // No list refetch - CustomersList reads its page with cache-and-network and picks the new customer up
  const [createCustomer] = useMutation(AdminCreateCustomerDocument)
  const createdId = useRef<string | null>(null)

  const record = useRecordForm({
    id: 'customer',
    title: 'Customer',
    label: 'New customer',
    isEdit: false,
    baseline: emptyNewCustomer,
    schema: newCustomerSchema,
    diff: () => [],
    // A typed password must never reach storage
    persist: { routeKey: `customer:new:${effectiveChannelId ?? ''}`, exclude: ['password'] },
    submit: async (v) => {
      if (!effectiveChannelId) throw new Error('Pick a channel in the header - customer accounts are scoped to a single store.')
      const result = await createCustomer({ variables: { input: toNewCustomerInput(v, effectiveChannelId, defaultGroup?.slug) } })
      createdId.current = result.data?.adminCreateCustomer.id ?? null
    },
    onSaved: () => navigate(createdId.current ? `/customers/${createdId.current}` : '/customers'),
  })
  const { form } = record

  return (
    <RecordFormPage
      record={record}
      backTo="/customers"
      backLabel="Back to customers"
      title="New customer"
      meta={
        <>
          <StoreIcon className="size-3.5" />
          <span>{selectedChannel?.name ?? <span className="italic">No store selected</span>}</span>
          <span>·</span>
          <span>Customer accounts are scoped per store</span>
        </>
      }
      createLabel="Create customer"
      actions={
        <Button type="button" variant="outline" size="sm" asChild>
          <Link to="/customers">Cancel</Link>
        </Button>
      }
    >
      {!effectiveChannelId && (
        <div className="px-6 pt-4">
          <Alert variant="destructive">
            <AlertTitle>Pick a channel</AlertTitle>
            <AlertDescription>
              Customer accounts are scoped per store. Switch the header from GLOBAL to a specific channel before creating a customer.
            </AlertDescription>
          </Alert>
        </div>
      )}

      <DetailPageSection title="Profile" description="The customer's identity and contact details.">
        <div className="grid grid-cols-1 items-start gap-4 sm:grid-cols-2">
          <form.AppField name="firstName">
            {(f) => <f.TextField id="c-first" label="First name" autoComplete="given-name" />}
          </form.AppField>
          <form.AppField name="lastName">
            {(f) => <f.TextField id="c-last" label="Last name" autoComplete="family-name" />}
          </form.AppField>
        </div>
        <form.AppField name="email">
          {(f) => (
            <f.TextField
              id="c-email"
              label="Email"
              type="email"
              autoComplete="off"
              hint="The same email on a different store creates a separate account."
            />
          )}
        </form.AppField>
        <form.AppField name="phone">
          {(f) => <f.TextField id="c-phone" label="Phone" type="tel" autoComplete="tel" className="sm:max-w-sm" />}
        </form.AppField>
      </DetailPageSection>

      <DetailPageSection title="Sign-in" description="Initial password and account flags.">
        <form.AppField name="password">
          {(f) => (
            <f.TextField
              id="c-password"
              label="Password"
              type="password"
              autoComplete="new-password"
              hint={`${PASSWORD_HINT}.`}
              className="sm:max-w-sm"
            />
          )}
        </form.AppField>
        <div className="grid gap-2">
          <form.AppField name="isActive">
            {(f) => <f.CheckboxField id="c-active" label="Active - inactive customers cannot sign in" />}
          </form.AppField>
          <form.AppField name="isEmailVerified">{(f) => <f.CheckboxField id="c-verified" label="Email verified" />}</form.AppField>
        </div>
      </DetailPageSection>

      <DetailPageSection title="Group" description="Customer group assignment for pricing and segmentation.">
        {groupsQuery.loading ? (
          <p className="text-sm text-muted-foreground">Loading groups…</p>
        ) : (
          <form.Field name="groupSlug">
            {(f) => (
              <div className="grid gap-2 sm:max-w-md" role="radiogroup" aria-label="Customer group">
                {groups.map((g) => (
                  <label
                    key={g.id}
                    className="flex cursor-pointer items-center gap-2 rounded border px-3 py-2 hover:bg-muted/50 has-checked:border-foreground/40 has-checked:bg-muted"
                  >
                    <input
                      type="radio"
                      name="customer-group"
                      value={g.slug}
                      // Nothing picked yet reads as the default group - that's what gets sent
                      checked={(f.state.value || defaultGroup?.slug) === g.slug}
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
      </DetailPageSection>
    </RecordFormPage>
  )
}

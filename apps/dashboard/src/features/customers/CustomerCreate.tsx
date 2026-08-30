import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useDraft } from '@/shared/saving/useDraft'
import { duringCommit } from '@/lib/nav-guard'
import { useMutation, useQuery } from '@apollo/client/react'
import { StoreIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { useConfigScope } from '@/lib/config-scope'
import { AdminCreateCustomerDocument } from './Customers.gql'
import { CustomerGroupsDocument } from './CustomerGroups.gql'

export function CustomerCreate() {
  const navigate = useNavigate()
  const { effectiveChannelId, selectedChannel } = useConfigScope()

  const groupsQuery = useQuery(CustomerGroupsDocument)
  const groups = groupsQuery.data?.customerGroups ?? []
  const defaultGroup = groups.find((g) => g.isDefault)

  const draft = useDraft({
    firstName: '',
    lastName: '',
    email: '',
    password: '',
    phone: '',
    isActive: true,
    isEmailVerified: false,
    groupSlug: '',
  }, 'New customer')
  const { firstName, lastName, email, password, phone, isActive, isEmailVerified, groupSlug } = draft.values
  const { setFirstName, setLastName, setEmail, setPassword, setPhone, setIsActive, setIsEmailVerified, setGroupSlug } = draft.setters
  const [submitError, setSubmitError] = useState<string | null>(null)

  // Seed the default group once it loads, unless the operator already picked one - Adjusting state during render
  // (guarded so it can't loop) instead of in an effect, per react.dev "You Might Not Need an Effect"
  if (!groupSlug && defaultGroup?.slug) setGroupSlug(defaultGroup.slug)

  // No list refetch - CustomersList reads its page with cache-and-network, so it picks the new
  // customer up on its own instead of dumping the whole table here
  const [createCustomer, { loading: creating }] = useMutation(AdminCreateCustomerDocument)

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setSubmitError(null)
    if (!effectiveChannelId) {
      setSubmitError('Pick a channel in the header - customer accounts are scoped to a single store.')
      return
    }
    try {
      // Send what was validated - the backend normalizes too, but the two should agree
      const result = await createCustomer({
        variables: {
          input: {
            firstName: firstName.trim(),
            lastName: lastName.trim(),
            email: email.trim(),
            password,
            phone: phone.trim() || null,
            isActive,
            isEmailVerified,
            customerGroupSlug: groupSlug || null,
            channelId: effectiveChannelId,
          },
        },
      })
      const newId = result.data?.adminCreateCustomer.id
      await duringCommit(() =>
        newId ? navigate(`/customers/${newId}`) : navigate('/customers'),
      )
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'Something went wrong.')
    }
  }

  const formReady =
    Boolean(effectiveChannelId) &&
    Boolean(firstName.trim()) &&
    Boolean(lastName.trim()) &&
    Boolean(email.trim()) &&
    password.length >= 8

  return (
    <form onSubmit={onSubmit}>
      <DetailPageLayout>
        <DetailPageHeader
          backTo="/customers"
          backLabel="Back to customers"
          title="New customer"
          meta={
            <>
              <StoreIcon className="size-3.5" />
              <span>
                {selectedChannel?.name ?? (
                  <span className="italic">No store selected</span>
                )}
              </span>
              <span>·</span>
              <span>Customer accounts are scoped per store</span>
            </>
          }
          actions={
            <>
              <Button type="button" variant="outline" size="sm" asChild>
                <Link to="/customers">Cancel</Link>
              </Button>
              <Button type="submit" size="sm" disabled={creating || !formReady}>
                {creating ? 'Creating…' : 'Create customer'}
              </Button>
            </>
          }
        />

        <DetailPageBody>
          {!effectiveChannelId && (
            <div className="px-6 pt-4">
              <Alert variant="destructive">
                <AlertTitle>Pick a channel</AlertTitle>
                <AlertDescription>
                  Customer accounts are scoped per store. Switch the header from GLOBAL to a
                  specific channel before creating a customer.
                </AlertDescription>
              </Alert>
            </div>
          )}

          <DetailPageSection
            title="Profile"
            description="The customer's identity and contact details."
          >
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="c-first">First name</Label>
                <Input
                  id="c-first"
                  required
                  autoComplete="given-name"
                  value={firstName}
                  onChange={(e) => setFirstName(e.target.value)}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="c-last">Last name</Label>
                <Input
                  id="c-last"
                  required
                  autoComplete="family-name"
                  value={lastName}
                  onChange={(e) => setLastName(e.target.value)}
                />
              </div>
            </div>

            <div className="grid gap-2">
              <Label htmlFor="c-email">Email</Label>
              <Input
                id="c-email"
                type="email"
                required
                autoComplete="off"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                The same email on a different store creates a separate account.
              </p>
            </div>

            <div className="grid gap-2 sm:max-w-sm">
              <Label htmlFor="c-phone">Phone</Label>
              <Input
                id="c-phone"
                type="tel"
                autoComplete="tel"
                value={phone}
                onChange={(e) => setPhone(e.target.value)}
              />
            </div>
          </DetailPageSection>

          <DetailPageSection
            title="Sign-in"
            description="Initial password and account flags."
          >
            <div className="grid gap-2 sm:max-w-sm">
              <Label htmlFor="c-password">Password</Label>
              <Input
                id="c-password"
                type="password"
                required
                minLength={8}
                autoComplete="new-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">Minimum 8 characters.</p>
            </div>

            <div className="grid gap-2">
              <div className="flex items-center gap-2">
                <Checkbox
                  id="c-active"
                  checked={isActive}
                  onCheckedChange={(v) => setIsActive(!!v)}
                />
                <Label htmlFor="c-active" className="cursor-pointer">
                  Active - inactive customers cannot sign in
                </Label>
              </div>
              <div className="flex items-center gap-2">
                <Checkbox
                  id="c-verified"
                  checked={isEmailVerified}
                  onCheckedChange={(v) => setIsEmailVerified(!!v)}
                />
                <Label htmlFor="c-verified" className="cursor-pointer">
                  Email verified
                </Label>
              </div>
            </div>
          </DetailPageSection>

          <DetailPageSection
            title="Group"
            description="Customer group assignment for pricing and segmentation."
          >
            {groupsQuery.loading ? (
              <p className="text-sm text-muted-foreground">Loading groups…</p>
            ) : (
              <div className="grid gap-2 sm:max-w-md">
                {groups.map((g) => (
                  <label
                    key={g.id}
                    className="flex cursor-pointer items-center gap-2 rounded border bg-card px-3 py-2 hover:bg-accent/50 has-[:checked]:border-primary has-[:checked]:bg-accent"
                  >
                    <input
                      type="radio"
                      name="customer-group"
                      value={g.slug}
                      checked={groupSlug === g.slug}
                      onChange={(e) => setGroupSlug(e.target.value)}
                      className="size-4"
                    />
                    <div className="flex-1">
                      <div className="text-sm font-medium">{g.name}</div>
                      <code className="text-xs text-muted-foreground">{g.slug}</code>
                    </div>
                    {g.isDefault && (
                      <span className="text-xs text-muted-foreground">default</span>
                    )}
                  </label>
                ))}
              </div>
            )}
          </DetailPageSection>

          {submitError && (
            <div className="px-6 py-4">
              <Alert variant="destructive">
                <AlertTitle>Couldn't create customer</AlertTitle>
                <AlertDescription>{submitError}</AlertDescription>
              </Alert>
            </div>
          )}
        </DetailPageBody>
      </DetailPageLayout>
    </form>
  )
}

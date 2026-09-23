import { useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { XIcon } from 'lucide-react'
import { FormDialog } from '@/components/form/FormDialog'
import { FormDialogBody } from '@/components/form/FormDialogBody'
import { FieldError } from '@/components/form/fields'
import { useDialogForm } from '@/components/form/useDialogForm'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { AdminCreateCustomerDocument, CustomersDocument } from '@/features/customers/Customers.gql'
import { AdminCreateCartDocument, CartsDocument } from './Carts.gql'
import { CustomerSearch } from './pickers'
import { emptyNewCart, newCartSchema, randomPassword, type NewCartValues } from './schemas/newCart'

// Creates a cart for a guest, an existing customer, or a brand-new customer (created inline), then
// opens the cart
export function NewCartDialog({
  open,
  onOpenChange,
  channelId,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelId: string | undefined
}) {
  return (
    <FormDialog open={open} onOpenChange={onOpenChange} className="sm:max-w-md">
      {(close) => <NewCartForm channelId={channelId} onClose={close} />}
    </FormDialog>
  )
}

function NewCartForm({ channelId, onClose }: { channelId: string | undefined; onClose: () => void }) {
  const navigate = useNavigate()
  const [createCart] = useMutation(AdminCreateCartDocument, {
    refetchQueries: channelId ? [{ query: CartsDocument, variables: { channelId, status: 'active' } }] : [],
  })
  const [createCustomer] = useMutation(AdminCreateCustomerDocument, { refetchQueries: [{ query: CustomersDocument, variables: {} }] })
  const createdId = useRef('')

  const dialog = useDialogForm({
    baseline: emptyNewCart,
    schema: newCartSchema,
    submit: async (v: NewCartValues) => {
      if (!channelId) throw new Error('Pick a channel in the header scope picker first.')
      let customerId: string | null = null
      if (v.mode === 'existing') customerId = v.customer?.id ?? null
      if (v.mode === 'new') {
        const res = await createCustomer({
          variables: {
            input: {
              channelId,
              email: v.email.trim(),
              firstName: v.firstName.trim(),
              lastName: v.lastName.trim(),
              password: randomPassword(),
            },
          },
        })
        customerId = res.data?.adminCreateCustomer.id ?? null
        if (!customerId) throw new Error('Customer creation returned no id.')
      }
      const res = await createCart({ variables: { channelId, customerId } })
      const id = res.data?.adminCreateCart.id
      if (!id) throw new Error('Cart creation returned no id.')
      createdId.current = id
    },
    onSaved: () => {
      onClose()
      navigate(`/carts/${createdId.current}`)
    },
  })
  const { form } = dialog
  const mode = useStore(form.store, (s) => s.values.mode)

  return (
    <FormDialogBody
      dialog={dialog}
      title="New cart"
      description={channelId ? 'Start a cart for a guest, an existing customer, or a new one.' : 'Pick a channel in the header scope picker first.'}
      submitLabel="Create cart"
      canSubmit={!!channelId}
      onCancel={onClose}
    >
      <form.Field name="mode">
        {(f) => (
          <Tabs value={f.state.value} onValueChange={(v) => f.handleChange(v as NewCartValues['mode'])}>
            <TabsList className="w-full">
              <TabsTrigger value="existing" className="flex-1">
                Existing
              </TabsTrigger>
              <TabsTrigger value="new" className="flex-1">
                New customer
              </TabsTrigger>
              <TabsTrigger value="guest" className="flex-1">
                Guest
              </TabsTrigger>
            </TabsList>
          </Tabs>
        )}
      </form.Field>

      {mode === 'existing' && (
        <form.Field name="customer">
          {(f) => (
            <div className="grid gap-1">
              {f.state.value ? (
                <div className="flex items-center justify-between gap-2 rounded-md border bg-muted/30 px-3 py-2 text-sm">
                  <span>{f.state.value.label}</span>
                  <Button type="button" variant="ghost" size="icon" className="size-7" onClick={() => f.handleChange(null)} aria-label="Clear">
                    <XIcon className="size-3.5" />
                  </Button>
                </div>
              ) : (
                <CustomerSearch onPick={(c) => f.handleChange({ id: c.id, label: `${c.firstName} ${c.lastName} · ${c.email}` })} />
              )}
              <FieldError field={f} />
            </div>
          )}
        </form.Field>
      )}

      {mode === 'new' && (
        <div className="grid gap-2">
          <form.AppField name="email">{(f) => <f.TextField id="nc-email" label="Email" type="email" />}</form.AppField>
          <div className="grid grid-cols-2 items-start gap-2">
            <form.AppField name="firstName">{(f) => <f.TextField id="nc-first" label="First name" />}</form.AppField>
            <form.AppField name="lastName">{(f) => <f.TextField id="nc-last" label="Last name" />}</form.AppField>
          </div>
          <p className="text-[10px] text-muted-foreground">A login is created with a random password; the customer can reset it later.</p>
        </div>
      )}

      {mode === 'guest' && (
        <p className="text-sm text-muted-foreground">Creates an unowned guest cart you can build and later finish or assign.</p>
      )}
    </FormDialogBody>
  )
}

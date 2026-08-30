import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation } from '@apollo/client/react'
import { XIcon } from 'lucide-react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { AdminCreateCustomerDocument, CustomersDocument } from '@/features/customers/Customers.gql'
import { AdminCreateCartDocument, CartsDocument } from './Carts.gql'
import { CustomerSearch } from './pickers'

type Mode = 'guest' | 'existing' | 'new'

// A strong throwaway password for an admin-created customer - they set their own
// via password reset - Satisfies typical complexity (length + mixed classes)
function randomPassword(): string {
  const bytes = new Uint8Array(18)
  crypto.getRandomValues(bytes)
  return Array.from(bytes, (b) => b.toString(36).padStart(2, '0')).join('') + 'Aa1!'
}

// NewCartDialog creates a cart for a guest, an existing customer, or a brand-new
// customer (created inline), then navigates to the cart detail
export function NewCartDialog({
  open,
  onOpenChange,
  channelId,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelId: string | undefined
}) {
  const navigate = useNavigate()
  const [mode, setMode] = useState<Mode>('existing')
  const [picked, setPicked] = useState<{ id: string; label: string } | null>(null)
  const [email, setEmail] = useState('')
  const [firstName, setFirstName] = useState('')
  const [lastName, setLastName] = useState('')
  const [error, setError] = useState<string | null>(null)

  const refetch = channelId ? [{ query: CartsDocument, variables: { channelId, status: 'active' } }] : []
  const [createCart, createState] = useMutation(AdminCreateCartDocument, { refetchQueries: refetch })
  const [createCustomer, custState] = useMutation(AdminCreateCustomerDocument, {
    refetchQueries: [{ query: CustomersDocument, variables: {} }],
  })
  const busy = createState.loading || custState.loading

  function reset() {
    setMode('existing')
    setPicked(null)
    setEmail('')
    setFirstName('')
    setLastName('')
    setError(null)
  }

  async function onSubmit() {
    if (!channelId) {
      setError('Pick a channel in the header scope picker first.')
      return
    }
    setError(null)
    try {
      let customerId: string | null = null
      if (mode === 'existing') {
        if (!picked) {
          setError('Pick a customer.')
          return
        }
        customerId = picked.id
      } else if (mode === 'new') {
        if (!email.trim() || !firstName.trim() || !lastName.trim()) {
          setError('Email, first name, and last name are required.')
          return
        }
        const res = await createCustomer({
          variables: {
            input: {
              channelId,
              email: email.trim(),
              firstName: firstName.trim(),
              lastName: lastName.trim(),
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
      onOpenChange(false)
      navigate(`/carts/${id}`)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to create the cart.')
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (v) reset()
        onOpenChange(v)
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>New cart</DialogTitle>
          <DialogDescription>Start a cart for a guest, an existing customer, or a new one.</DialogDescription>
        </DialogHeader>
        <div className="grid gap-3">
          <Tabs value={mode} onValueChange={(v) => setMode(v as Mode)}>
            <TabsList className="w-full">
              <TabsTrigger value="existing" className="flex-1">Existing</TabsTrigger>
              <TabsTrigger value="new" className="flex-1">New customer</TabsTrigger>
              <TabsTrigger value="guest" className="flex-1">Guest</TabsTrigger>
            </TabsList>
          </Tabs>

          {mode === 'existing' &&
            (picked ? (
              <div className="flex items-center justify-between gap-2 rounded-md border bg-muted/30 px-3 py-2 text-sm">
                <span>{picked.label}</span>
                <Button variant="ghost" size="icon" className="size-7" onClick={() => setPicked(null)} aria-label="Clear">
                  <XIcon className="size-3.5" />
                </Button>
              </div>
            ) : (
              <CustomerSearch onPick={(c) => setPicked({ id: c.id, label: `${c.firstName} ${c.lastName} · ${c.email}` })} />
            ))}

          {mode === 'new' && (
            <div className="grid gap-2">
              <div className="grid gap-1">
                <Label htmlFor="nc-email" className="text-xs">Email</Label>
                <Input id="nc-email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} />
              </div>
              <div className="grid grid-cols-2 gap-2">
                <div className="grid gap-1">
                  <Label htmlFor="nc-first" className="text-xs">First name</Label>
                  <Input id="nc-first" value={firstName} onChange={(e) => setFirstName(e.target.value)} />
                </div>
                <div className="grid gap-1">
                  <Label htmlFor="nc-last" className="text-xs">Last name</Label>
                  <Input id="nc-last" value={lastName} onChange={(e) => setLastName(e.target.value)} />
                </div>
              </div>
              <p className="text-[10px] text-muted-foreground">A login is created with a random password; the customer can reset it later.</p>
            </div>
          )}

          {mode === 'guest' && (
            <p className="text-sm text-muted-foreground">Creates an unowned guest cart you can build and later finish or assign.</p>
          )}

          {error && (
            <Alert variant="destructive">
              <AlertTitle>Failed</AlertTitle>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={busy}>Cancel</Button>
          <Button onClick={onSubmit} disabled={busy || !channelId}>{busy ? 'Creating…' : 'Create cart'}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

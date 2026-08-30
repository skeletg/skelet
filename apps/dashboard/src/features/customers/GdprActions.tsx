import { useState } from 'react'
import { useLazyQuery, useMutation } from '@apollo/client/react'
import { useNavigate } from 'react-router-dom'
import { DownloadIcon, ShieldAlertIcon } from 'lucide-react'
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
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { AnonymizeCustomerDocument, ExportCustomerDataDocument } from './Customers.gql'
import { useCustomerAccess } from './useCustomerAccess'

// GDPR data-subject controls: DSAR JSON export + irreversible
// anonymize-in-place (typed confirmation required) - both need customers.manage
export function GdprActions({ customerId, customerName }: { customerId: string; customerName: string }) {
  const navigate = useNavigate()
  const { canManage } = useCustomerAccess()
  const [runExport, { loading: exporting }] = useLazyQuery(ExportCustomerDataDocument, {
    fetchPolicy: 'network-only',
  })
  const [anonymize, { loading: anonymizing }] = useMutation(AnonymizeCustomerDocument)
  const [confirming, setConfirming] = useState(false)
  const [typed, setTyped] = useState('')
  const [error, setError] = useState<string | null>(null)

  async function onExport() {
    const res = await runExport({ variables: { customerId } })
    if (!res.data) return
    const blob = new Blob([res.data.exportCustomerData], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `customer-data-${customerId}.json`
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
  }

  async function onAnonymize() {
    setError(null)
    try {
      await anonymize({ variables: { customerId } })
      setConfirming(false)
      navigate('/customers')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Anonymization failed.')
    }
  }

  if (!canManage) return null

  return (
    <>
      <Button variant="outline" size="sm" onClick={onExport} disabled={exporting} title="GDPR data export (JSON) - profile, orders, consents, store credit and loyalty ledgers">
        <DownloadIcon className="size-4" />
        {exporting ? 'Exporting…' : 'Export data'}
      </Button>
      <Button
        variant="outline"
        size="sm"
        className="text-destructive hover:bg-destructive/10 hover:text-destructive"
        onClick={() => { setTyped(''); setError(null); setConfirming(true) }}
      >
        <ShieldAlertIcon className="size-4" />
        Anonymize
      </Button>

      <AlertDialog open={confirming} onOpenChange={setConfirming}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Anonymize {customerName}?</AlertDialogTitle>
            <AlertDialogDescription>
              This IRREVERSIBLY scrubs all personal data: profile fields, addresses, wishlist,
              carts, back-in-stock requests and analytics identities are removed, and their orders
              keep only redacted placeholders (notes, PO numbers and metadata included).
              The account can never sign in again. Type <strong>anonymize</strong> to confirm.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <Input value={typed} onChange={(e) => setTyped(e.target.value)} placeholder="anonymize" />
          {error && <p className="text-sm text-destructive">{error}</p>}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={anonymizing}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => { e.preventDefault(); void onAnonymize() }}
              disabled={typed !== 'anonymize' || anonymizing}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {anonymizing ? 'Anonymizing…' : 'Anonymize permanently'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

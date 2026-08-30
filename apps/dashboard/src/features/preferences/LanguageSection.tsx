import { useEffect, useState } from 'react'
import { Trans } from '@lingui/react/macro'
import { DetailPageSection } from '@/components/layout/detail-page'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { fetchAdminLocales, SOURCE_LOCALE, type AdminLocale } from '@/lib/i18n'
import { useLocalePref } from '@/lib/locale-pref'

// Only renders when at least one language pack is installed - an English-only install has no
// choice to offer, so it shouldn't grow a picker with a single entry
export function LanguageSection() {
  const [locales, setLocales] = useState<AdminLocale[]>([])
  const [locale, setLocale] = useLocalePref()

  useEffect(() => {
    let cancelled = false
    void fetchAdminLocales().then((list) => {
      if (!cancelled) setLocales(list)
    })
    return () => {
      cancelled = true
    }
  }, [])

  if (locales.length === 0) return null

  return (
    <DetailPageSection
      title={<Trans>Language</Trans>}
      description={
        <Trans>
          Languages beyond English come from installed language packs - anything a pack doesn't
          cover falls back to English.
        </Trans>
      }
    >
      <Select value={locale} onValueChange={setLocale}>
        <SelectTrigger className="w-72">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={SOURCE_LOCALE}>English</SelectItem>
          {locales.map((l) => (
            <SelectItem key={l.locale} value={l.locale}>
              {l.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </DetailPageSection>
  )
}

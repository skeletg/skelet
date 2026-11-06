import { ExternalLinkIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { DetailPageSection } from '@/components/layout/detail-page'
import { guideFor, PURPOSE_LABELS } from './platformGuides'
import type { Platform } from './status'

// Steps one and two of setting a platform up: what it does and where to go, then the consent every event needs
export function PlatformIntro({ platform, numbered = false }: { platform: Platform; numbered?: boolean }) {
  const guide = guideFor(platform.kind)
  const step = (n: number, title: string) => (numbered ? `${n}. ${title}` : title)
  return (
    <>
      <DetailPageSection title={step(1, 'What you need')}>
        <div className="grid max-w-2xl gap-3 text-sm">
          <p>{guide.intro}</p>
          {platform.fields.some((f) => f.required) && (
            <p className="text-muted-foreground">
              From {platform.label}: {platform.fields.filter((f) => f.required).map((f) => f.label).join(', ')}.
            </p>
          )}
          {guide.links.length > 0 && (
            <ul className="flex flex-wrap gap-3">
              {guide.links.map((l) => (
                <li key={l.url}>
                  <a href={l.url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-sm underline underline-offset-4">
                    {l.label}
                    <ExternalLinkIcon className="size-3" />
                  </a>
                </li>
              ))}
            </ul>
          )}
        </div>
      </DetailPageSection>
      <DetailPageSection title={step(2, 'Consent')}>
        <div className="grid max-w-2xl gap-2 text-sm">
          {platform.purposes.length === 0 ? (
            <p>Every event goes, whatever the visitor chose in the cookie banner.</p>
          ) : (
            <>
              <p className="flex flex-wrap items-center gap-2">
                An event goes only when the visitor allowed
                {platform.purposes.map((p) => (
                  <Badge key={p} variant="info">{PURPOSE_LABELS[p] ?? p}</Badge>
                ))}
              </p>
              <p className="text-muted-foreground">
                Visitors who refuse are still counted in Signals as nobody - no cookie, no identity - but nothing about them reaches {platform.label}.
              </p>
            </>
          )}
          {platform.necessarySetting && (
            <p className="text-muted-foreground">
              You can treat it as necessary on this channel under Settings → Privacy, so it needs no consent. Whether that is lawful where you sell is your decision.
            </p>
          )}
        </div>
      </DetailPageSection>
    </>
  )
}

import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useDraft } from '@/shared/saving/useDraft'
import { duringCommit } from '@/lib/nav-guard'
import { useMutation } from '@apollo/client/react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  DetailPageBody,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  CmsAttributesDocument,
  CreateCmsAttributeDocument,
  UpsertCmsAttributeContentDocument,
} from './Cms.gql'
import type { CmsInputType, CmsRelationshipTarget } from '@/graphql/schema.types'

const INPUT_TYPES: { value: CmsInputType; label: string }[] = [
  { value: 'TEXT', label: 'Text (single line)' },
  { value: 'TEXTAREA', label: 'Text (multi-line)' },
  { value: 'RICHTEXT', label: 'Rich text (Lexical)' },
  { value: 'NUMBER', label: 'Number' },
  { value: 'BOOLEAN', label: 'Boolean' },
  { value: 'DATE', label: 'Date' },
  { value: 'DATETIME', label: 'Date + time' },
  { value: 'URL', label: 'URL' },
  { value: 'SELECT', label: 'Select (one option)' },
  { value: 'MULTISELECT', label: 'Multi-select' },
  { value: 'ASSET', label: 'Media asset (one)' },
  { value: 'ASSETS', label: 'Media assets (many)' },
  { value: 'RELATIONSHIP', label: 'Relationship' },
  { value: 'JSON', label: 'JSON (free-form)' },
]

const RELATIONSHIP_TARGETS: CmsRelationshipTarget[] = [
  'CMS_PAGE',
  'BLOG_POST',
  'PRODUCT',
  'COLLECTION',
  'CATEGORY',
]

// CmsAttributeCreate is the standalone /cms/attributes/new create page (reached
// from the attributes table's "New attribute" button) - It captures the full
// definition (slug + input type are immutable after creation) and seeds the
// scoped-locale display name, then jumps to the detail editor
export function CmsAttributeCreate() {
  const navigate = useNavigate()
  const { has } = usePermissions()
  const canManage = has('cms.types_manage')
  const { scope, selectedChannel, globalDefaultLocale } = useConfigScope()
  const scopedLocale =
    scope.mode === 'CHANNEL'
      ? scope.locale || selectedChannel?.defaultLocale || globalDefaultLocale
      : globalDefaultLocale

  const draft = useDraft({
    slug: '',
    name: '',
    inputType: 'TEXT' as CmsInputType,
    translatable: false,
    requiredDefault: false,
    unit: '',
    target: '' as CmsRelationshipTarget | '',
  }, 'New page attribute')
  const { slug, name, inputType, translatable, requiredDefault, unit, target } = draft.values
  const { setSlug, setName, setInputType, setTranslatable, setRequiredDefault, setUnit, setTarget } = draft.setters
  const [submitError, setSubmitError] = useState<string | null>(null)

  const [createAttr, { loading: creating }] = useMutation(CreateCmsAttributeDocument, {
    refetchQueries: [{ query: CmsAttributesDocument }],
  })
  const [upsertContent] = useMutation(UpsertCmsAttributeContentDocument, {
    refetchQueries: [{ query: CmsAttributesDocument }],
  })

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setSubmitError(null)
    if (!slug.trim() || !name.trim()) {
      setSubmitError('Slug and name are required.')
      return
    }
    try {
      const res = await createAttr({
        variables: {
          input: {
            slug: slug.trim(),
            inputType,
            isTranslatable: translatable,
            isRequiredDefault: requiredDefault,
            unit: unit.trim() || null,
            relationshipTarget: inputType === 'RELATIONSHIP' && target ? target : null,
          },
        },
      })
      const created = res.data?.createCMSAttribute
      if (created) {
        await upsertContent({
          variables: {
            input: { attributeId: created.id, locale: scopedLocale, name: name.trim() },
          },
        })
        await duringCommit(() => navigate(`/cms/attributes/${created.id}`))
      } else {
        await duringCommit(() => navigate('/cms/attributes'))
      }
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'Create failed.')
    }
  }

  return (
    <form id="cms-attribute-create-form" onSubmit={onSubmit} className="flex flex-col">
      <div className="sticky top-0 z-10 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/80">
        <div className="flex flex-wrap items-center justify-between gap-3 px-6 py-3">
          <div className="min-w-0 flex-1">
            <h1 className="truncate text-lg font-semibold tracking-tight">New attribute</h1>
            <p className="mt-0.5 truncate text-xs text-muted-foreground">
              Custom field definition reused across page types. Slug + input type are immutable
              after creation.
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Button type="button" variant="outline" size="sm" asChild>
              <Link to="/cms/attributes">Cancel</Link>
            </Button>
            <Button
              type="submit"
              size="sm"
              disabled={creating || !canManage || !slug.trim() || !name.trim()}
            >
              {creating ? 'Creating…' : 'Create attribute'}
            </Button>
          </div>
        </div>
      </div>

      <DetailPageBody>
        {!canManage && (
          <div className="px-6 py-4">
            <Alert variant="destructive">
              <AlertTitle>Not permitted</AlertTitle>
              <AlertDescription>
                You don't have permission to create CMS attributes.
              </AlertDescription>
            </Alert>
          </div>
        )}
        {submitError && (
          <div className="px-6 py-4">
            <Alert variant="destructive">
              <AlertTitle>Create failed</AlertTitle>
              <AlertDescription>{submitError}</AlertDescription>
            </Alert>
          </div>
        )}

        <DetailPageSection
          title="Definition"
          description="Pick a slug, a display name for the current locale, and the input type. Add options to select-types on the next screen."
        >
          <div className="grid gap-4">
            <div className="grid gap-1.5">
              <Label htmlFor="attr-slug">Slug</Label>
              <Input
                id="attr-slug"
                value={slug}
                onChange={(e) => setSlug(e.target.value)}
                placeholder="hero_headline"
                disabled={!canManage}
                autoFocus
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="attr-name">Display name ({scopedLocale})</Label>
              <Input
                id="attr-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Hero headline"
                disabled={!canManage}
              />
            </div>
            <div className="grid gap-1.5">
              <Label>Input type</Label>
              <Select
                value={inputType}
                onValueChange={(v) => setInputType(v as CmsInputType)}
                disabled={!canManage}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {INPUT_TYPES.map((t) => (
                    <SelectItem key={t.value} value={t.value}>
                      {t.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            {inputType === 'RELATIONSHIP' && (
              <div className="grid gap-1.5">
                <Label>Relationship target</Label>
                <Select
                  value={target}
                  onValueChange={(v) => setTarget(v as CmsRelationshipTarget)}
                  disabled={!canManage}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="Select target…" />
                  </SelectTrigger>
                  <SelectContent>
                    {RELATIONSHIP_TARGETS.map((t) => (
                      <SelectItem key={t} value={t}>
                        {t}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}
            {inputType === 'NUMBER' && (
              <div className="grid gap-1.5">
                <Label htmlFor="attr-unit">Unit (optional)</Label>
                <Input
                  id="attr-unit"
                  value={unit}
                  onChange={(e) => setUnit(e.target.value)}
                  placeholder="px, %, kg…"
                  disabled={!canManage}
                />
              </div>
            )}
            <div className="flex items-center gap-2">
              <Switch
                id="attr-translatable"
                checked={translatable}
                onCheckedChange={setTranslatable}
                disabled={!canManage}
              />
              <Label htmlFor="attr-translatable">Translatable</Label>
            </div>
            <div className="flex items-center gap-2">
              <Switch
                id="attr-required"
                checked={requiredDefault}
                onCheckedChange={setRequiredDefault}
                disabled={!canManage}
              />
              <Label htmlFor="attr-required">Required by default</Label>
            </div>
          </div>
        </DetailPageSection>
      </DetailPageBody>
    </form>
  )
}

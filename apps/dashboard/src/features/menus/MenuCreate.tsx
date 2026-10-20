import { useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useMutation } from '@apollo/client/react'
import { Button } from '@/components/ui/button'
import { DetailPageSection } from '@/components/layout/detail-page'
import { slugify } from '@/lib/slug'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { CreateMenuDocument, MenusListDocument } from './Menus.gql'
import { NEW_MENU_LABELS, emptyNewMenu, newMenuSchema, toNewMenuInput } from './schemas/newMenu'

// The /menus/new create page (reached from the table's "New menu" button) - items are added on the
// menu's own page once it exists
export function MenuCreate() {
  const navigate = useNavigate()
  // UI state, not the admin's work - stays out of the form (and out of the stored draft)
  const [slugTouched, setSlugTouched] = useState(false)
  const [createMenu] = useMutation(CreateMenuDocument, { refetchQueries: [MenusListDocument] })
  const createdId = useRef<string | null>(null)

  const record = useRecordForm({
    id: 'menu',
    title: 'Menu',
    label: 'New menu',
    isEdit: false,
    baseline: emptyNewMenu,
    schema: newMenuSchema,
    diff: () => [],
    persist: { routeKey: 'menu:new' },
    submit: async (v) => {
      const res = await createMenu({ variables: { input: toNewMenuInput(v) } })
      createdId.current = res.data?.createMenu.id ?? null
    },
    onSaved: () => navigate(createdId.current ? `/menus/${createdId.current}` : '/menus'),
  })
  const { form } = record

  return (
    <RecordFormPage
      record={record}
      backTo="/menus"
      backLabel="Back to menus"
      title="New menu"
      meta="Admin-curated navigation menus for the channel header, footer, and any other slot."
      createLabel="Create menu"
      actions={
        <Button type="button" variant="outline" size="sm" asChild>
          <Link to="/menus">Cancel</Link>
        </Button>
      }
      labels={NEW_MENU_LABELS}
    >
      <DetailPageSection
        title="Basics"
        description="Pick a slug the channel will reference (e.g. main-nav, footer-shop) and a human-readable name. Items can be added on the next screen."
      >
        <div className="grid grid-cols-1 items-start gap-4 sm:grid-cols-2">
          <form.AppField
            name="name"
            listeners={{
              onChange: ({ value }) => {
                if (!slugTouched) form.setFieldValue('slug', slugify(value))
              },
            }}
          >
            {(f) => <f.TextField id="menu-name" label="Name" placeholder="Main Navigation" autoFocus />}
          </form.AppField>
          <form.AppField name="slug">
            {(f) => (
              <f.TextField
                id="menu-slug"
                label="Slug"
                placeholder="main-nav"
                onValueChange={() => setSlugTouched(true)}
                hint="Auto-filled from name - edit to override."
              />
            )}
          </form.AppField>
        </div>
      </DetailPageSection>
    </RecordFormPage>
  )
}

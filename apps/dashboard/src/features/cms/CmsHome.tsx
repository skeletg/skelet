import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { PlusIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { DetailPageBody, DetailPageHeader, DetailPageLayout } from '@/components/layout/detail-page'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import { NewPostDialog } from '@/features/blog/NewPostDialog'
import { CmsHomeBlogDocument, CmsHomeDocument, CmsHomeFormsDocument, type CmsHomeQuery } from './home/CmsHome.gql'
import { Board } from './home/Board'
import type { SitePage } from './home/site'
import { Toolkit, type Tool } from './home/Toolkit'

// How many pages the board draws. cmsPageCount says how many there are, so it knows when it was handed fewer
const PAGE_LIMIT = 500
function sitePages(rows: CmsHomeQuery['cmsPages'], channelId: string | undefined): SitePage[] {
  return rows.map((p) => ({
    id: p.id,
    urlPath: p.urlPath,
    title: p.content?.title.trim() || p.urlPath,
    isPublished: p.isPublished,
    scheduledAt: p.scheduledAt ?? null,
    workflowState: p.workflowState,
    updatedAt: p.updatedAt,
    home: !!channelId && p.channelListings.some((l) => l.channelId === channelId && l.isHomepage),
    typeId: p.pageTypeId,
    outline: p.content?.outline ?? [],
  }))
}

// The content hub: the way in to everything first, then the site as a board of pages drawn by their blocks
export function CmsHome() {
  const { has } = usePermissions()
  const { scope } = useConfigScope()
  const channelId = scope.mode === 'CHANNEL' && scope.channelId ? scope.channelId : undefined
  const { data, loading, error, refetch } = useQuery(CmsHomeDocument, {
    variables: { filter: { onlyPublished: false, ...(channelId ? { channelId } : {}) }, limit: PAGE_LIMIT },
  })
  const { data: blog, error: blogError } = useQuery(CmsHomeBlogDocument, { skip: !has('blog.view') })
  const { data: forms, error: formsError } = useQuery(CmsHomeFormsDocument, { variables: { channelId: channelId ?? '' }, skip: !channelId || !has('webforms.view') })

  const [writing, setWriting] = useState(false)
  const pages = sitePages(data?.cmsPages ?? [], channelId)
  const total = data?.cmsPageCount ?? pages.length
  const custom = new Map((data?.cmsBlockTypes ?? []).map((t) => [t.key, t.name]))
  const types = (data?.cmsPageTypes ?? []).map((t) => ({ id: t.id, name: t.content?.name.trim() || t.slug }))
  const tools: Tool[] = [
    { to: '/cms/page-types', title: 'Page types', about: 'What a kind of page holds: its fields and the blocks it starts with.', perm: 'cms.view', glyph: 'types', serves: 'pages', count: data?.cmsPageTypes.length },
    { to: '/cms/attributes', title: 'Page attributes', about: 'Fields that page types share.', perm: 'cms.view', glyph: 'attributes', serves: 'pages', count: data?.cmsAttributes.length },
    { to: '/cms/sections', title: 'Sections', about: 'Blocks saved once and reused across pages.', perm: 'cms.view', glyph: 'sections', serves: 'pages', count: data?.cmsSnippets.length },
    { to: '/cms/block-types', title: 'Block types', about: 'Blocks of your own: a set of fields and an HTML template.', perm: 'cms.view', glyph: 'blocks', serves: 'pages', count: data?.cmsBlockTypes.length },
    { to: '/blog/posts', title: 'Blog posts', about: 'Articles, with their drafts and their schedule.', perm: 'blog.view', glyph: 'posts', serves: 'blog', count: blog?.blogPosts.totalCount ?? undefined, failed: blogError?.message },
    { to: '/blog/taxonomy', title: 'Categories, tags and authors', about: 'How blog posts are filed and credited.', perm: 'blog.view', glyph: 'taxonomy', serves: 'blog' },
    { to: '/menus', title: 'Menus', about: "The storefront's navigation.", perm: 'cms.view', glyph: 'menus', serves: 'storefront', count: data?.menus.length },
    { to: '/cms/forms', title: 'Forms', about: 'Forms with conditional fields, placed on a page with a Form block.', perm: 'webforms.view', glyph: 'forms', serves: 'pages', count: forms?.webForms.length, failed: formsError?.message },
  ]

  return (
    <DetailPageLayout toc={false}>
      <DetailPageHeader
        title="CMS"
        meta={<span className="text-xs text-muted-foreground">The site's pages and how each is built, with the models, menus and blog behind them.</span>}
        actions={
          <div className="flex items-center gap-2">
            {has('blog.manage') ? (
              <Button type="button" variant="outline" size="sm" onClick={() => setWriting(true)}>
                New post
              </Button>
            ) : null}
            {has('cms.manage') ? (
              <Button asChild size="sm">
                <Link to="/cms/pages/new">
                  <PlusIcon />
                  New page
                </Link>
              </Button>
            ) : null}
          </div>
        }
      />
      <NewPostDialog open={writing} onOpenChange={setWriting} />
      <DetailPageBody className="divide-y-0">
        <div className="@container px-6 py-6">
          <div className="grid gap-6">
            <Toolkit pages={data ? pages : null} total={total} loading={loading} tools={tools.filter((t) => has(t.perm))} />
            {error ? (
              <Alert variant="destructive">
                <AlertTitle>The pages couldn't be loaded</AlertTitle>
                <AlertDescription className="grid justify-items-start gap-3">
                  {error.message}
                  <Button type="button" variant="outline" size="sm" disabled={loading} onClick={() => void refetch()}>
                    Try again
                  </Button>
                </AlertDescription>
              </Alert>
            ) : (
              <Board pages={data ? pages : null} types={types} custom={custom} canAdd={has('cms.manage')} total={total} />
            )}
          </div>
        </div>
      </DetailPageBody>
    </DetailPageLayout>
  )
}

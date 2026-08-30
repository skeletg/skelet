import { Link } from 'react-router-dom'
import {
  BlocksIcon,
  FileTextIcon,
  FormInputIcon,
  LayoutTemplateIcon,
  MenuIcon,
  NewspaperIcon,
  PuzzleIcon,
  SlidersHorizontalIcon,
  TagsIcon,
  type LucideIcon,
} from 'lucide-react'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { usePermissions } from '@/shared/auth/usePermissions'

type CmsCard = { to: string; icon: LucideIcon; title: string; desc: string; perm: string }

const PAGE_CARDS: CmsCard[] = [
  { to: '/cms/pages', icon: FileTextIcon, title: 'Pages', desc: 'Content pages built on your typed page models.', perm: 'cms.view' },
  { to: '/cms/page-types', icon: LayoutTemplateIcon, title: 'Page types', desc: 'Define page models and the fields they expose.', perm: 'cms.view' },
  { to: '/cms/attributes', icon: SlidersHorizontalIcon, title: 'Page attributes', desc: 'Reusable attributes shared across page types.', perm: 'cms.view' },
  { to: '/cms/sections', icon: PuzzleIcon, title: 'Sections', desc: 'Reusable block sections embedded into pages via a Reusable-section block.', perm: 'cms.view' },
  { to: '/cms/block-types', icon: BlocksIcon, title: 'Block types', desc: 'Custom page-builder blocks: a field schema plus an HTML template.', perm: 'cms.view' },
  { to: '/cms/forms', icon: FormInputIcon, title: 'Forms', desc: 'Custom forms with conditional fields, embedded on pages via the Form block.', perm: 'webforms.view' },
  { to: '/menus', icon: MenuIcon, title: 'Menus', desc: 'Storefront navigation menus.', perm: 'cms.view' },
]

const BLOG_CARDS: CmsCard[] = [
  { to: '/blog/posts', icon: NewspaperIcon, title: 'Posts', desc: 'Write and publish blog posts and articles.', perm: 'blog.view' },
  { to: '/blog/taxonomy', icon: TagsIcon, title: 'Categories, tags & authors', desc: 'Organize and attribute your blog content.', perm: 'blog.view' },
]

// CmsHome is the content hub: a single sidebar entry that gathers pages, page
// models, menus and the blog - Cards are gated to what the admin may access
export function CmsHome() {
  const { has } = usePermissions()
  const pages = PAGE_CARDS.filter((c) => has(c.perm))
  const blog = BLOG_CARDS.filter((c) => has(c.perm))

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="CMS"
        meta={<span className="text-xs text-muted-foreground">Pages, page models, menus and your blog - everything content.</span>}
      />
      <DetailPageBody>
        {pages.length > 0 && (
          <DetailPageSection title="Pages & navigation" description="Storefront content pages, the models behind them, and menus.">
            <CardGrid cards={pages} />
          </DetailPageSection>
        )}
        {blog.length > 0 && (
          <DetailPageSection title="Blog" description="Posts and the taxonomy that organizes them.">
            <CardGrid cards={blog} />
          </DetailPageSection>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}

function CardGrid({ cards }: { cards: CmsCard[] }) {
  return (
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      {cards.map((c) => (
        <Link
          key={c.to}
          to={c.to}
          className="group flex items-start gap-3 rounded-lg border p-4 transition-colors hover:border-primary/40 hover:bg-muted/40"
        >
          <span className="grid size-9 shrink-0 place-items-center rounded-md bg-primary/10 text-primary">
            <c.icon className="size-5" />
          </span>
          <span className="min-w-0">
            <span className="block font-medium group-hover:text-primary">{c.title}</span>
            <span className="mt-0.5 block text-xs leading-snug text-muted-foreground">{c.desc}</span>
          </span>
        </Link>
      ))}
    </div>
  )
}

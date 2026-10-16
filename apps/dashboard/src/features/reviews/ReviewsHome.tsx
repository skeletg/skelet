import { Link, useLocation } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'

import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ReviewCountsDocument } from './Reviews.gql'
import { ReviewsList } from './ReviewsList'
import { ReviewInsightsContent } from './ReviewInsights'
import { ReviewQuestionsTable } from './ReviewQuestionsTable'
import { ReviewCategoriesContent } from './ReviewCategoriesList'
import { ReviewModerationTagsContent } from './ReviewModerationTagsList'
import { Tile } from '@/components/layout/panels'

// ReviewsHome is the single tabbed shell for the whole Reviews area: the
// moderation queue, issue insights, the question library, categories, and
// moderation tags all live here under deep-linkable tabs (/reviews,
// /reviews/insights, …) - no separate sidebar pages - Detail/edit pages
// (/reviews/:id, /reviews/questions/:id) render standalone outside the shell

type TabKey = 'reviews' | 'insights' | 'questions' | 'categories' | 'tags'

const TABS: { key: TabKey; label: string; path: string }[] = [
  { key: 'reviews', label: 'Reviews', path: '/reviews' },
  { key: 'insights', label: 'Insights', path: '/reviews/insights' },
  { key: 'questions', label: 'Questions', path: '/reviews/questions' },
  { key: 'categories', label: 'Categories', path: '/reviews/categories' },
  { key: 'tags', label: 'Mod tags', path: '/reviews/tags' },
]

function activeTab(pathname: string): TabKey {
  if (pathname.startsWith('/reviews/insights')) return 'insights'
  if (pathname.startsWith('/reviews/questions')) return 'questions'
  if (pathname.startsWith('/reviews/categories')) return 'categories'
  if (pathname.startsWith('/reviews/tags')) return 'tags'
  return 'reviews'
}

export function ReviewsHome() {
  const { pathname } = useLocation()
  const active = activeTab(pathname)

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Reviews"
        meta={
          <span>
            Moderate customer reviews, track issues over time, and manage the questions, categories,
            and moderation tags that power them.
          </span>
        }
      />

      {/* manual activation: the triggers are links, so the URL decides the tab and arrow keys only move focus */}
      <Tabs value={active} activationMode="manual">
        <div className="px-6 pt-6">
          <TabsList>
            {TABS.map((t) => (
              <TabsTrigger key={t.key} value={t.key} asChild>
                <Link to={t.path}>{t.label}</Link>
              </TabsTrigger>
            ))}
          </TabsList>
        </div>

        <TabsContent value={active} className="mt-0">
          {active === 'reviews' && <ReviewsOverview />}
          {active === 'insights' && <ReviewInsightsContent />}
          {active === 'questions' && <ReviewQuestionsTable />}
          {active === 'categories' && <ReviewCategoriesContent />}
          {active === 'tags' && <ReviewModerationTagsContent />}
        </TabsContent>
      </Tabs>
    </DetailPageLayout>
  )
}

// ReviewsOverview is the default tab: exact status tiles + the moderation queue
function ReviewsOverview() {
  const { data } = useQuery(ReviewCountsDocument, { fetchPolicy: 'cache-and-network' })
  const counts = data?.reviewCounts
  const fmt = (n: number | undefined) => (n === undefined ? '-' : n.toLocaleString())

  return (
    <DetailPageBody>
      <DetailPageSection title="At a glance">
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <Tile label="Pending" value={fmt(counts?.pending)} />
          <Tile label="Approved" value={fmt(counts?.approved)} />
          <Tile label="Rejected" value={fmt(counts?.rejected)} />
          <Tile label="Total" value={fmt(counts?.total)} />
        </div>
      </DetailPageSection>

      <DetailPageSection title="Reviews">
        <ReviewsList />
      </DetailPageSection>
    </DetailPageBody>
  )
}

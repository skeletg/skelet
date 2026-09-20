# Code conventions

House rules for writing code here, per app. The dashboard's staged-save
convention is the one most code comments point at.

## Resolver hygiene

- Private resolver helpers (e.g. `customerAuthPayload`) live in
  `helpers.go` or `<domain>_helpers.go` inside the same surface dir -
  gqlgen strips unrecognized methods from `*.resolvers.go` on
  regeneration.
- Resolver field for the admin service is `AdminSvc` (not `Admin`) to
  avoid shadowing the generated `Admin(id)` query resolver method.
- Front model autobinds to `apps/backend/core/graph/admin/model` for shared
  types (Customer, Product, Order, …); cart-only types live in
  `apps/backend/core/graph/front/model`.
- Migrations in `apps/backend/core/internal/db/migrations/` - numbered
  sequentially. Each `<n>_<name>.up.sql` paired with
  `<n>_<name>.down.sql`.
- Both streams apply themselves on `docker compose up`: the `migrate` and
  `signals-migrate` one-shots run `up` and gate app/worker/media/signals/ingest
  on completing successfully, so nothing boots against a stale schema. Adding a
  migration means restarting the stack, not remembering to run `./skelet db migrate`.

## Dashboard hygiene

- `apps/dashboard/src/features/<domain>/*.gql.ts` - most are
  graphql-codegen output, but a few (e.g. `Categories.gql.ts`) are
  hand-written to share fragments. Don't blindly delete and regen the
  whole tree.
- `apps/dashboard/src/lib/apollo.ts` - silent refresh on 401, default
  fetch policy `network-only` (no cached reads).
- **Routes are code-split.** Pages register in `src/nav/routes.tsx` via
  `lazyPage(() => import('@/features/x/Page'), 'Page')` (see
  `src/nav/lazy.tsx`) - never import a page statically there, or it
  lands back in the eager bundle. The Suspense fallback lives in
  App.tsx.
- **Cross-cutting code lives in `src/shared/`** (auth, errors, saving) -
  `features/` is for actual product domains only.
- **Staged save is THE rule, not a pattern to opt into.** On any detail
  page, field edits must be staged locally and applied through the
  single page-header Save button, which opens `SaveConfirmDialog`
  (`src/shared/saving/SaveConfirmDialog.tsx`) listing every pending
  before→after change for confirmation. Never give an individual
  section/card its own Save button, and never fire a field mutation
  on change. Wiring: the page calls `useSaveRegistry()` and wraps its
  body in `SaveRegistryContext.Provider`
  (`src/shared/saving/saveRegistry.tsx`); each editable section
  registers a `(diff, save)` pair via `useSaveSection(...)`; the header
  Save opens the dialog with the dirty sections and confirming runs
  each `save()` sequentially. Reference implementations:
  `ProductDetail.tsx`, `AttributeDetail.tsx`, `CategoriesPage.tsx`
  (`productSaveRegistry.ts` re-exports the same registry under the names
  the product pages import).
  Allowed exceptions: entity CRUD rows/dialogs with explicit
  Save/Cancel (e.g. adding an attribute value, menu items,
  url-rewrite dialogs) - though any editors embedded inside such a
  form must flush with that form's Save, not carry their own button;
  destructive actions behind their own confirm dialog; lifecycle /
  non-field actions (publish, reindex, rotate token, order
  transitions); personal preferences (`/preferences`, nav pins, Home
  layout, table layouts) which write immediately because the control
  IS its own preview and there is no entity to diff - their
  destructive resets still carry a confirm. Diff entries must be a FAITHFUL projection of the
  staged draft or rely on `useSaveSection`'s call-time save thunk -
  the registry only re-registers when the diff STRING changes, so a
  save closure captured alongside a lossy diff (clipped text, rich
  text shown as 'previous → updated') would otherwise persist stale
  drafts. Also key draft-reset effects on entity IDENTITY
  (`[entity.id]`), never the refetched object, or unrelated refetches
  wipe staged edits mid-edit.
- **Diff entries follow the standard contract**
  (`src/shared/saving/diff.ts`). Build them with `changed(label,
  before, after)` / `added(label, summary)` / `removed(label,
  summary)` or, for form sections, `fieldDiff(original, draft,
  fields)` - never hand-roll placeholder rows. Rules: one entry = one
  human-readable change; the label names the thing (`Value · Deluxe ·
  price`); `before`/`after` are display strings (`display()`
  normalizes: '' ≡ null ≡ undefined, yes/no booleans, clipped text) -
  no '(edit)'/'(new)'/'(no change)' placeholders, no raw JSON, no bare
  IDs. Entries carry `kind: 'change' | 'add' | 'remove'` so
  SaveConfirmDialog renders +/− rows correctly. `changed()` returns
  null for equal values and the registry centrally DROPS no-op change
  entries (before === after, with a dev-console warning naming the
  section) - so a section that round-trips its form state can never
  light the Save button. If you see that warning, fix the section's
  diff computation rather than relying on the central filter.
- **Every page declares its unsaved work, in one of four ways.** A page
  that holds edits the admin could lose has to say so, or closing the
  tab, refreshing, switching channel/locale, closing a workspace tab and
  clicking a link all discard them silently. Everything lives in
  `src/lib/unsaved-changes.ts`, and which entry point you use is decided
  by what the baseline is, not by taste:
  1. **Staged-save pages get it free** - `useSaveRegistry()` registers
     for you, so a page following the staged-save rule above needs no
     extra wiring. This is the default; prefer it.
  2. **`useDraft(initialValues, label)`**
     (`src/shared/saving/useDraft.ts`) for a single form - create forms,
     and editors whose inner component takes its record as an `initial`
     prop. It OWNS the values, so dirt is complete by construction rather
     than depending on a field list somebody has to remember to extend.
     It replaces the `useState` pairs and generates setters named exactly
     like them (`setName`, `setSlug`), so field bindings in the JSX don't
     change:
     ```
     const draft = useDraft({ name: '', slug: '' }, 'New menu')
     const { name, slug } = draft.values
     const { setName, setSlug } = draft.setters
     ```
     An editor that stays on the page after saving must call
     `draft.saved()` on success to move the baseline, or it reads dirty
     until it remounts. `draft.reset()` throws edits away; `reset(next)`
     adopts a refetched record as the new baseline. Keep UI-only state
     (`slugTouched`, dialog flags, errors) out of the draft - it is for
     the admin's work, not the page's chrome.
  3. **`useUnsavedFields([...values], label)`** when the state genuinely
     cannot live in a draft object: xyflow owns the nodes and edges in
     `FlowBuilder`, `EmailLibraryDetail` keeps an undo history, and
     `PromotionRuleForm` has initializers that read values declared
     further down. Same mount-snapshot semantics, hand-listed.
  4. **`useUnsavedChangesGuard(isDirty, label)`** when the page computes
     dirt itself, which it must when state is re-seeded AFTER mount (see
     `MenuDetail.tsx`, which re-syncs from the query and so diffs against
     the loaded menu). A mount snapshot would read as dirty forever
     there.
  Pages that navigate after committing - every create form, and any
  delete that returns to a list - must wrap that navigation in
  `duringCommit()` (`src/lib/nav-guard.ts`), or the work they just saved
  gets a "discard changes?" prompt on the way out. `ConfirmDialog` and
  `useSaveRegistry().saveAll()` already do this for you.
  `src/lib/unsaved-work-coverage.test.ts` enforces this: it reads the
  route table and fails when a detail page stages and commits edits
  without declaring them, which is how the product page silently lost
  edits for a while. Genuine exceptions go in its `NO_STAGED_EDITS` map
  with a reason.

## Storefront hygiene

- **Next.js 16, not your training data.** Read
  `apps/channels/vetrina-demo/node_modules/next/dist/docs/` before writing code;
  APIs, conventions, and file layout differ from older versions. Heed
  deprecation notices.
- **Server-only GraphQL.** `app/lib/graphql.ts` is RSC/route-handler
  only - the browser can't reach `BACKEND_INTERNAL_URL`. If you need
  client-side data, expose it via a Next.js route handler instead of
  leaking the internal URL into the client bundle.
- **Force-dynamic everywhere.** `app/layout.tsx` sets
  `export const dynamic = "force-dynamic"` and `gqlFetch` uses
  `cache: "no-store"` - every request hits the backend. Don't introduce
  Full Route Cache / ISR without a deliberate per-route decision.
- **Operations are colocated.** `*.graphql` files live next to their
  consumers under `app/lib/` (or feature dirs); codegen produces
  `*.gql.ts` siblings. The shared baseline is
  `app/graphql/schema.types.ts` (DO NOT EDIT).
- **Channel + locale are server-resolved.** `getChannel()`
  (`app/lib/channel/read.ts`) and
  `getCurrentLocale()`/`getCurrentChannelId()` (`app/lib/locale/read.ts`)
  are the source of truth; pass results down via the `ChannelProvider`
  for client components.
- **JWT in cookies, not localStorage.** `app/lib/auth.ts` reads
  customer tokens from the cookie store; `gqlFetch` attaches
  `Authorization: Bearer <token>` automatically when present.
- **URL routing is backend-driven.** The catch-all `[...slug]` route
  resolves arbitrary paths via the front `urlResolver` query
  (`app/lib/url-resolver.graphql`) so canonical URLs + 301s defined in
  `internal/urlrewrite/` work end-to-end.
- **HMR over bind mounts.** If file changes don't trigger reloads,
  confirm `CHOKIDAR_USEPOLLING` / `WATCHPACK_POLLING` are still set in
  `docker-compose.yml` (inotify is unreliable on WSL2/macOS).

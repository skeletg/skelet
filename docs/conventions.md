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
- Migrations in `apps/backend/core/internal/db/migrations/` - one dbmate file
  each, `<timestamp>_<name>.sql` from `skelet db migrate new <name>`, holding its
  `-- migrate:up` and `-- migrate:down` halves.
- Every stream applies itself: the processes that open a database migrate it as
  they boot, before anything reads it, so nothing runs against a stale schema. Adding
  a migration means restarting the app, not remembering to run `skelet db migrate` -
  see [database.md](database.md).

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
- **State colours are tokens, never raw Tailwind hues.** `index.css`
  defines `success`, `warning`, `info` and `destructive` outside the
  palettes and registers each in `@theme`, so they are plain utilities
  that flip with the mode on their own - no `dark:` twin at the call
  site. Each state has three roles:

  | Utility | Use |
  |---|---|
  | `text-success`, `bg-success`, `border-success`, `ring-success` | the colour itself: text, icons, dots, bars, strong lines |
  | `text-success-foreground` | text on a solid `bg-success` fill |
  | `bg-success-soft` | tinted badge or banner background |

  A tinted badge is `bg-warning-soft text-warning border-warning/30`,
  or `<Badge variant="warning">`; `Alert` has the same variants. Opacity
  modifiers (`border-info/40`) cover one-offs. Never use `primary` /
  `accent` for state - admins recolour them. Categories with no state
  (a kind badge, an order status like Processing) map to the nearest
  state or to `muted`, not to a raw hue like `bg-violet-500`. A new
  colour goes in as a CSS variable plus a `--color-*` entry in `@theme`.
  Chart series and legend dots that only tell categories apart use
  `series-1…8` and `series-other` (`bg-series-3` in markup;
  `SERIES` / `SERIES_OTHER` / `CHART_ACCENT` from `src/lib/chart-colors.ts`
  in recharts props) - never a hex.
  The auth screens' fixed white-card chrome is the one deliberate
  exception.
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
- **Every page declares its unsaved work, in one of five ways.** A page
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
  5. **Record forms** (`useRecordForm` + `RecordFormPage`, both in
     `src/shared/saving/`) are the default for create and edit pages, for new
     ones and for existing ones when they're next touched. The page passes a
     zod schema, a memoised `baseline`, a `diff`, a `persist` key and its
     mutations. The hook wires the rest: the save registry, a TanStack form
     (`useAppForm`, `src/components/form/form.ts`), the stored draft, the
     section the header Save reviews, and the confirm dialog. `RecordFormPage`
     renders the header with the one Save button, the stored-draft banner,
     the create-failure alert and `SaveConfirmDialog`, so the page file is only
     its sections. Bind fields with
     `<form.AppField name="x">{(f) => <f.TextField … />}</form.AppField>`;
     `TextField`, `SwitchField`, `CheckboxField` and `MetadataField` are
     registered. A save rejects when validation fails or `submit` throws, so
     the dialog stays open with the reason, and `fieldErrorsFrom` can route a
     server message to the field it's about.
     - **Schemas** live in `features/<domain>/schemas/<entity>.ts`: the zod
       schema, its `Values` type, `to<Entity>Values(record)`,
       `to<Entity>Input(values)`, the field labels and the diff, with a
       `<entity>.test.ts` beside it. One schema serves both create and edit.
       Key/value metadata uses `src/shared/schemas/metadata.ts`, so it's
       always diffed, one entry per key.
     - **Drafts** (`persist`, `src/lib/form-drafts.ts`) keep the admin's work
       in localStorage, keyed admin → workspace tab → page, with a 7-day TTL.
       After a refresh or a closed browser the draft is offered, never applied
       on its own: the banner says one was found, Review lists what it would
       change (the Save dialog's rows, plus any field someone else changed
       since), and Restore puts it back into the form, which still needs Save.
       Editing instead of deciding drops the offer. Refresh, close and scope
       switches stop prompting; closing the workspace tab still does, and
       deletes the draft. Dialogs pass `persist: false`. List secrets in
       `exclude`, because they must never reach storage. Other staged
       sections on the same page (a values table, the registry's config
       fields) keep no draft, so while one of them is dirty the refresh
       prompt comes back - the registry skips it only when every dirty
       section is persisted.
     - **Every migrated page gets a page test** (`<Page>.test.tsx`, Apollo
       `MockedProvider`): edit, check that Save enables, check what the
       dialog lists, confirm, and assert the mutation's variables. A section
       registered in the same component as `useSaveRegistry()` needs the
       registry passed explicitly (`useSaveSection(reg, registry)`), because
       context can't reach the component that renders the Provider. Several
       pages lost their Save button that way.
     Reference: `features/inventory/InventoryLocationForm.tsx`.
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

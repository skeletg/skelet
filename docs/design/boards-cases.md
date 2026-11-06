# Boards as owned, shareable cases

This is the design for turning Boards (PR #210, shared per shop) into boards that are owned, private by default and
shareable, and that can carry a subject such as "sku = X" or "campaign Y" that every report on the board follows.
Reports are built inside the board, so nobody has to jump between screens. Each section is one stage, written and
agreed before the next one starts.

Decided so far:
- Reports are built and edited in a side sheet beside the board, and the tile being edited updates live.
- The grid has 4 columns, or 5 from about 1680px wide. Tiles span 1, 2 or 4 columns.
- The Synapse report pages stay as a scratch area, built from the same components.
- Saving is staged, per [conventions.md](../conventions.md).
- Drafts are kept on the server until they are saved and shared.

## 1. Top tasks and the journey

### The questions a board must answer

| # | Question | What a board for it holds | Answerable today? |
|---|---|---|---|
| Q1 | How is product X doing? | views → cart adds → orders → revenue for one SKU, over time | **Partly.** `page_view`, `view_item` and `cart_add` carry `sku`. `order_placed` has no lines or SKU, so orders and revenue for X are missing |
| Q2 | Did campaign or flow Y work? | sent → opened → clicked → attributed orders and revenue, for one campaign or flow | **Partly.** `email_*` and `conversion` carry `campaign_id` and `flow_id`, and `conversion` carries `revenue_cents`. Only the funnel can filter on them, and nothing can sum revenue |
| Q3 | Where do shoppers drop off? | the checkout funnel by device or source, plus the paths after the step where shoppers drop off | **Mostly.** The funnel has a breakdown and step filters. Flow can't start from "did step N but not step N+1" |
| Q4 | Do buyers come back? | repeat-order retention by cohort, optionally for buyers of X | **Partly.** Retention works for the whole shop. It can't be narrowed to buyers of X (no filter, and no SKU on orders) |

Two gaps show up in every question:
- **There's no sum measure.** Insights counts events (`total`) or people (`unique`). Revenue, from `conversion.revenue_cents`
  or `order_placed.total_cents`, can't be put on a tile.
- **Only the funnel takes a property filter.** Insights, Engagement, Retention and Flow can't follow a board's subject
  (stage 3).

### The journey, step by step

Each competitor step comes from the vendor's docs. The chrome-devtools browser failed to start ("Target closed") during
this stage, so nothing was clicked through. Skelet's steps today come from the code
(`features/boards/*` and `features/synapse/*`).

| Step | Mixpanel | Amplitude | PostHog | GA4 Explorations | Skelet today | Skelet proposed |
|---|---|---|---|---|---|---|
| **Create a report** | Built in its own report view, then saved to a board | Built as a chart with its own owner, then added to a dashboard | Built from the dashboard ("Add a chart" → "Build it myself" → "Save & add to dashboard") or from the insight page | Built inside the exploration: Variables, Tab settings and Canvas sit side by side, and every tab is a technique | Built on its own page (`/synapse/insights` etc.), with settings in local state that are lost on navigation | Built in the board's side sheet, with the tile previewing live. The scratch pages use the same controls |
| **Add to a board** | Adding links the report to the board, and the report inherits the board's permissions | Charts are added to a dashboard. The chart keeps its own owner and permissions | "Add to dashboard". One insight can sit on many dashboards and isn't copied | Not needed: a tab lives inside the exploration | "Add to board" → dialog → navigate to the board → Save. That's 4 screens, and a tile can't be edited later | "+ Add report" on the board, or a question template. On a scratch page, "Add to board" opens the board with the sheet already open |
| **Set the board's subject** | Board-level filters | Dashboard filter bar: date, interval, property and segment, applied to every chart | Dashboard date and property filters. Each tile can override them or "Ignore dashboard filters" | Segments and filters apply per tab, and there's no exploration-wide filter | None. Period is the only board-level control | A subject chip in the board header (`sku = X`, campaign, flow). Each tile shows whether it follows the subject, and a tile can opt out |
| **Share** | Per board, Viewer or Editor. Reports inherit the board's permissions. Public boards are off by default | Per dashboard, "Can view" or "Can edit" (edit makes them co-owners). Admins can change the owner | "Share" restricts who can edit. There are public links. No private mode is documented | Private when created. "Share exploration" gives read-only access, and to edit you duplicate it | Everyone with `boards.view` on the shop sees every board | Private draft → Save → Share with an admin or a role, as view or edit (stage 2) |
| **Revisit** | Search and favourites | Owned-by-me and shared lists | Dashboards list and pinned dashboards | Explorations list, owned and shared | Boards list for the shop | "Favourites", "Mine", "Shared with me" and "My drafts". An unsaved draft is waiting where you left it |

### What the four tools agree on, and what we take

1. **The board is where people work.** PostHog and GA4 let you build from the board or exploration itself, and
   Mixpanel and Amplitude let you edit a tile from the board. We do the same, with the side sheet.
2. **Board filters apply to every tile, and each tile can opt out.** PostHog's "Override tile filters" and "Ignore
   dashboard filters" is the clearest version. Our subject works the same way.
3. **Sharing is per board, with two levels (view and edit).** Nobody exposes more than two. Mixpanel and Amplitude
   both treat edit as co-ownership.
4. **Linked vs copied reports split the field.** Mixpanel and PostHog link one report to many boards, while GA4 copies.
   The decision comes in stage 2. A copy is simpler, and a linked report only pays off if one report has to stay the
   same across boards.
5. **Drafts are a gap everywhere.** None of the four documents draft persistence. PostHog's layout edit mode is
   E to save and Escape to discard. Keeping drafts on the server is a place where we can do better than all of them.

### Our journey today: where it breaks

- **Four screens to add one report**, and the report's settings live only in the URL-less page state.
- **A saved tile is frozen.** To change a report you rebuild it on its page and add it again.
- **There's no subject**, so a "product X" board is impossible except as a funnel with a filter on every step.
- **There's no revenue tile** (no sum measure).
- **There's no notion of mine or shared.** Every board belongs to the whole shop.
- **A viewer without `customers.view`** opens a board where every tile is an error. Boards gate on `boards.view`,
  but Synapse reads gate on `customers.view`.
- **The fixed 2-column grid** fits only 2 tiles across, so a board for one question doesn't fit on one screen.

### Answers (2026-11-04)

1. **Copied.** A report added to a second board is its own copy. Editing it on one board never changes another.
   "Duplicate to board…" is the action.
2. **Base currency.** Revenue tiles convert to the shop's base currency (stage 3 checks where the rates come from).
3. **Several conditions at once.** A subject is a list of conditions joined with AND (stage 3).
4. **Templates aren't needed for now.** Today's four starter boards stay.

Sources (stage 1): [PostHog dashboards](https://posthog.com/docs/product-analytics/dashboards),
[Mixpanel sharing and permission](https://docs.mixpanel.com/docs/boards/sharing-and-permission),
[Amplitude sharing dashboards](https://amplitude.com/docs/analytics/share-dashboards),
[Amplitude dashboard filters](https://amplitude.com/docs/analytics/dashboard-filter),
[GA4 Explorations](https://support.google.com/analytics/answer/7579450).

## 2. Objects and access

### What exists to build on

- **Who the caller is reaches Signals already.** `signalsclient.Client.Authed` signs the admin's id, superuser flag
  and Signals grants into the service token on every call. The token doesn't carry the admin's **role ids** yet, so
  they have to be added to it.
- **Roles** are `admin_roles` plus `admin_user_roles`. There are no groups, so roles are what you share with.
- **Listing admins or roles** (`admins`, `roles`) needs the global `admins.view` permission, which most board owners
  won't have. Sharing needs a narrower lookup of its own.
- **Signals never sees an admin being deleted.** `DeleteAdmin` is core-only.
- **Report reads already run as the viewer.** Every tile calls the Synapse query with the viewer's own session, gated
  on `customers.view`.

### The objects

| Object | Where | Shape |
|---|---|---|
| Board | `boards` (Signals) | adds `owner_admin_id uuid NULL`, `status draft\|published` and `owner_removed_at` |
| Share | `board_shares` (new) | `board_id`, exactly one of `admin_id` or `role_id`, `level view\|edit`, `created_by`. Unique on (board, target) |
| Favourite | `board_favourites` (new) | `admin_id`, `board_id`. Your favourites are your own, and nobody else sees them |
| Draft | `board_drafts` (new) | `board_id`, `admin_id`, `payload jsonb` (name, subject, reports), `base_updated_at`, `updated_at` |
| Report | inside the board, as now | A copy per board (answer 1). Gains `size` and an opt-out from the subject |

**A shop board is a board with no owner.** That's what the four starter boards and every board from #210 already
are, so **#210 needs no data migration**: those rows keep `owner_admin_id = NULL` and keep behaving the way they do
today. Only boards created from now on get an owner and start private.

### Who can do what

Every rule below also needs `boards.view` on the board's channel, so channel scoping works the same as everywhere else.

| Action | Owner | Edit share (admin or role) | View share | Shop board, `boards.manage` | Shop board, `boards.view` | Superuser |
|---|---|---|---|---|---|---|
| See it in the list and open it | ✓ | ✓ | ✓ | ✓ | ✓ | ✓, under "All boards" only |
| Edit reports, name and subject | ✓ | ✓ | – | ✓ | – | ✓ |
| Share and unshare | ✓ | – | – | ✓ | – | ✓ |
| Delete, or hand over to a new owner | ✓ | – | – | ✓ | – | ✓ |
| Duplicate to a board of my own | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

Creating a board needs `boards.manage` on the channel, and the creator becomes its owner.

- **An edit share isn't co-ownership.** The person can change what the board shows, but not who sees it. That's
  narrower than Amplitude's "Can edit", on purpose: whoever shared the board stays in control of who sees it.
- **Role shares are live.** Role ids come with every call, so someone added to the Marketing role sees Marketing's
  boards on their next request, and someone removed stops seeing them.
- **A view share doesn't stop people exploring.** A viewer can change the period or subject for their own session
  (that isn't saved), or "Duplicate to my board" to keep their changes. This is how GA4 handles it.
- **Superusers** see private boards only under an "All boards" filter, never in their default list. This keeps
  governance possible (taking over an orphaned board, for example) without the superuser's own list filling up
  with other people's work.

### Drafts (your note)

- **A new board is a draft.** It's created with `status = draft` the first time something is added, and only its
  creator sees it, under "My drafts". It can't be shared. Its first Save publishes it.
- **Unsaved edits to a published board** go in `board_drafts`, one row per admin per board, and are written about
  1 second after the last change. When you come back to the board, it opens with "You have unsaved changes from
  Tuesday 14:02 – Review & save / Discard".
- **Save is still the single, staged step** (conventions.md). The confirm dialog diffs the draft against the
  published board. If someone else saved since the draft started (`base_updated_at` no longer matches), Save is
  refused with today's stale error, and the dialog shows what changed on both sides.
- **This is an exception to "never fire a mutation on change", and conventions.md will say so.** Writing a draft
  changes nothing anyone else sees. It only keeps your staged work.
- **Drafts are kept for 30 days.** A nightly sweep deletes older ones, and the draft banner says the date.

### Sharing with someone who can't see the data

- **The share dialog warns before you share.** For each person or role it checks `boards.view` and `customers.view`
  on the board's channel, and says, for example, "Marketing can't see customer analytics on EU – they'll see the
  board's layout without numbers."
- **The tile tells the viewer what's missing.** Without `customers.view`, a tile says "You need Customer analytics
  on EU to see this report" instead of showing an error. The layout, titles and subject still show.
- **Nothing is shown with the owner's rights.** That would leak data. The board stores only definitions, never
  results.
- **The share picker gets its own query.** `boardShareTargets(channelId)` is gated on `boards.manage`, and returns
  only the id and display name of each admin and role that has `boards.view` on that channel. It never returns
  emails or grants.

### When the owner leaves

- **A deactivated owner** keeps the board. Its shares keep working, and nothing moves.
- **A deleted owner:** core sends `AdminRemoved(admin_id)` to Signals over the outbox, the same way channels are
  pushed. Signals stamps `owner_removed_at` on their boards, drops their drafts and favourites, and deletes their
  admin shares.
- **An orphaned board can be claimed.** It shows as "Owner removed – claim" to anyone with `boards.manage` on the
  channel who could already open it, and to superusers. Claiming makes them the owner. Nothing is deleted
  automatically.

### What changes in #210's code

- **Proto:** `BoardSummary` gains `owner`, `status`, `my_access`, `favourite` and `has_my_draft`. New calls:
  `ShareBoard`, `UnshareBoard`, `SetBoardFavourite`, `SaveBoardDraft`, `DiscardBoardDraft`, `TransferBoard` and
  `AdminRemoved`.
- **Core:** the service token gains `RoleIDs`, there's the `boardShareTargets` query, and `DeleteAdmin` publishes
  `AdminRemoved` to the outbox.
- **Signals:** `board.Service` takes an `Actor{AdminID, RoleIDs, Superuser}` on every method. One function,
  `access(board, actor) → none|view|edit|own`, answers every check, and the access matrix test is written against it.
- **Dashboard:** the board list gets the tabs Favourites, Mine, Shared with me, Shop boards and My drafts. There's a
  Share dialog with the warning, a star in the board header, the draft banner, and the tile's no-permission state.

### Answers (2026-11-04)

1. **Yes.** Someone with `boards.manage` can make a board a shop board, which clears the owner and makes it visible
   to the whole channel.
2. **No.** An edit share can't share the board on. Only the owner, a shop-board manager or a superuser shares.
3. **Superusers see private boards**, under "All boards".
4. **Drafts are kept for 30 days.**

## 3. The board as a case

A board's **subject** is a list of conditions joined with AND, such as `sku = TSHIRT-RED` and
`utm_source = instagram`, or `campaign_id = <Y>`. It is stored on the board (and in drafts), never on its reports,
and every report on the board follows it unless that report opts out.

### What a condition applies to

**A condition filters only the events that carry its property.** An event that never carries the property is left
alone. Each tile then shows which of its events follow the subject: "Follows sku on view_item, cart_add, order_line".

That one rule answers all four questions without any setting per tile:

| Question | Tile | What the subject does |
|---|---|---|
| Q1 How is product X doing? | Funnel view_item → cart_add → order_line | every step carries `sku`, so every step is filtered |
| Q1 | Revenue: Insights, net sales (order_line minus return_line) | only X's lines are summed |
| Q2 Did campaign Y work? | Funnel email_sent → opened → clicked → conversion | every step carries `campaign_id` |
| Q2 | Attributed revenue: sum of `revenue_base_cents` on conversion | only Y's conversions |
| Q3 Where do shoppers drop off? | Funnel with a device breakdown, and Flow after begin_checkout | a `utm_source` subject filters the page_view steps. begin_checkout doesn't carry it and is left alone |
| Q4 Do buyers come back? | Retention: start order_line, return order_placed | the start carries `sku`, so the cohort is buyers of X. The return doesn't, so any later order counts |

**Whether an event "carries" a property is declared, not guessed.** Browser events are declared in the tracking plan
(`analytics/plan.go`). Server events get the same declaration in a new `analytics.ServerEventKeys`, filled from the
shapes the tracker writes. A declaration is necessary: `page_view` carries `sku` only on product pages, and
"`page_view` with sku = X" must mean X's product pages, not every page that lacks a sku. `skelet gen` adds the server
events to the tracking-plan doc.

**Exceptions:**
- **Flow** filters only its focus event, so the people are those who did the focus event on X. The paths after it stay
  whole, because cutting other products' events out of a journey would invent paths that never happened.
- **Engagement (Stickiness)** filters the event it counts, by the rule above.
- **A tile can opt out** with "Ignore the board's subject" (PostHog's "Ignore dashboard filters"). For example, a
  "whole shop" tile next to X's tiles for comparison.

### What each engine can do today, and what's missing

| Engine | Property filter today | Needed |
|---|---|---|
| Funnel | yes: `FunnelQuery.Filters{Step, Key, Value}` with `propText = ?` | the subject added to each step that carries the property. Today's step filters stay |
| Insights | no | `filters []PropertyFilter` ANDed into its one `WHERE`, plus a **sum measure** (`measure = sum`, `sumKey`) |
| Engagement (Stickiness) | no | the same `filters` in its `WHERE` |
| Retention | no | separate filters for the `anchors` and `returns` CTEs (a start filter and a return filter) |
| Flow | no | filters on the `focus_actors` CTE only |

All of them reuse the funnel's `propText` expression, so a number compares the same way in every engine.
`PropertyFilter{Key, Op, Value}` starts with `Op = eq` only. The type has room for `neq` and `in` later.

### Events that are missing

**`order_line`: one event per order line, emitted next to `order_placed`.** `Tracker.onOrderPlaced` already loads
the order, so the lines cost no extra query. Its properties:
- `order_id`, `line_id`, `sku`, `product_name`, `qty`
- `gross_cents`, `discount_cents` (its share of the order discount), `line_net_cents`, `currency` and `line_net_base_cents`
- `is_gift`

The dedupe key is `act:order_line:<line_id>`. Lines with `removed_at` set are skipped.

Why one event per line and not a `skus` array on order_placed: an array can say *that* X was bought, not how many or
for how much, so it can't produce revenue for X.

**Revenue in the base currency (your answer 2).** Rates live only in core (`currencies.exchange_rate`, which holds
the current rate and no history), and Signals has none. Core therefore stamps the converted amount **when the event
happens**, with `currency.Convert`:
- `order_placed.total_base_cents` and `order_line.line_net_base_cents` and `return_line.refund_net_base_cents`
- `revenue_base_cents` on the conversion signal Signals records

Stamping then is the only way to keep historical revenue at the rate of the day, since no rate history exists.

Events from before this change have no base amount:
- in the base currency, the amount is used as it is
- in another currency, the event is left out of the sum, and the tile says "12 older orders in other currencies
  aren't in this total"

**Cart events.** `cart_update` and `cart_remove` carry `line_id` and no `sku`, so "removed X from cart" can't be
asked. Adding `sku` to both is a two-line change in the tracker, and it goes in the same branch.

### What changes

- **Signals `board`:** `Board.Subject []PropertyFilter` (in boards and drafts). `BoardReportDefinition` gains
  `ignore_subject` and `sum_key`. Both empty mean today's behaviour, so stored reports need no migration.
- **Proto and core:** each Synapse request takes `filters`, and Insights takes `sum_key`. The relaying resolvers pass
  them through.
- **Dashboard:**
  - the subject chips in the board header: a property picker from the declared keys, and a value picker from the
    top values (`topValues` already exists)
  - each tile shows a "follows subject on …" line and an "Ignore the board's subject" switch
  - an Insights "Sum of" measure, shown as money when the key ends in `_cents`

### Revenue, explained (your questions 1 and 2)

**A worked example.** One order, in a shop whose prices don't include tax:

| | T-shirt X | Mug Y | Order |
|---|---|---|---|
| Lines | 2 × €20 = €40 | 1 × €10 = €10 | subtotal €50 |
| 10% coupon on the order | | | −€5 |
| VAT 20% on €45 | | | €9 |
| Shipping | | | €4.90 |
| Paid | | | **€58.90** |
| A week later, one T-shirt is returned | refund €18 + its VAT | | |

What is "the revenue of T-shirt X"? There are several defensible answers. They form the standard ladder in Shopify's
sales reports, and GA4's item revenue is the same idea:

| Figure | X | What it is | Where it comes from |
|---|---|---|---|
| **Gross sales** | €40 | list price × quantity | `order_lines.total_price` |
| **− Discounts** | −€4 | X's share of the coupon: €5 × 40/50 | core's `allocateLineDiscount`, the same split tax and refunds already use |
| **= Net sales at checkout** | **€36** | what X earned when the order was placed | |
| **− Returns** | −€18 | one T-shirt's net value (€36 / 2), booked on the day of the return | `order_return_lines.refund_unit_cents`, without its tax |
| **= Net sales** | **€18** | what X earned in the end | |

Tax is never revenue: it's collected for the state. In a shop whose prices include tax, each figure has its tax
share taken out first (core's per-line tax allocation already does that split, `returns.go`), so the ladder reads
the same.
Shipping and the order-level fees never go to a product. They aren't the product's revenue.

**Best practice, and the recommendation:**
1. **Use net sales: after discounts, without tax and shipping.** That's what GA4's item revenue and Shopify's net
   sales mean. Splitting an order coupon by each line's share of the subtotal is the standard way, and it is
   exactly what core already does for tax and refunds, so the board can't disagree with the refund a customer got.
2. **Book a return on the day it happens, not on the original order's day.** Last month's figures then never change
   after the fact, and the board matches accounting. Shopify works the same way.
3. **So there are two events, and the board does the subtraction:**
   - `order_line` at checkout: `line_net_base_cents` (after the discount share, without tax) plus the gross and
     discount for anyone who wants them
   - `return_line` when a return is received: `refund_net_base_cents` for that line's quantity, from
     `order_return_lines`
   - a "Net sales" tile is `sum(order_line.line_net) − sum(return_line.refund_net)` over the period. The subject on
     `sku` filters both, because both carry `sku`
4. **Order edits after checkout** (a line removed, or a quantity changed in `edit.go`) are booked the same way as a
   return, as an `order_line_adjusted` event dated on the edit, with the difference. Otherwise an edited order
   would keep its old revenue on the board.

**What this means for stage 5's reconciliation test:** for product X over a period, the board's net sales must equal
the commerce DB's lines placed in the period (net of discount, without tax), minus returns received in the period,
plus or minus edits made in the period. All three come from tables that already exist.

### Answers (2026-11-04)

3. **Backfill: yes.** A one-off `skelet cli` command writes `order_line`, `return_line` and `order_line_adjusted`
   events for past orders. Dedupe keys are the same as live events, so running it twice changes nothing.
4. **Operators:** `=`, `≠` and "is one of" in the first version. `≠` treats an event that lacks the property as
   passing, and "is one of" takes up to 50 values.

### Decided (2026-11-04)

1. **Net sales, the standard way.** Discounts are taken off, and tax and shipping are left out. Returns and edits are
   counted on the day they happen.
2. **An order discount is split across lines** by each line's share of the subtotal, with core's
   `allocateLineDiscount`.

## 4. Heuristic evaluation and cognitive walkthrough

**How this was done:**
- **Checked in the running dev dashboard** as the dev superuser on the Default Store (15,448 events, 23 event names):
  each builder in its default state, plus the boards list, a board and the "Add to board" dialog.
- **Interactions not exercised** (filters, breakdowns, switching events) were checked in the code
  (`features/synapse/*`). Those findings are marked *code*.
- **Severity uses Nielsen's scale:** 0 not a problem, 1 cosmetic, 2 minor, 3 major, 4 blocks the task.

### Issues, most severe first

| # | Sev | Where | Heuristic | Issue | Fix | Branch |
|---|---|---|---|---|---|---|
| 1 | 4 | Insights, Engagement, Retention, Flow | Flexibility | No property filter, so none of them can answer "for product X" or "for campaign Y" | Filters on every engine, and the board's subject (stage 3) | engine-property-filters, boards-subject |
| 2 | 4 | Insights | Match with the real world | No sum measure, so revenue can't be shown at all | "Sum of" measure over `*_cents` properties, shown as money | engine-property-filters |
| 3 | 3 | All five | User control | The period is fixed and only stated in the subtitle: "Last 30 days", and Retention "Cohort window: last 60 days" (*code*: hard-coded `sinceDays`) | A period picker in each builder and on the board, with the board's period as the default for its tiles | boards-canvas |
| 4 | 3 | All five | Recognition, user control | Settings live only in component state (*code*: no `useSearchParams`), so a reload, the back button or a pasted link loses the query | Keep the definition in the URL (`?q=<definition>`). On the board, the tile is in the URL (`?tile=n`) | boards-canvas |
| 5 | 3 | All five, and Boards | Visibility, error prevention | With global scope, each page is a dead end: "Select a channel – pick one in the header scope picker". The picker is in the top-left corner, not in a header | Put a channel picker inside the empty state itself | boards-canvas |
| 6 | 3 | Add to board, Board | Minimalism, user control | Four screens to add one report, the builder's state is lost on leaving, and a saved tile can't be edited | The canvas: build and edit in the side sheet beside the board | boards-canvas |
| 7 | 3 | Engagement vs Retention | Match, consistency | Both answer "do people come back?" Engagement's subtitle says exactly that, so for Q4 an admin can't tell which one to use. The names also differ: Engagement in the menu, stickiness in the URL and code, "Return rate" on the page | Name them by their question: "Return rate" (window over window) and "Cohort retention" (period by period), with one line on each pointing to the other | boards-canvas |
| 8 | 3 | Funnel → Flow | Flexibility | Q3 stops at "41% dropped at Added to cart". Nothing leads to *what those people did instead* | "See where they went" on each funnel step adds a Flow tile that starts from people who did step N but not step N+1. The Flow engine needs an "excluding" focus | boards-canvas, engine-property-filters |
| 9 | 3 | Funnel step filter, Funnel breakdown | Error prevention, recognition | Property and value are free-text inputs ("property, e.g. device"), while Insights' breakdown is a list. A typo gives a silent zero | One property picker everywhere (declared keys, stage 3), with value suggestions from `topValues` | engine-property-filters |
| 10 | 2 | Insights | Match | The default is "Any event", which adds page views, web vitals and rage clicks into one meaningless 11,864 | Default to page_view, and group the event list (Shopping, Email, Quality signals) | boards-canvas |
| 11 | 2 | Flow | Minimalism | Technical events show up as journey steps ("$web vitals", 1%) | Leave `$`-events out of paths by default, behind a "Show technical events" toggle | boards-canvas |
| 12 | 2 | Retention | Help users recognise | Cohorts of 3 to 8 people show 0% to 100% with nothing to say the sample is tiny, and the default page_view → page_view by day buries the signal | Fade cells for cohorts under 20 people and add a "small cohort" note. Default to order_placed → order_placed by week when orders exist | boards-canvas |
| 13 | 2 | Funnel, Flow, Engagement, Retention | Consistency, conventions.md | Series are drawn in `bg-primary` (*code*), so they're red in this theme and change with an admin's brand colour. On Engagement, red "Returning" next to blue "New" reads as bad and neutral, and the board's tiles use `bg-series-1` (purple) for the same data | Draw every series with the `series-*` tokens, so a tile and its builder look the same | boards-canvas |
| 14 | 2 | Insights, Retention toggles | Accessibility | Total/Unique and Day/Week are plain buttons with no `aria-pressed` (*code*), so a screen reader can't tell which one is selected | Use `aria-pressed`, or a radio group | boards-canvas |
| 15 | 2 | Add to board dialog | Error prevention | The board picker defaults to "A new board…" even when the shop has boards, and the title defaults to the raw description ("All events") | Default to the last board used, and suggest a title from the definition plus the subject | boards-canvas |
| 16 | 2 | Board | Minimalism | "Name" is a whole section with an always-open input. The empty state is a sentence with no button in it | The name becomes editable in the header. The empty state is the "+ Add report" tile | boards-canvas |
| 17 | 1 | Boards list | Consistency | It has no page header or title, unlike every other list page | Use `DetailPageHeader` with the title "Boards" | boards-canvas |
| 18 | 1 | All five | Minimalism | Every subtitle repeats "People resolved across anonymous + signed-in activity" | Say it once, in an info tooltip next to "people" | boards-canvas |

**What works and should stay:**
- the prior-period delta and "includes 165 from visitors who declined analytics" on Insights
- the friendly step names next to the raw event names on Funnel
- Flow's "most common journeys"
- Retention's "out of the members old enough to have reached it"

### Cognitive walkthrough of your four questions (today's dashboard)

At each step there are four checks: does the admin know what to do, see the control for it, connect the control to
the goal, and understand the result? ✗ marks where it fails.

**Q1 How is product X doing?**
1. Open Synapse → Insights. ✗ With global scope it's a dead end (#5).
2. Choose "View item". ✓
3. Narrow it to X. ✗ There's no filter (#1). The admin tries "Break down by sku" and then has to find X in a list of
   every SKU.
4. Orders and revenue for X. ✗ Impossible: no SKU on orders, and no sum (#2, stage 3).
5. Put it on a board. ✗ Four screens, and every tile has to be narrowed to X again, one by one (#6, no subject).

**Q2 Did campaign Y work?**
1. Funnel: replace the five default steps with email_sent → opened → clicked → conversion. ✓, but slow (four
   removals and four additions).
2. Narrow it to campaign Y. ✗ The admin has to know the property is called `campaign_id` and paste a UUID into a
   free-text box, once per step (#9).
3. Revenue from Y. ✗ No sum (#2).

**Q3 Where do shoppers drop off?**
1. Funnel with the defaults. ✓ The best path today.
2. Break it down by device. ✗ Free text: the admin has to guess `device` (#9).
3. See what the people who dropped off did instead. ✗ There's no way from the funnel to Flow, and Flow can't start
   from "did step N but not step N+1" (#8).

**Q4 Do buyers come back?**
1. Choose a builder. ✗ Engagement and Retention both claim this question (#7).
2. In Retention, change the start and return events to Order placed, and the unit to Week. ✓, but the defaults work
   against the admin (#12).
3. Limit it to buyers of X. ✗ No filter, and no SKU on orders (#1, stage 3).

**Score today:** none of the four questions can be answered end to end. With the canvas, the filters, `order_line`
and the sum measure, every ✗ above has a fix in the table.

### Answers and proposals (2026-11-04)

1. **Engagement and Retention: merge (decided)** them into one builder, **Retention**, with two views: "Return rate"
   (the headline number Engagement shows today) and "By cohort" (today's table). They answer the same question, Q4,
   so they belong in one place. The two engines stay, and a stored board definition keeps its kind (`stickiness` or
   `retention`), so no definition has to be migrated. Only the menu, the labels and the tile picker change.
2. **Flow from a drop-off: now (decided)**, in `engine-property-filters`. Flow's `focus_actors` gains an
   exclusion: people who did A and did **not** do B within the window. The same CTE is already being changed for
   filters there. The funnel step's "See where the 82 went" button goes in `boards-canvas`.
3. **Decided:** the fixes that aren't about boards (#5, #10, #11, #12, #14, #18) go into `boards-canvas`, because the
   builders' controls are being split out there anyway. There's no separate `synapse-builder-fixes` branch.

## 5. Test plan

The goal is evidence that doesn't depend on our own judgment: tests that fail when access or numbers are wrong, and
people who aren't us doing the task.

### 5.1 Access matrix (table-driven)

**Where:**
- `apps/backend/signals/internal/board/access_test.go`: a pure test of `access(board, actor)`
- `service_db_test.go`: the same rows through `List`, `Get`, `Save`, `Share` and `Delete` against `testdb`

**Actors** (all with `boards.view` on channel A unless the actor says otherwise):
- the owner
- an admin with a direct view share, and one with a direct edit share
- a member of a role that has a view share, and of a role that has an edit share
- a former member of that role (removed since the share was made)
- an unrelated admin, with `boards.view`
- an unrelated admin, with `boards.manage`
- a superuser
- an admin who has the share but `boards.view` only on channel B
- an admin who has the share but no `boards.view` at all

**Board states:** private draft, private published, shared, shop board, and orphaned (owner removed).

**Expected result per row:** one of none, view, edit or own, plus the error each refused call gives:
- `NotFound` when the actor may not know the board exists
- `PermissionDenied` when they can see it but not do the action

So a private board answers `NotFound` to an unrelated admin, and never `PermissionDenied`.

**Data permission, the third axis.** For every row that can view, the core relay test
(`graph/admin/signals_boards_db_test.go`, using the fake-client pattern of `signals_scope_db_test.go`) checks two
things:
- with `customers.view`, the tile query is relayed
- without it, the board still opens, the tile query answers `FORBIDDEN`, and the board response holds no report
  results

A dashboard test (`ReportTile.test.tsx`) checks that the "You need Customer analytics on …" placeholder renders for
that error.

**Drafts and ownership:**
- a draft is visible only to its admin
- Save with a stale `base_updated_at` is refused, and both sides are kept
- `AdminRemoved` stamps `owner_removed_at`, drops the removed admin's drafts, favourites and shares, and leaves the
  board otherwise untouched
- claiming needs `boards.manage`
- the 30-day sweep deletes only expired drafts

### 5.2 Reconciliation: the board equals the commerce DB

**Where:** `apps/backend/core/internal/e2e/boards_reconcile_test.go`, run by `skelet test e2e`. That suite already
runs real core, real Signals and its own ClickHouse spine.

**The scenario** uses one product X, one other product Y and two currencies:
1. Place order 1: 2 × X and 1 × Y with a 10% order coupon (the worked example in stage 3).
2. Place order 2: 1 × X in a second currency with a known exchange rate.
3. Place order 3: Y only.
4. Return one X from order 1.
5. Edit order 2 so X's quantity goes from 1 to 2.
6. Wait until the spine has the events (poll the board query with a timeout, not a sleep).

**Asserted, through the same admin GraphQL the board uses, with the subject `sku = X`:**

| Board figure | Must equal (SQL on the commerce DB) |
|---|---|
| Orders containing X | `count(DISTINCT order_id)` over `order_lines` where `product_sku = X` and `removed_at IS NULL` |
| Units of X | `sum(quantity)` over the same lines, minus returned quantity |
| Net sales of X in the base currency | lines net of `allocateLineDiscount`, converted with `currency.Convert`, minus returns net of tax, plus or minus the edit |
| Buyers of X (unique) | `count(DISTINCT customer)` over the same orders |
| Order 3 | contributes nothing to any X figure |

Figures are compared exactly, in integer cents with no tolerance. That works because both sides use the same
allocation and conversion functions, so any drift is a bug.

**Re-run check:** the backfill command is run twice on the same orders, and the figures don't change, because
dedupe keys are identical to the live events'.

### 5.3 Engine known answers

These go beside the existing ClickHouse known-answer tests (commit 0197c414). Each one seeds a few hand-made events
and asserts exact numbers:
- every engine with the filters `=`, `≠` (an event without the property passes) and "is one of"
- the rule that a filter applies only to events that declare its key: `page_view` with sku is filtered strictly,
  and `order_placed` without sku is left alone
- the sum measure, including events in another currency that have no base amount (left out, and counted for the
  "not converted" note)
- Flow's exclusion: people who did A and not B within the window
- Retention with separate start and return filters (buyers of X who come back to any order)

### 5.4 The canvas (dashboard, Vitest)

- `BoardPage.test.tsx`: add, edit in the sheet, resize, drag-reorder and remove all stage changes. Save opens
  `SaveConfirmDialog` with each before→after. Discard restores the board.
- a draft is written about a second after the last change, and reopening the board shows the "unsaved changes" banner
- each kind's `KindControls` produces the definition the engine expects (one test per kind), and the scratch page and
  the sheet produce the same definition from the same clicks
- the subject chips: a tile shows which of its events follow the subject, and the opt-out switch works

### 5.5 A task test with five people

**Who:** five people who aren't on the team and haven't seen the design. They should be shop staff or marketers who
use an admin panel weekly (Nielsen's five-user rule finds about 85% of usability problems). **You recruit and run it.**
I've written the script, but I can't take part.

**Setup:**
- the seeded dev shop, plus a product X with 40+ orders over 60 days
- a role "Marketing" that has `boards.view` but **not** `customers.view`
- each participant signs in as an admin with `boards.manage` and `customers.view`
- record the screen, and have them think aloud

**Tasks** (read aloud, one at a time, with no hints):
1. "Make a board that shows how product X has done over the last 60 days: how many people looked at it, how many
   added it to the cart, how many bought it, and how much it earned."
2. "Share it with the Marketing team so they can look but not change it."
3. "Close the browser, come back, and find your board again."
4. (Follow-up) "Someone in Marketing says the board is empty for them. Why?" This checks whether the warning from
   stage 2 is understood.

**Measured for each participant and task:**
- **Success:** complete without help / complete with one hint / failed. The success rate counts unaided completions
  only.
- **Time on task**, from the end of reading the task to the participant saying "done".
- **Errors:** wrong subject, a wrong share level, or leaving without saving.
- **Single Ease Question** after each task: "How easy was that?", 1–7.

**What counts as a pass:**
- tasks 1 and 2: at least 4 of 5 succeed unaided, with a median time under 5 minutes for task 1 and under 1 minute
  for task 2
- task 3: 5 of 5
- task 4: at least 4 of 5 name the missing permission

**Results table** (filled in after the sessions):

| P | T1 ok | T1 time | T2 ok | T2 time | T3 ok | T4 ok | SEQ T1/T2 | Errors and notes |
|---|---|---|---|---|---|---|---|---|
| 1 | | | | | | | | |
| 2 | | | | | | | | |
| 3 | | | | | | | | |
| 4 | | | | | | | | |
| 5 | | | | | | | | |

Each problem seen in two or more sessions becomes an issue with a severity on stage 4's scale, and anything of
severity 3 or higher is fixed before the branch merges. One pilot session with a colleague comes first, to check that
the script and data work.

### 5.6 Which branch carries which test

| Branch | Tests that come with it |
|---|---|
| `feat/boards-canvas` | 5.4, plus the builder-fix tests (toggle `aria-pressed`, channel picker in the empty state, defaults) |
| `feat/boards-ownership` | 5.1 |
| `feat/spine-order-lines` | tracker unit tests for `order_line`, `return_line` and `order_line_adjusted`, backfill idempotence, and the first half of 5.2: the spine's raw events for X, summed in ClickHouse, equal the commerce DB |
| `feat/engine-property-filters` | 5.3, and 5.2 in full through Insights' sum with a filter |
| `feat/boards-subject` | the subject parts of 5.4, plus 5.2 extended to the subject on every tile kind |
| after all of them | 5.5 on a build that includes everything |

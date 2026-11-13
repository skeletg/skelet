- **The CMS home leads with everything you can open.** Pages come first, with how many there are, a mark for each
  one by state and the last edit. Page types, attributes, sections, block types, the blog, menus and forms follow,
  each with a small drawing, a count, an arrow to open it and a note saying whether it's for pages, the blog or
  the storefront.
- **Beneath it, a map of the site.** Every page is drawn as a card listing the blocks it's built from, top to
  bottom, with its state, address and last edit. Pages sit in lanes by where they live on the site, or by page
  type. The board narrows to a state or to a title or address you type, and picking a kind of block (content,
  media, catalog and blog) shows where it's used.
- **A page's content says which blocks it holds.** `outline` on the admin API's `CMSPageContent` lists the body's
  block types in order, with a container opened onto what it holds, so a list can show how pages are built
  without loading every body.
- **The admin API counts pages and posts.** The new `cmsPageCount(filter)` says how many pages a filter matches
  beyond what `cmsPages` hands over, and `blogPosts` now fills `totalCount`, which it declared but always left
  empty. The CMS home uses both, so it shows the real number of pages on a site larger than the board draws.

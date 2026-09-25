# Forms - manual QA

`forms.spec.ts` checks, against the real API, that a Save writes what the review dialog listed,
that sections sharing one backend row keep each other's fields, and that a draft survives a
reload. This list covers what it cannot judge. Run it once per forms change that touches rich
text, builders or styling.

## Before / after payloads

For the product page, a CMS page and a promotion rule, make the same edits on `main` and on the
branch with DevTools → Network open, filter `graphql`, and compare the mutation variables. They
should match, except where the branch changed them on purpose:

- `upsertProductContent` carries the whole locale row (name, descriptions, meta), not just the
  section's fields
- `setProductListing` keeps the store's price, published flag and URL it did not change
- clearing a product's canonical URL sends `canonicalUrl: ""`
- the product update sends only the SEO fields that changed

## Rich text and builders

- [ ] Product short description and description: bold, a list, a link, an image upload. Save,
      reload - formatting and the image are still there
- [ ] Blog post body: same as above
- [ ] A formatting-only edit (bold a word, text unchanged) shows as "previous formatting →
      updated formatting" in the review dialog
- [ ] CMS page layout: add, reorder and remove blocks by drag and drop; the live preview follows;
      Save lists the layout change
- [ ] Edit a rich-text field, reload: the restored draft shows in the editor, not only in the form
- [ ] Web form builder (still on `useDraft`): behaves as on `main`

## Look and states

- [ ] Light and dark mode: field errors, the amber "pending" markers (per-store pricing,
      variants, categories), the restored-draft banner
- [ ] An admin without `catalog.manage` sees the product page read-only - no Save, inputs
      disabled, no drafts written
- [ ] Closing a workspace tab with unsaved edits still asks first; a page refresh does not

## Drafts

- [ ] Edit two sections of the product page, reload: both come back, one banner per section
- [ ] Change the same field in another browser and save there, then reload the first: the
      banner flags the clash and shows the server value under the field
- [ ] Log out: no `skelet.draft.*` keys are left in localStorage

- **SVG uploads work when the admin API has a slug.** The dashboard posted them to `/api/admin/<slug>/media/svg`,
  a path the backend never served, so with `ADMIN_API_SLUG` set every SVG upload failed with a 404.

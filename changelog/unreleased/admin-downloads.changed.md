- **Downloads in the dashboard are saved by the browser.** Exports, import templates and form uploads used to be fetched into the
  page's memory before being saved, which strained the browser on large files. The dashboard now asks
  `/api/admin/download-ticket` for a link to the one file, good for a minute and tied to the admin's session, and
  the browser streams it to disk.
- **The admin file routes answer 401 when there is no session.** Export and import downloads, form uploads and library
  files used to answer 403 for an expired session as well as a missing permission, so the dashboard couldn't tell when to renew
  the session. It now renews it once and tries again, the way GraphQL requests do.

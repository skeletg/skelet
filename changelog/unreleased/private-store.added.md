- **Private files have a store of their own.** Set `PRIVATE_S3_BUCKET` (or `PRIVATE_STORAGE_DIR` on the
  filesystem driver) to a bucket with no public access, and exports, import uploads and the files shoppers send
  through forms are kept there instead of in the media bucket, where only a random address stood between them
  and anyone. At boot the app tries to read a file from that store without credentials and refuses to start in
  production if it can. The dev stack creates and uses a `private` bucket.
- **Files sent through a form can be downloaded in the dashboard.** The submissions page lists each
  submission's files, fetched through a route that checks the admin's grant to read that form's submissions.
  They were stored before, but there was no way to open one.

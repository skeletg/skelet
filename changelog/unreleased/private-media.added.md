- **The media library has a private half.** On an install with a private store (`PRIVATE_S3_BUCKET` or
  `PRIVATE_STORAGE_DIR`), the library page gets a Public / Private switch. Private files are uploaded straight to
  the private store, keep their own folders and have no address: the dashboard shows them through the API with the
  admin's session, and they can't be linked to, picked for a product or reached from the storefront. A file's
  details panel moves it between the halves, and making one private takes the public original and every resized
  copy of it down. Without a private store the library is what it was.
- **The private half has permissions of its own.** `media.private.view` and `media.private.manage` sit on top of
  `media.view` and `media.manage`, so someone who looks after product pictures can't open or change a private file,
  and never sees the Private switch. They count only when granted globally, since the library isn't split by
  channel. Moving a file between the halves takes the grant for both. A private picture is never sent to the AI
  provider for alt text.

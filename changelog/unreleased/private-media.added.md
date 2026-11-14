- **The media library has a private half.** On an install with a private store (`PRIVATE_S3_BUCKET` or
  `PRIVATE_STORAGE_DIR`), the library page gets a Public / Private switch. Private files are uploaded straight to
  the private store, keep their own folders and have no address: the dashboard shows them through the API with the
  admin's session, and they can't be linked to, picked for a product or reached from the storefront. A file's
  details panel moves it between the halves, and making one private takes the public original and every resized
  copy of it down. Without a private store the library is what it was.
- **A private file can be shared with a customer.** From the file's details panel, with an optional download limit
  and last day. It shows under Downloads in the customer's account and is streamed through the same token link a
  digital purchase uses. Sharing again starts the share over under the new limits, a share can be taken back, and
  it ends when the customer's account is deleted or erased. Only private files are shared, and only with an open
  account. `myDownloads.productId` is now nullable on the storefront API, since no
  product stands behind a shared file.
- **The private half has permissions of its own.** `media.private.view`, `media.private.manage` and `media.share`
  sit on top of `media.view` and `media.manage`, so someone who looks after product pictures can't open, change
  or share a private file, and never sees the Private switch. They count only when granted globally, since the
  library isn't split by channel. Moving a file between the halves takes the grant for both, a share also takes
  `customers.view` on the customer's channel, and who a file is shared with is kept back from an admin who may
  not see that customer. A private picture is never sent to the AI provider for alt text.

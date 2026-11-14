- **Private files have a store of their own.** Set `PRIVATE_S3_BUCKET` (or `PRIVATE_STORAGE_DIR` on the
  filesystem driver) to a bucket with no public access, and exports and import uploads are kept there instead of
  in the media bucket, where only a random address stood between them and anyone. At boot the app tries to read a
  file from that store without credentials and refuses to start in production if it can. The dev stack creates
  and uses a `private` bucket.

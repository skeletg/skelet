- **Production needs a private store to start.** `PRIVATE_S3_BUCKET` (or `PRIVATE_STORAGE_DIR` on the filesystem
  driver) is required when `APP_ENV=production`. Without one, exports and import uploads would sit in the public
  media bucket behind a random address. Add a second bucket with no public access before upgrading.

- **`download.accessed` says which kind of file was downloaded.** Its payload has a new `AssetID`, set for a file
  the shop shared from the media library, and `ProductID` is null for those instead of an all-zero id. A file a
  customer bought looks the same as before.
- **A shared file's title stays out of webhooks.** `download.accessed` for a file shared from the media library
  carries an empty `Title`, since it's the shop's own name for a private file.

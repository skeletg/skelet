- **`skelet add channel` refuses an override with no `services:` section.** It added the
  storefront's volumes there, dropped the service itself, and reported it written.
- **`skelet add channel` handles an override that lists `volumes:` above `services:`.** The new
  volumes landed under `services:`, compose rejected the file, and nothing could be added.
- **`skelet add channel` leaves the source's `node_modules` and `.next` behind** instead of copying
  them into the fork and deleting them afterwards - a host install alone can run to a gigabyte.
- **`skelet add channel` refuses a name the override already uses, and a `--from` that isn't a
  storefront name,** before anything is copied.
- **`skelet rm channel` says when a tracked storefront holds untracked files.** It called the
  directory recoverable with git checkout, which brings none of them back.

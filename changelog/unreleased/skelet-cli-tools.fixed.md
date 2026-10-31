- **`skelet gen` writes sqlc's files as you.** sqlc ran as its image's root, so on a Linux Docker
  engine every regenerated file under `internal/db/sqlc` belonged to root.
- **`skelet gen buf` hands its arguments to buf as they are.** They were spliced into a shell
  command, so one with a space or a quote in it was split or misread.

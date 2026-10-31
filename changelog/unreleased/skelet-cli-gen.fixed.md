- **`skelet gen --check` catches a generated file that's new, and lists only what it changed.** It
  compared the diff of tracked files, so a newly generated file that wasn't committed passed, and
  files that were already modified before the run showed up as stale.

- **Doctor's `*.localhost` check can fail.** Its fallback passed on any output from curl, including the
  `000` curl prints when the name doesn't resolve, so the check never warned. Now only a failed lookup
  warns, and any answer from the server - a 404 included - passes.

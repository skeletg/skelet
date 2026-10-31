# Unreleased changes

Each change that belongs in the changelog lands here as its own small file, so branches
never fight over the same lines. A release gathers them into `changelog/<version>.md`,
lists that in [CHANGELOG.md](../../CHANGELOG.md), and deletes them.

Name the file `<slug>.<section>.md`, where the slug is usually your branch name and the
section is one of `added`, `changed`, `deprecated`, `removed`, `fixed` or `security`:

```
changelog/unreleased/filesystem-storage.added.md
changelog/unreleased/download-url-keys.fixed.md
```

Inside, write the entry exactly as it should read in the release - one or more bullets,
no headings:

```markdown
- **Files can live on the local disk.** `STORAGE_DRIVER=filesystem` keeps every object
  under `STORAGE_DIR` instead of S3.
```

A change that fits two sections gets two files. A `<slug>.summary.md` holds a plain
paragraph instead, printed at the top of the release before any section - for a release
that deserves a sentence or two on what it's about.

`skelet changelog` shows what the next release will say, `skelet check` fails on a
badly named or formatted fragment, and `skelet changelog release <version>` writes the
release - see [docs/releasing.md](../../docs/releasing.md).

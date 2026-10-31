- **Install the CLI, then start a project with `skelet new`.**
  `curl -fsSL https://github.com/skeletg/skelet/releases/latest/download/install.sh | sh` puts
  `skelet` on your PATH after checking it against the release's checksums, and
  `skelet new my-shop` fetches a release into `my-shop` and runs its setup. The project is a git
  checkout with Skelet as its `upstream` remote and your own `main` branch, so a later release is
  a merge. Inside a project the installed `skelet` hands every command to that project's own CLI,
  so it never runs one with a different release's code. Each release now carries the CLI for
  macOS and Linux, built by the tag push with a provenance attestation.

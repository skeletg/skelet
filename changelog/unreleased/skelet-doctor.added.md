- **`skelet doctor` checks more of what breaks a clone.** git new enough for setup's sparse-checkout,
  Go on the host against `go.mod` (lint and parts of codegen run it there), Node against `.nvmrc` for
  the frontends you run on the host, `plugins.json` against the plugin directories by the rules codegen
  enforces, whether the pre-commit hook is installed, and a WSL clone sitting on the Windows drive.
  Outside a project, the installed `skelet doctor` checks the machine alone, before `skelet new`.

- **A project's own `v*` tag no longer tries to publish a Skelet release.** The release workflow runs only in
  Skelet's repository, so a project made with `skelet new` can tag its versions freely. The docs also say how a
  project keeps Renovate to its own shops, so its bumps don't collide with the next release's.

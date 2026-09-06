# Releasing

How a version gets cut, and what the branches mean.

## The two branches

**`main` is released code only.** Every commit on it is a release, and every
release is tagged. If you are looking at `main`, you are looking at what someone
could be running in production.

**`develop` is where work integrates.** Feature branches cut from it and merge
back into it. It is not expected to be released, and CI does not gate it - the
suite runs on pull requests into `main`, so a develop→main PR is where a
regression surfaces. Run `bin/check` and `bin/test-e2e` locally before merging
anything sizeable, or the release PR is the first time anyone finds out.

Nothing else is long-lived. Feature branches are short and get deleted after they
merge; the repo already carries forty of them, and that is forty too many.

```
feature/x ──┐
            ├──> develop ──────> main + tag vX.Y.Z
feature/y ──┘
```

A fix that cannot wait for the next release branches from `main`, merges to
`main`, gets tagged, and is merged back into `develop` so it is not lost.

## Version numbers, and which is which

Four things carry versions, and they move independently. Conflating them is the
mistake to avoid:

| | Where | What it means |
|---|---|---|
| **Release tag** | `vX.Y.Z` git tag | the whole product |
| **SDK** | `sdk.Version` | the plugin API surface |
| **Provider contract** | `contract.Version` | the remote-provider wire format |
| **Plugins** | each plugin's `Manifest.Version` | that plugin |

They all read `0.1.0` today because everything starts there. They will diverge on
the first release that changes one and not the others, and that is correct - the
product reaching v0.4 does not mean the SDK did.

While the product is `0.x`, breaking changes ride the MINOR and everything else
the PATCH. There is no compatibility promise until `1.0.0`.

## Cutting a release

1. **Land everything.** `develop` is green, the working tree is clean, and no
   half-finished work is sitting uncommitted.

2. **Decide the version.** Look at what changed since the last tag. Breaking
   change in `0.x` means a MINOR bump.

3. **Update the changelog.** Move `Unreleased` entries under the new version with
   today's date. See [CHANGELOG.md](../CHANGELOG.md).

4. **Bump the version constants that actually changed.** `sdk.Version` only if
   the SDK surface moved, `contract.Version` only if the wire format did. Do not
   bump them in sympathy with the product version.

5. **Run the gates.** `bin/check`, `bin/lint`, `bin/test-e2e`, and
   `bin/codegen --check`. All of them, not the fast ones.

6. **Merge `develop` into `main`.** No squash - `main`'s history should show what
   went into the release.

7. **Tag it.**

   ```
   git tag -a v0.1.0 -m "v0.1.0"
   git push origin main --follow-tags
   ```

   Annotated, not lightweight: `git describe` and the SDK gate both want a real
   tag object.

8. **Publish images**, if you are shipping them:

   ```
   REGISTRY=registry.example.com bin/push-images v0.1.0
   ```

9. **Merge `main` back into `develop`** so the changelog and any release-time
   fixes are on both.

## What the first tag switches on

The SDK's compatibility gate (`sdk.TestAPICompatibility`) resolves its baseline
with `git describe --tags --match "v*"` and **skips while no such tag exists**.
It has been inert this whole time.

Tagging `v0.1.0` activates it. From that point every change to the SDK's exported
surface is diffed against the last release with `apidiff`, and a change that
breaks compatibility fails CI unless `sdk.Version` carries the bump the rules
demand. That is the point - but it means the first release is also the moment the
SDK stops being freely reshapeable, so get the surface where you want it *before*
tagging rather than after.

## Before the first release

One thing is release-blocking and it is not about code:

- **Copyright and contribution terms.** The code is GPL-3.0 with the
  [plugin exception](../PLUGIN-EXCEPTION.txt), and copyright is asserted to Skelet
  Group. If that entity is to hold the rights - which is what keeps commercial
  licensing possible later - the contribution terms need to say so before the
  first outside contribution, not after.

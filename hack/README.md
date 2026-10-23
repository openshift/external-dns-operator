# Development scripts

## Renovate policy regression checks

Run `hack/verify-renovate.cjs` with Node.js 22 or newer and Renovate available
on `NODE_PATH`. The checks use Renovate's actual update flattening and grouping
implementation, in addition to asserting the repository's policy selectors.
For example, install Renovate outside the checkout and run:

```sh
validation_dir=$(mktemp -d)
npm install --prefix "$validation_dir" --no-audit --no-fund renovate@42.99.0
NODE_PATH="$validation_dir/node_modules" node hack/verify-renovate.cjs
```

The test-only Renovate version is pinned because these checks use its internal
APIs. Update the pin together with the test when upgrading those APIs; it does
not select the version of Renovate deployed by MintMaker.

Only digest updates extracted by the Tekton manager from `.tekton/**` on `main`
receive `allow-automatic-merge`. Their dedicated `konflux/tekton-digests/main`
branch cannot be reused by semantic, Dockerfile, or release-branch updates,
even if a label persists after a PR update or a rebase. Other updates retain
their existing configuration, including the Konflux migration commands.
This label opts into a separate Tide approval policy; it does not enable
Renovate or GitHub native automerge and does not bypass required checks.

Before rollout, confirm the active MintMaker inherited configuration and bot
identity. GitHub labels manually changed by another account are not guaranteed
to be resynchronized by Renovate; do not manually apply this opt-in label to
mixed or noneligible PRs. Enable labeling first, inspect an isolated digest PR,
then enable the Tide query. Existing Konflux reference PRs are not retroactively
eligible and continue through the manual approval paths.

## Version synchronization

This script automates the synchronization of version numbers between the project's source of truth and its Containerfiles.

### What this does

The script reads the version number from the central `VERSION` file and updates the version and release labels in the specified Containerfiles. This prevents build failures due to version mismatches and ensures consistency across all built artifacts.

### Why here?
This script lives in the hack/ directory because it's a developer tool for local use. It helps developers prepare their commits and pull requests by ensuring all Containerfiles are properly versioned before pushing changes. While Konflux has its own checks, this script helps maintain a clean and consistent repository history.

### The script
`sync-version.sh`

This script finds and updates version labels in the Containerfiles based on the `VERSION` file. It will fail if the `VERSION` file or a Containerfile is not found, or if a version label is not present in the correct format.

### How to use this
Before committing changes that update the VERSION file, run this script to ensure all Containerfiles are in sync.

### Local testing
Run the script from the repository root

`make set-version`

The script will output UPDATE messages to show the status of each Containerfile and will automatically update any files that are out of sync.

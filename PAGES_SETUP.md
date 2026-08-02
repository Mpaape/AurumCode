# GitHub Pages source configuration

The AurumCode action publishes through the GitHub Actions deployment flow
(`actions/upload-pages-artifact` and `actions/deploy-pages`), not by pushing a
branch. A repository whose Pages source is still a branch rejects that artifact.

## Required setting

1. Open Settings > Pages of the repository that runs the action.
2. Under "Build and deployment", set **Source** to **GitHub Actions**.

## When this is needed

| `publish` input | Pages source must be "GitHub Actions" |
|-----------------|---------------------------------------|
| `none` (default) | no, nothing is uploaded |
| `artifact` | no for this job; yes for the job that later runs `actions/deploy-pages` |
| `pages` | yes |

The publishing job also has to grant `pages: write` and `id-token: write`, since
a composite action cannot declare permissions itself. The full workflow is in
[ACTION_USAGE.md](ACTION_USAGE.md).

## Checking it worked

The action's `page-url` output carries the published URL and is empty unless
`publish` was set to `pages`.

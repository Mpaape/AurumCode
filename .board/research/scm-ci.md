# SCM and CI contract research

- Researched: 2026-08-04
- Status: design input for AUR-018 and the SCM/CI adapter cards that depend on
  it; not implementation proof
- Scope: the capability and trust-boundary matrix that a GitHub, Gitea, or
  GitLab adapter must respect when an AurumCode `analyzer` reads a change and
  a `publisher` writes a result back; this card does not implement a client
  or a pipeline

## Sources examined

| Source | Version / accessed | Useful property | What this card does not adopt |
|---|---|---|---|
| [GitHub Docs — Workflow syntax for GitHub Actions, `permissions`](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax) | accessed 2026-08-04 | States the least-privilege rule directly: "As a good security practice, you should grant the `GITHUB_TOKEN` the least required access." Unspecified scopes default to `none` once `permissions:` is set. | Treating the default (unset) token as already least-privilege; it is not until scoped. |
| [GitHub Docs — Automatic token authentication](https://docs.github.com/en/actions/security-for-github-actions/security-guides/automatic-token-authentication) | accessed 2026-08-04 | Documents that a workflow triggered by `pull_request_target` receives a `GITHUB_TOKEN` with read/write repository permission "even when it is triggered from a public fork." For plain `pull_request` from a fork, the token is read-only unless an administrator opts into sending write tokens to fork PR workflows. | Assuming `pull_request_target` is safe for analyzing untrusted content; the doc treats it as the dangerous, privileged case. |
| [GitHub Docs — Security hardening for GitHub Actions](https://docs.github.com/en/actions/security-for-github-actions/security-guides/security-hardening-for-github-actions) | accessed 2026-08-04 | States workflows on `pull_request_target` / `workflow_run` are "privileged," "may have repository write access and access to referenced secrets," and "must not explicitly check out untrusted code, including from pull request forks." | The doc's own escape hatch (manual review before a privileged run) as a substitute for a read-only analyzer; AurumCode still requires the read-only credential by default. |
| [GitLab Docs — Merge request pipelines](https://docs.gitlab.com/ci/pipelines/merge_request_pipelines/) | reflects GitLab 18.1 protected-variable controls, 18.11 custom inputs; accessed 2026-08-04 | "Merge request pipelines from forked repositories cannot access these protected resources" (protected CI/CD variables, protected runners); the pipeline runs in the fork project's own context by default. | The documented maintainer override ("run pipeline in the parent project") as a default analyzer path; GitLab shows a malicious-code warning on that path for a reason. |
| [GitLab Docs — CI/CD job token (`CI_JOB_TOKEN`)](https://docs.gitlab.com/ci/jobs/ci_job_token/) | reflects features through GitLab 19.1; accessed 2026-08-04 | The job token carries "the same access level as the user that triggered the pipeline," is scoped to the originating project by default (cross-project access needs an explicit allowlist entry), is valid only while the job runs, is revoked afterward, and is masked in job logs. | Cross-project allowlisting as a substitute for scope reduction; the doc is explicit that allowlisting "does not give additional permissions." |
| [Gitea Docs — Actions job token permissions (`GITEA_TOKEN`)](https://docs.gitea.com/usage/actions/token-permissions) | Gitea Docs v1.27.1; accessed 2026-08-04 | States plainly: "workflows triggered by pull requests from forks are always restricted to read-only permissions for repository contents, regardless of workflow `permissions:` or settings." This is a platform-enforced floor, not a configurable default. | Relying on Gitea's stronger platform guarantee to skip explicit `permissions:` scoping in the analyzer workflow; AurumCode still declares read-only explicitly so the contract is forge-independent. |
| [Gitea Docs — API usage, access tokens](https://docs.gitea.com/development/api-usage) | Gitea Docs v1.27.1; accessed 2026-08-04 | Personal/API tokens use fine-grained `<read or write>:<category>` scopes (e.g. `repository`, `issue`, `package`, `organization`); a token can instead request `"scopes":["all"]`, which grants full read/write. | Using an `all`-scoped token for either the analyzer or the publisher; the matrix below requires the narrowest category scope for each role. |

Each source above is an official, versioned vendor document reachable at the
listed URL; none is a blog, forum, or unversioned mirror. Where the vendor
documentation itself carries a product version (GitLab, Gitea) that version is
recorded; GitHub's docs are undated per-page, so the access date pins the
research to a point in time, matching the same convention `memory-design.md`
uses for GitHub-hosted prior art.

## Findings

1. All three forges separate an ephemeral, job-scoped CI token
   (`GITHUB_TOKEN`, `CI_JOB_TOKEN`, `GITEA_TOKEN`) from a longer-lived
   personal/API token. The CI token is the correct credential for both the
   `analyzer` and the `publisher` role; a personal access token is reserved
   for operations no CI token can perform (for example cross-project GitLab
   calls or Gitea org-level administration), and never for reading a hostile
   head.
2. GitHub and GitLab both draw the same trust line at the fork boundary:
   GitHub restricts the default `GITHUB_TOKEN` to read-only for `pull_request`
   from a fork and calls the write/secrets-bearing alternative
   (`pull_request_target`, `workflow_run` on untrusted input) "privileged" and
   dangerous to combine with checking out the untrusted ref. GitLab restricts
   fork merge-request pipelines from protected variables and protected
   runners for the identical reason. Gitea goes further and makes the
   fork-PR read-only restriction platform-enforced rather than
   workflow-configurable.
3. None of the three forges grants "least privilege" by default merely by
   existing — GitHub's own docs frame it as something the workflow author must
   grant deliberately (`permissions:` key); an unscoped classic PAT or an
   `all`-scoped Gitea token is the opposite of least privilege. The matrix
   below is therefore a requirement on the adapter's requested scope, not an
   assumption about the forge's default.
4. CI job tokens are pipeline-lifetime and log-masked (documented for GitLab;
   equivalent ephemeral-token handling applies to GitHub Actions and Gitea
   Actions runs), which makes them strictly preferable to a standing PAT for
   any automated `analyzer` or `publisher` run, independent of forge.
5. The `publisher` role (posting a check result, a PR/MR comment, a commit
   status, or a release) always requires a context distinct from the one that
   executed the hostile head: a separate job/workflow, the base-repository
   context, or an explicit maintainer-triggered rerun. No forge in this
   research offers a mode where the same credential both executes untrusted
   code and safely holds write/secrets access at once.

## Capability and trust-boundary matrix

| Forge | Analyzer credential | Analyzer scope | Publisher credential | Publisher scope | Trust boundary at the hostile head |
|---|---|---|---|---|---|
| GitHub | `GITHUB_TOKEN` under `pull_request` (never `pull_request_target`/`workflow_run` against the same untrusted checkout) | `contents: read`, `pull-requests: read` only; explicit `permissions:` block, no inherited defaults | `GITHUB_TOKEN` under a decoupled trusted trigger (e.g. `workflow_run` after the analyzer completes, or a maintainer-triggered run), or a fine-grained PAT scoped to the one repository | `pull-requests: write` / `checks: write` / `statuses: write` as needed, never `contents: write` beyond what publishing requires | Fork `pull_request` token is read-only by GitHub default; `pull_request_target`/`workflow_run` are documented as privileged and must not check out the untrusted ref |
| GitLab | `CI_JOB_TOKEN` from the merge-request pipeline running in the fork/source project | Default job-token scope only (no expanded allowlist, no protected-variable access) | `CI_JOB_TOKEN` from a pipeline in the target (parent) project, or a project/group access token scoped to that project | API scope limited to the merge-request/notes/release endpoints being written | Fork MR pipelines cannot reach protected CI/CD variables or protected runners of the parent project by GitLab design |
| Gitea | `GITEA_TOKEN` in restricted mode (`code: read`) or the platform-enforced fork read-only floor | Read-only `repository`/`code`; never `"scopes":["all"]` | `GITEA_TOKEN` from a workflow not triggered by the fork PR, or a fine-grained API token with `write:repository`/`write:issue` as needed | Narrowest `write:<category>` for the action taken, never `all` | Gitea enforces read-only `GITEA_TOKEN` permissions for fork pull request workflows regardless of configured `permissions:` |

Machine-readable limits for adapters are fixed in `standards/scm`.

## MVP acceptance consequences

- An `analyzer` MUST NOT hold a write-scoped credential while its input
  includes a hostile head (a fork, an external branch, or any ref not owned
  by a repository maintainer); the credential presented to it is read-only on
  every one of GitHub, Gitea, and GitLab.
- A `publisher` MUST run in a context decoupled from the hostile-head
  checkout: a separate job/workflow/pipeline, the base/target project, or an
  explicitly re-triggered maintainer run — never the same execution that
  analyzed untrusted content while holding write/secrets access.
- Least privilege is enforced per role and per forge: the adapter requests
  the narrowest scope in the matrix above, never an unscoped classic PAT and
  never an `all`/broad-scope token, even when the forge would issue one.
- Every capability the matrix grants is traceable to a dated, versioned
  vendor source in this file; a capability without such a source is not
  added to `standards/scm`.

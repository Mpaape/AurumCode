# Unified diff validator

- Pinned version: `3.10` (see `../versions.yaml`, `unified-diff.version`) —
  the GNU Diffutils release whose manual is the normative text for this
  format; unified diff itself carries no independent version number.
- Source: [GNU Diffutils 3.10 manual — Detailed Description of Unified
  Format, Wayback Machine snapshot captured 2023-05-27 (six days after the
  3.10 release, and a match for the release: the snapshot text itself reads
  "This manual is for GNU Diffutils (version 3.10, 2 January 2023)")](https://web.archive.org/web/20230527075044/https://www.gnu.org/software/diffutils/manual/html_node/Detailed-Unified.html).
  The live `gnu.org` page is not used as the citation because it has no
  version segment in its path and its content tracks the diffutils release
  current at fetch time (3.12 as of this research), not 3.10 — unlike
  `git-apply/2.43.0` below, `sarif-v2.1.0`, `oas/v3.2.0`, or
  `draft/2020-12`, whose paths carry the pinned version so the same URL
  cannot silently drift to a different one.
- Named validator for a future adapter card: `git apply --check` (Git
  2.43.0, [git-apply documentation](https://git-scm.com/docs/git-apply/2.43.0)).
  Not installed or invoked by this card; the sealed acceptance container has
  no `git` binary.
- Structural rule this card's own bounded lint checks instead: a `--- a/`
  line and a `+++ b/` line are present, at least one `@@ -<old_start>,
  <old_count> +<new_start>,<new_count> @@` hunk header is present, and for
  each hunk the number of body lines that are context (` `) or removed
  (`-`) equals the header's declared `<old_count>`, and the number of body
  lines that are context or added (`+`) equals the header's declared
  `<new_count>`. `git apply --check` performs the equivalent internal
  consistency check before attempting to apply a hunk.
- `fixtures/valid.patch` satisfies the rule. `fixtures/invalid.patch`
  declares `@@ -1,3 +1,3 @@` but its body has only two old lines and two
  new lines, so both counts mismatch.

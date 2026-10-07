---
name: aurum-review
description: Ask the Aurum review gate (MCP tool aurum_gate) before every commit or push, fix what it reports and never disable, weaken or except a rule to make it pass. Use when you are about to commit, push or open a pull request in a repository that uses AurumCode.
---

# Aurum review before committing

This repository is reviewed by AurumCode in CI. The `aurum` MCP server
(`aurumcode mcp`) gives you the same gate locally, with the same policy,
skills and redaction, so you can fix a finding before anyone sees it.

## When

- Before every `git commit`, `git push` or pull request.
- After fixing a finding, to confirm the fix.

## How

1. Commit your work on the feature branch as usual (the review covers the
   commits between `base` and `HEAD`; uncommitted edits are not seen). If you
   must check before the commit exists, use the optional pre-commit hook in
   `hooks/pre-commit`, which reviews the staged change.
2. Call `aurum_gate` with `base` set to the branch the change will merge into
   (for example `main` or `origin/main`).
3. Read `decision`:
   - `pass`: push or open the pull request.
   - `fail`: for each entry of `blocking_findings`, read `rule_id`,
     `suggestion` and `evidence` (or call `aurum_explain` with its `id`), fix
     the code, amend or add a commit, and call `aurum_gate` again.
   - `inconclusive`: this is **not** a pass. Read `reason` (for example
     `provider_failure` when no model provider answered, `empty_change` when
     nothing was committed since `base`, or a scanner that failed), fix the
     cause or tell the user, and do not push as if it passed.
4. Use `aurum_rules` with the paths you are about to change to learn the
   conventions (skills) that apply before you write code, and `aurum_review`
   when you want the full report.

## Never

- Never edit `.aurumcode/`, the central policy, skills or exceptions to make
  the gate pass. A rule change or an exception is a decision for a human
  reviewer, in its own pull request.
- Never commit with `--no-verify` to skip the pre-commit hook.
- Never treat `inconclusive` as `pass`, and never retry until an
  inconclusive answer goes away by itself.
- Never paste a secret the review redacted (`[REDACTED]`) back into the code
  or the conversation.

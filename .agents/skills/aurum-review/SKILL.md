---
name: aurum-review
description: Publish AurumCode changes through repository CI and the batched self review. Use when committing, pushing, or opening a pull request in this repository; local MCP review is optional.
---

# Repository review and publication

Follow `AGENTS.md` and the user's existing publication authorization. Local
Aurum installation, provider credentials, scanners, and MCP access are optional;
never make them prerequisites for committing, pushing, or opening a card PR.
Do not install the optional local hook unless the user asks for it.

- Publish a card branch as a pull request to `dev`; these PRs run CI only.
- The batched `dev -> main` PR runs the required Aurum self review in GitHub,
  using the repository's configured provider. Merge only with every required
  check green. Missing provider credentials there must fail closed.
- If the user opts into local MCP review, use `aurum_rules`, `aurum_review`,
  `aurum_gate`, or `aurum_explain` as available. An inconclusive local result is
  not approval; report it accurately and continue to the repository checks.
- Fix findings without weakening `.aurumcode/`, central policy, scanners, or
  required repository checks. Keep policy changes in their own pull request.
- Use only the user's configured human identity and credentials. Never add AI
  attribution or paste a redacted secret back into code or conversation.

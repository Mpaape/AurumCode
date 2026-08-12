---
description: Start the bounded continuous AurumCode office loop toward zero backlog
agent: build
---

Start the installed Ralph Loop now with `maxIterations: 100` for this task:

Operate the AurumCode reconstruction office continuously toward zero backlog.
Each iteration is one 20-minute sprint cycle. Use the `aurumcode-office-loop`
skill and `.agents/skills/escritorio/SKILL.md`. Start by measuring the canonical
board and current fleet, then dispatch only cards authorized by
`.board/cards/ready/`, with one owner per card and isolated worktrees. Use the
maximum safe parallelism on disjoint paths. Review, skeptic, evidence, PR checks,
and `.board/validate.sh` are mandatory; do not fabricate or bypass them.

Track `done` between iterations. After two iterations without a verified card
transition, cancel the loop, write a concise blocker report under
`/tmp/aurum-manager/`, and change the approach instead of repeating. Do not
output `<promise>DONE</promise>` until the entire board is verifiably empty and
green according to the skill.

First call the Ralph Loop tool with this task and `maxIterations=100`, then
execute the first sprint cycle.

# Reconstruction Kanban

The board is a dependency-driven execution graph. It contains no human
schedule. Cards enter parallel execution as soon as their dependencies and red
tests are satisfied.

## State summary

| State | Meaning | Cards |
|---|---|---:|
| Ready | Dependencies complete; isolated test designer may establish red | 1 |
| Backlog | Fully specified, dependency-blocked work | 409 |
| Doing | Builder currently owns the patch | 2 |
| Review | Candidate is immutable and under review | 0 |
| Done | Evidence-bound double review and skeptical approval complete | 11 |
| Blocked on owner | Requires new authority or product choice | 0 |

## Dependency spine

```text
bootstrap locks + card/evidence schemas + safe OCI profiles
                |
        Go core + Git facts + fake OpenAI endpoint
                |
       +--------+-------------------+
       |                            |
stateless local review         deterministic Go docs
memory=off MVP                 no LLM/site/memory
       |                            |
       +-------- public CLI --------+
                |
      +---------+----------+------------------+
      |                    |                  |
optional memory       provider/SCM      skills/MCP/testgen
SQLite under .git     adapters          and language adapters
      +--------------------+------------------+
                           |
                  consumer demo matrix
                           |
        adversarial metrics + compatibility + release
```

This is a partial order, not a phase plan. Independent child cards run in
parallel. Each completed patch is reviewed immediately; there is no batch-wide
review barrier.

## Work queues

The authoritative queue is the set of card files. Run:

```bash
./.board/validate.sh
find .board/cards/ready -maxdepth 1 -name 'AUR-*.md' -print | sort
```

The validator rejects duplicated IDs, mismatched path/status, missing TDD,
container acceptance, mutation, or review sections, and dependencies that do
not resolve to another card.

The complete readable registry is [`INDEX.md`](INDEX.md). It lists all 423
cards with state, office, risk, dependencies, and outcome; individual card files
remain authoritative.

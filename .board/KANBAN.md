# Reconstruction Kanban

The board is a dependency-driven execution graph. It contains no human
schedule. Cards enter parallel execution as soon as their dependencies and
builder preflight are satisfied. The table below is informational; use the
card directories and `bash .board/pipeline.sh` for live state.

## State summary

| State | Meaning | Cards |
|---|---|---:|
| Ready | Dependencies complete; isolated builder passed builder preflight | 2 |
| Backlog | Fully specified, dependency-blocked work | 403 |
| Doing | Builder currently owns the patch | 1 |
| Review | Candidate is immutable and under review | 0 |
| Validating | Approved candidate awaiting same-SHA validation | 1 |
| Done | Evidence-bound delivery record complete | 16 |
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

The authoritative queue is the set of card files. Run the lightweight process
gates, never the frozen legacy validator:

```bash
bash .board/office-cycle.sh --status
bash .board/pipeline.sh
find .board/cards/ready -maxdepth 1 -name 'AUR-*.md' -print | sort
```

The pipeline rejects duplicated IDs, mismatched path/status, missing delivery
evidence, impossible active candidates, and dependencies that do not resolve to
another card. The per-card preflight adds the clean-worktree and image-runtime
checks before dispatch.

The complete readable registry is [`INDEX.md`](INDEX.md). It lists all 423
cards with state, office, risk, dependencies, and outcome; individual card files
remain authoritative.

# `standards/security-review`

This directory is the enforced secure-code-review standard. It is checked
by `tests/acceptance/AUR-015.sh` and is a normative baseline for every
future review card, human or agent-assisted.

- [`rules.md`](rules.md) — the nine-rule crosswalk: every rule binds one
  OWASP Top 10 category, one CWE identifier, and one NIST SSDF practice
  (each versioned and dated) to one of the nine `CR-*` controls this
  project already defines in
  [`.board/research/code-review-standards.md`](../../.board/research/code-review-standards.md#gates),
  plus a vulnerable and a fixed fixture under `tests/specs/AUR-015/`.

The source selection, the criteria matrix that compared six candidate
frameworks, and the working definitions of **threat**, **trust boundary**,
and **fail closed** used throughout `rules.md` are fixed in
[`.board/research/secure-code-review.md`](../../.board/research/secure-code-review.md),
not repeated here.

Changing a rule's OWASP/CWE/NIST SSDF citation, its bound control, or its
fixture pair changes what `tests/acceptance/AUR-015.sh` accepts; all three
are within this card's owned paths and reviewed together.

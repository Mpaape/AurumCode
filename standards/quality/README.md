# Quality-evidence vocabulary and abstention rule

This directory fixes the vocabulary AurumCode uses to talk about
ISO/IEC 25010:2023 product-quality characteristics as an evidence taxonomy,
and the one abstention rule every consumer of `characteristics.yaml` MUST
honor. It does not implement a scoring engine, does not compute a quality
score for this repository, and does not claim conformity with ISO/IEC
25010:2023 or any other standard. Research and source provenance live in
[`.board/research/quality-standards.md`](../../.board/research/quality-standards.md);
this file states the resulting rule, and `characteristics.yaml` is the
machine-checked data that rule applies to.

## The abstention rule

A product-quality characteristic without an observable, currently wired
metric MUST return `not_assessed`. Assigning a numeric score to a
characteristic that has no wired observable metric behind it MUST NOT
happen, and is treated as invalid evidence wherever it is found.

Concretely, for every entry in `characteristics.yaml`:

- `status: not_assessed` is the default and REQUIRED whenever `measurable`
  is `false`.
- `status: measured` is permitted only when `measurable` is `true`, which in
  turn requires both a non-null `metric` and a non-null `metric_source`.
- `score` MUST be `null` whenever `status` is `not_assessed`. A non-null
  `score` on a `not_assessed` (or `measurable: false`) entry is the exact
  failure this rule exists to catch: a number presented as a signal when no
  signal was observed.
- `score` MAY be a number only when `status` is `measured`.

No entry in the shipped `characteristics.yaml` currently has `measurable:
true`: this card does not wire any real computation, so every one of the
nine ISO/IEC 25010:2023 characteristics is reported `not_assessed` today.
Moving any single characteristic to `measured` requires a dependent card
that adds a real, repeatable, sourced computation for that characteristic
and changes only that characteristic's row — never a change to this
abstention rule.

## File

- [`characteristics.yaml`](characteristics.yaml): one entry per ISO/IEC
  25010:2023 characteristic, exactly the nine names and order fixed by
  [`tests/specs/AUR-016/characteristics.txt`](../../tests/specs/AUR-016/characteristics.txt).
  Each entry carries `name`, `measurable`, `status`, `score`, `metric`
  (nullable), and `metric_source` (nullable). `tests/acceptance/AUR-016.sh`
  parses this file directly (no YAML library — the acceptance program runs
  inside a minimal, network-denied container) and enforces the abstention
  rule above as a structural check, not a prose claim.

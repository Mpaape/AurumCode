# Quality-evidence research: ISO/IEC 25010:2023 as an evidence taxonomy

- Researched: 2026-08-04
- Status: design input for AUR-016 and quality-evidence dependents; not
  implementation proof and not a certification claim
- Scope: what vocabulary and abstention rule this repository uses to talk
  about product-quality characteristics, not a scoring engine and not an
  audited conformity assessment

This document does not claim conformity with ISO/IEC 25010:2023 or with any
other standard listed below. It uses ISO/IEC 25010:2023 only as a naming and
evidence taxonomy: a fixed vocabulary for which product-quality
characteristic an observation is about, and a rule for what to report when no
observable metric exists for that characteristic. No score, certification, or
audit outcome is produced by this document or by `standards/quality/`.

## Sources examined

| Source | Version | Date | Scope used in this project |
|---|---|---|---|
| [ISO/IEC 25010:2023(en), Product quality model](https://www.iso.org/standard/78176.html) | Edition 2 | 2023-11-01 | Canonical nine-characteristic set and their names; this project reads only the publicly catalogued title, edition, and characteristic names, and does not claim conformity to the paywalled full text. |
| [arc42 quality model: "Update on ISO 25010, version 2023"](https://quality.arc42.org/articles/iso-25010-update-2023) | rev 2023 | 2023-11-01 | Independent secondary confirmation that Usability was renamed Interaction capability, Portability was renamed Flexibility, and Safety was added as a ninth characteristic. |
| [SARM: "New Version of ISO 25010 released"](https://sarm.org.uk/2023/12/06/new-version-of-iso-25010-released/) | post 2023-12-06 | 2023-12-06 | Second independent secondary confirmation of the same nine-characteristic set and the three renamed/added characteristics, published by an unrelated party five weeks after the arc42 article. |

None of the three sources above is treated as authoritative on measurement
technique or scoring; they are used only to fix the characteristic names this
project's vocabulary must resolve. `standards/quality/characteristics.yaml`
is the machine-checked artifact derived from this table, not the table
itself.

## The nine ISO/IEC 25010:2023 characteristics

ISO/IEC 25010:2023 (edition 2) revises the eight characteristics of the 2011
first edition into nine: Usability was renamed **Interaction capability**,
Portability was renamed **Flexibility**, and **Safety** was added as a new
ninth characteristic. The full canonical set, in the order used throughout
this project's artifacts:

1. Functional suitability
2. Performance efficiency
3. Compatibility
4. Interaction capability
5. Reliability
6. Security
7. Maintainability
8. Flexibility
9. Safety

`tests/specs/AUR-016/characteristics.txt` is the single source of truth for
this exact list (name, count, order); the acceptance lint and
`standards/quality/characteristics.yaml` both resolve against that fixture
instead of duplicating the list.

## Findings

1. A "quality score" is only evidence when it traces to an observable,
   currently wired metric with a named source. Absent that, a numeric score
   is an unsupported claim dressed as a measurement — the exact failure mode
   `.board/README.md` and `CR-EVD-*` forbid for any card evidence.
2. This card's non-goal is explicit: it does not build a scoring engine and
   does not emit a quality score for the AurumCode repository. No metric
   listed in `standards/quality/characteristics.yaml` is wired to a live
   computation yet; every characteristic is therefore reported as
   `not_assessed` today. Wiring a real, computed metric for any
   characteristic is left to a dependent card, which changes only that one
   characteristic's row from `not_assessed` to `measured` once it has a
   real, sourced, wired signal — never by editing this vocabulary card.
3. Two failure modes are distinguished because they have different causes
   and different fixes:
   - No candidate metric is even named for a characteristic (the taxonomy
     itself has nothing observable to offer yet) — `not_assessed`.
   - A candidate metric is named (with a source) but is not wired into any
     computation this repository runs — also `not_assessed`, because
     "named" is not "observed."
   Both collapse to the same reported status; a consumer of
   `standards/quality/characteristics.yaml` never needs to distinguish them
   to decide whether to trust a score, because in both cases there isn't
   one.
4. The single invariant this card fixes, and the one its skeptical mutation
   (`MUT-001`) targets directly: **a characteristic entry may carry a numeric
   `score` only if it is `measurable: true` and cites a `metric` and a
   `metric_source`.** Any entry with `measurable: false` (equivalently,
   `status: not_assessed`) that also carries a non-null `score` is invalid
   evidence and the acceptance lint rejects it — assigning a number is not
   the same as observing a signal.
5. Source provenance for the taxonomy itself is bound the same way
   `.board/research/memory-design.md` binds its alternatives matrix: every
   cited source needs a `[label](url)` link whose host is on an explicit
   allowlist (`tests/specs/AUR-016/allowed-source-domains.txt`), a version
   token, and an ISO-8601 date. `MUT-002` targets exactly this: replacing a
   cited source with a non-allowlisted host or a version-less citation must
   make the acceptance lint fail.

## Vocabulary this card fixes

- **Characteristic**: one of the nine ISO/IEC 25010:2023 product-quality
  names listed above. Fixed, closed set; not extensible by an individual
  card.
- **Candidate metric**: an informational, human-readable description of a
  metric that could, in principle, evidence a characteristic (e.g. "static
  analysis finding density per changed hunk" for Maintainability). Naming a
  candidate metric is not a claim that it is computed anywhere in this
  repository.
- **Metric source**: a citation (standard, paper, or tool documentation) for
  where a candidate metric's definition comes from. Required whenever a
  candidate metric is named; optional (and null) otherwise.
- **`measurable`**: `true` only when a candidate metric is both named and
  currently wired to a real, repeatable computation in this repository.
  `false` in every other case, including "a metric is named but nothing
  computes it yet."
- **`status`**: `measured` when `measurable: true` and a `score` has actually
  been computed and recorded from that wiring; `not_assessed` in every other
  case. There is no third status. A characteristic is never silently
  dropped, and never defaults to a numeric score by omission.
- **`score`**: nullable. MUST be null whenever `status` is `not_assessed`.
  MAY be a number only when `status` is `measured`.

This vocabulary and the abstention rule above are consumed by
`standards/quality/characteristics.yaml` and enforced by
`tests/acceptance/AUR-016.sh`. Building the first real, wired metric for any
one characteristic — and therefore moving its row from `not_assessed` to
`measured` — is out of scope for this card and is left to a dependent card
under the relevant office.

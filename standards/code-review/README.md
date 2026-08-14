# Versioned code-review standard

This directory is the card-scoped, machine-checkable catalog for the nine
`CR-*` controls in the code-review baseline. It is policy data, not a scanner,
review engine, approval service, or publication adapter.

## Contract

`cases.yaml` is a strict, bounded YAML subset consumed by
`tests/acceptance/AUR-014.sh`. Each rule has:

- one primary official source, an explicit version, an ISO date, and
  `status: current`;
- one `positive` case with `expect: pass`;
- one `negative` case with `expect: fail`;
- a distinct local fixture under this directory; and
- a non-empty assertion describing the observation the fixture represents.

The acceptance program checks the catalog without network access. It resolves
the controls declared by AUR-014, rejects duplicate or missing rules, rejects
withdrawn or non-allowlisted sources, and rejects missing, reused, unbound, or
indistinguishable fixtures. The fixtures are examples for the rule contract;
they are not a claim of external certification or conformity.

## Delivery boundary

This card delivers a versioned standard only. It does not execute a review,
assign an approval, scan a diff, fetch a source, or create a reviewer identity.
The current lightweight delivery process validates the catalog with its own
acceptance command and a human-authored commit; the retired multi-reviewer
ceremony is not part of this artifact.

## Crosswalk

The complete `CR -> source/version -> examples -> card/AC/mutation` crosswalk
is recorded here so every catalog row has an explicit acceptance boundary.

| Control | Primary source and version | Positive / negative | Card coverage | Mutation |
|---|---|---|---|---|
| `CR-EVD-001` | NIST SP 800-218, SSDF 1.1 | command-bound evidence / unbound claim | AUR-014 AC-001 | MUT-001 |
| `CR-EVD-002` | NIST SP 800-218, SSDF 1.1 | changed input invalidates verdict / stale verdict survives | AUR-014 AC-001 | MUT-001 |
| `CR-EVD-003` | NIST SP 800-218, SSDF 1.1 | red-green-mutation sequence / green-only claim | AUR-014 AC-001 | MUT-001 |
| `CR-REV-001` | Google Engineering Practices, current online edition | complete context / sampled context | AUR-014 AC-001 | MUT-001 |
| `CR-REV-002` | Google Engineering Practices, current online edition | sealed result / leaked result | AUR-014 AC-001 | MUT-001 |
| `CR-REV-003` | NIST SP 800-218, SSDF 1.1 | isolated inputs / shared state | AUR-014 AC-001 | MUT-001 |
| `CR-REV-004` | NIST SP 800-218, SSDF 1.1 | reversible challenge fails / challenge omitted | AUR-014 AC-001 | MUT-001 |
| `CR-REV-005` | Google Engineering Practices, current online edition | blocking issue rejects / issue waived | AUR-014 AC-001 | MUT-001 |
| `CR-GATE-001` | NIST SP 800-218, SSDF 1.1 | ordered fail-closed gate / downstream override | AUR-014 AC-001 | MUT-002 |

The source-selection rationale and the definitions used by the controls remain
in [the research baseline](../../.board/research/code-review-standards.md).

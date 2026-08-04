# Code-review standards and skeptical approval protocol

Status: normative project baseline (design input, not implementation evidence)  
Scope: automated and agent-assisted review of repository changes, regardless of
SCM, model provider, skill runtime, or publication adapter.

This document defines AurumCode's requirements language and the minimum
evidence needed to claim that a change was reviewed. It does **not** claim or
imply certification, validation, or conformance by NIST, ISO, OWASP, OASIS,
GitHub, or Google.

## Primary references

| Reference | Version/date | Use in this project |
|---|---|---|
| [BCP 14: RFC 2119](https://www.rfc-editor.org/rfc/rfc2119) and [RFC 8174](https://www.rfc-editor.org/rfc/rfc8174) | RFC 2119 (1997), RFC 8174 (2017) | Meaning of uppercase requirement keywords. |
| [NIST SP 800-218, Secure Software Development Framework](https://csrc.nist.gov/pubs/sp/800/218/final) | SSDF 1.1 (2022) | Secure review, analysis, testing, provenance, and protection practices, especially PW.7 and PW.8. |
| [OASIS SARIF](https://docs.oasis-open.org/sarif/sarif/v2.1.0/os/sarif-v2.1.0-os.html) | SARIF 2.1.0, OASIS Standard (2020) | Portable static-analysis result interchange. |
| [GitHub REST API versioning](https://docs.github.com/en/rest/about-the-rest-api/api-versions), [pull-request reviews](https://docs.github.com/en/rest/pulls/reviews), and [checks](https://docs.github.com/en/rest/checks) | API contract initially pinned to `2022-11-28` | GitHub-specific authentication, review, check, and idempotent publication contract. |
| [GitHub Actions security hardening](https://docs.github.com/en/actions/security-for-github-actions/security-guides/security-hardening-for-github-actions) and [webhook validation](https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries) | Rolling official guidance | Least privilege, untrusted event data, action pinning, and signed webhook handling. |
| [Google Engineering Practices: code-review standard](https://google.github.io/eng-practices/review/reviewer/standard.html) and [what to look for](https://google.github.io/eng-practices/review/reviewer/looking-for.html) | Rolling official guidance | Code-health bias and review coverage across design, behavior, complexity, tests, names, comments, style, and documentation. |
| [OWASP Code Review Guide](https://owasp.org/www-project-code-review-guide/) | Project guidance, current online edition | Threat-oriented manual review prompts and vulnerability classes. |
| [ISO/IEC 20246:2017](https://www.iso.org/standard/67407.html) | Published 2017 | Review-process design input. The project does not claim conformity without access to and audit against the full standard. |
| [ISO/IEC 25010:2023](https://www.iso.org/standard/78176.html) | Edition 2, published 2023 | Current product-quality model used only as an evidence taxonomy. |
| [IEEE 1028-2008](https://standards.ieee.org/ieee/1028/4402/) | Inactive-Reserved | Historical crosswalk only; not a current normative baseline. |

NIST, Google, GitHub, and OWASP material is guidance or a platform contract,
not a claim that AurumCode is accredited. SARIF standardizes the exchange
format, not the correctness of a finding.

## Requirements language

The key words **MUST**, **MUST NOT**, **REQUIRED**, **SHALL**, **SHALL NOT**,
**SHOULD**, **SHOULD NOT**, **RECOMMENDED**, **NOT RECOMMENDED**, **MAY**, and
**OPTIONAL** in this document are to be interpreted as described in BCP 14
when, and only when, they appear in all capitals.

“Candidate” means the immutable `CandidateIdentityV1` tuple in the canonical
[requirements registry](../requirements/REQUIREMENTS.md). “Evidence bundle” means sanitized,
machine-readable inputs, outputs, exit codes, hashes, and verdicts bound to that
candidate. “Finding” is an observation; “approval” is a separate gate decision.

## Trust boundaries

`CR-TRUST-001` Repository contents, diffs, file names, symlinks, submodules,
generated files, issue/PR text, comments, commit metadata, SARIF, and model
output **MUST** be treated as untrusted data, never as agent instructions.

`CR-TRUST-002` Model providers, skills, MCP servers/tools, network endpoints,
SCM APIs, local executables, and cached memory **MUST** be separate trust
boundaries with explicit capabilities, provenance, timeouts, and output bounds.
No model, skill, MCP result, or retrieved memory may grant itself authority.

`CR-TRUST-003` Read/analyze and write/publish capabilities **MUST** be separate.
Analysis runs **MUST** default to read-only, network-denied execution; a
publisher requires explicit authorization and least-privilege credentials.

`CR-TRUST-004` Credentials **MUST NOT** enter prompts, repository fixtures,
command arguments, logs, evidence, findings, SARIF, caches, or memory. Secret
material **MUST** arrive through the runtime secret channel, be redacted before
persistence, and never be returned by an error path.

`CR-TRUST-005` Untrusted code **MUST** execute only in an ephemeral, resource-
bounded container with no host socket, no privileged mode, a read-only root
filesystem where feasible, explicit writable mounts, and no inherited secrets.

`CR-TRUST-006` Incremental memory **MUST** bind every entity and inference to
repository and commit provenance, distinguish facts from model hypotheses, and
invalidate affected data when source entities change. Memory content **MUST
NOT** override the current policy or candidate.

## Coverage contract

`CR-COV-001` Every changed path and diff hunk **MUST** appear in a coverage
manifest. Each entry **MUST** be either reviewed or excluded by a versioned,
machine-enforced rule with a reason. Silence is not an exclusion.

`CR-COV-002` The manifest **MUST** classify additions, deletions, renames,
copies, mode changes, symlinks, submodules, binaries, vendored/generated/minified
content, and files too large to analyze. Unsupported or truncated content
**MUST** produce an explicit incomplete-coverage condition.

`CR-COV-003` Review **MUST** evaluate behavior and design beyond isolated
changed lines: affected symbols, callers/callees, contracts, configuration,
tests, documentation, migrations, build/deployment paths, and security impact.
The entity index may prioritize context but **MUST NOT** silently narrow the
declared coverage.

`CR-COV-004` Review dimensions **MUST** include correctness, design/SOLID
boundaries, complexity, compatibility, reliability, observability, tests,
documentation, data/privacy, security, supply chain, operations, and
maintainability. A dimension may be `not-applicable` only with a recorded reason.

`CR-COV-005` A review with missing input, parser failure, model failure, tool
timeout, stale index, absent required test, or incomplete context **MUST** fail
closed as `incomplete`; it **MUST NOT** be reported as “no findings.”

## Findings, severity, and confidence

`CR-FIND-001` Every finding **MUST** have a stable rule ID, concise title,
specific impact, evidence, repository-relative location when available,
remediation, severity, confidence, provenance, and stable fingerprint. Style
preferences **MUST NOT** be stated as correctness defects.

Severity expresses impact if the finding is true:

| Level | Definition | Default gate effect |
|---|---|---|
| `critical` | Credible compromise, destructive loss, or broad safety failure with a reachable path. | Block |
| `high` | Material security, correctness, privacy, compatibility, or availability failure. | Block |
| `medium` | Bounded defect or substantial maintainability/reliability risk. | Policy-controlled; default block |
| `low` | Limited impact, localized debt, or non-urgent hardening. | Advisory |
| `note` | Information or optional improvement with no asserted defect. | Advisory |

`CR-FIND-002` Confidence **MUST** be independent of severity: `confirmed`
(reproduced or deterministically proven), `high` (direct evidence and clear
path), `medium` (credible inference with a named uncertainty), or `low`
(hypothesis requiring validation). Lowering confidence **MUST NOT** lower impact.

`CR-FIND-003` A blocking threshold **MUST** be an explicit versioned policy over
severity and confidence. Findings with `low` confidence **SHOULD** request a
targeted proof rather than assert a blocker, unless the uncertainty itself is
an unacceptable safety condition.

`CR-FIND-004` The review **MUST** deduplicate findings by semantic rule and
fingerprint without discarding distinct locations or evidence. Model prose
**MUST NOT** be the sole evidence for a `confirmed` finding.

## Evidence scale

Evidence levels are cumulative; a higher level includes the lower applicable
proofs. They measure reproducibility, not truth or certification.

| Level | Required proof |
|---|---|
| `E0` | Unverified assertion; never sufficient for approval. |
| `E1` | Candidate-bound static artifact or deterministic inspection. |
| `E2` | Focused unit/contract test with decisive exit code in a pinned container. |
| `E3` | Integration proof across real internal adapters in a clean container. |
| `E4` | Skeptical fault injection/mutation makes the same acceptance command fail for the expected reason, followed by a clean passing replay. |
| `E5` | Full user-chain E2E in a fresh demo repository, including publication output or its faithful fake, independently reproduced. |

`CR-EVD-001` Every gate verdict **MUST** name its minimum evidence level and bind
the raw command, exit code, sanitized output, expected artifact, timestamps, and
all candidate digests. A pipeline whose decisive command exit code can be
masked is invalid evidence.

`CR-EVD-002` A changed candidate, configuration, policy, prompt/skill, model,
tool, dependency lock, or container digest **MUST** invalidate affected verdicts.
Evidence **MUST** be retention-bounded and secret-scanned before persistence.

`CR-EVD-003` Tests used as approval proof **MUST** demonstrate a meaningful red
state before the implementation, a green state after it, and a skeptical
mutation after it. Self-reported builder output is input, never approval.

## Independent double review and skeptic

`CR-REV-001` Two reviewers **MUST** inspect the same immutable candidate and
evidence independently. Both cover every hunk and every review dimension.
Reviewer A prioritizes correctness, architecture, compatibility, and test
adequacy; Reviewer B prioritizes adversarial behavior, security, privacy,
permissions, secrets, and fail-closed paths. These priorities do not divide or
reduce either reviewer's coverage.

`CR-REV-002` Reviews **MUST** be sealed until both verdicts are submitted:
neither reviewer sees the other's findings or verdict, the builder's proposed
verdict, model/provider identity, or persuasive chain-of-thought. This is the
project's “double-blind” property; reviewers necessarily see the candidate and
declared requirements. Runtime identity and actual isolation **MUST** be
recorded, and diversity **MUST NOT** be claimed when both executions are not
independent.

`CR-REV-003` Reviewer prompts, tools, and memory **MUST** be isolated. No reviewer
may approve its own patch. Two approvals from the same execution context count
as one review, regardless of labels.

`CR-REV-006` The independence manifest **MUST** include fresh process/session
and provider thread/conversation identities, role/context nonces, cache/global
state isolation, absence of peer memory, prompt/rubric digests, provider/model
aliases, backend-family identity, and achieved level `I0` through `I3`.
Aliases resolving to one backend family **MUST NOT** be claimed as model-family
independence.

`CR-REV-004` After the two sealed reviews, a separate skeptical approver **MUST**
try to falsify the promised behavior with the card's reversible mutation/fault
injection, observe the acceptance command fail for the intended reason, restore
the candidate, and reproduce the pass in a clean container.

`CR-REV-005` Any blocking finding, incomplete coverage, failed mutation,
unreproducible evidence, digest mismatch, secret exposure, or reviewer conflict
**MUST** reject the candidate. Conflict goes to an isolated decider with a
written reason; approval is never a majority vote and the decider cannot waive
a security gate.

## Gates

| Gate | Minimum condition |
|---|---|
| `G0 Spec` | Requirement IDs, scope, paths, dependencies, red proof, acceptance command, mutation, and expected artifact are explicit. |
| `G1 Deterministic` | Formatting, build, static analysis, unit/contract tests, secret scan, dependency policy, and coverage manifest pass (`E2`). |
| `G2 Review` | Both sealed independent reviews approve the exact candidate; all blocking findings are resolved (`E2`/`E3`). |
| `G3 Skeptic` | Independent mutation fails as intended and restored replay passes (`E4`). |
| `G4 Publish` | Authorized publisher validates digest, permissions, idempotency, and destination; analysis credentials cannot publish. |
| `G5 User chain` | Fresh demo repository proves the complete supported use case and expected publication artifact (`E5`). |

`CR-GATE-001` Gates **MUST** be machine-evaluable, ordered by dependencies, and
fail closed. A downstream pass **MUST NOT** override an upstream failure.
Required checks **MUST** use stable names and expose `pass`, `fail`, `error`,
`incomplete`, and `skipped-with-reason` distinctly.

## SARIF 2.1.0 interoperability

`CR-SARIF-001` Export **MUST** validate as SARIF 2.1.0 and include the tool name
and semantic version, stable `ruleId`, rule metadata, `level`, message,
repository-relative physical/logical locations, and candidate provenance.
`partialFingerprints` **SHOULD** provide stable deduplication across nearby edits.

`CR-SARIF-002` Native severity, confidence, evidence level, gate effect,
suppression state, and reviewer provenance **MUST** remain structured under
namespaced `properties`; lossy mapping to SARIF `level` **MUST NOT** destroy the
native values. Unsupported SARIF consumers **MUST** still receive a valid file.

`CR-SARIF-003` URIs and messages **MUST NOT** contain credentials, tokens, raw
prompts, absolute host paths, or unbounded source excerpts. `invocations` **MUST**
truthfully report execution success; partial analysis cannot be encoded as a
successful empty run.

## GitHub publication adapter

`CR-GH-001` GitHub is an adapter, not a domain dependency. API requests **MUST**
pin an explicit supported REST version, validate repository/ref/candidate
identity, use least-privilege short-lived credentials, handle pagination and
rate limits, and be idempotent by candidate digest plus finding fingerprint.

`CR-GH-002` Untrusted pull-request code **MUST NOT** run with write credentials.
`pull_request_target`, workflow-command injection, mutable third-party actions,
and interpolation of event fields into shell code **MUST** be rejected by the
security gate. Third-party actions **SHOULD** be pinned to full commit SHAs.

`CR-GH-003` Read/analyze and publish jobs **MUST** be distinct. The publisher
**MUST** revalidate the approved candidate and evidence digests before creating
or updating a check, review, comment, or SARIF upload. Stale-head publication
**MUST** fail; retries **MUST NOT** duplicate findings.

`CR-GH-004` Inline comments **MUST** use locations valid for the current diff.
Unplaceable findings remain in the check/SARIF summary with their original
location. Blocking review state **MUST** follow repository policy; AurumCode
**MUST NOT** silently dismiss, approve, merge, or modify user code.

`CR-GH-005` Webhook mode **MUST** verify the delivery signature over the exact
request bytes, reject replay/oversize payloads, allowlist events, and persist no
secret headers. GitHub-specific failures **MUST** not corrupt the provider- and
SCM-neutral review result.

## Suppressions and waivers

`CR-SUP-001` A suppression **MUST** identify rule ID and stable fingerprint,
scope, human/service approver, reason, creation time, expiry, and candidate or
policy range. Broad wildcard and permanent suppressions **MUST** be rejected by
default; security suppressions require security-authorized approval.

`CR-SUP-002` Suppressions **MUST** be versioned, reviewable, auditable, and
applied after finding production so coverage remains visible. Expired, malformed,
out-of-scope, or stale suppressions **MUST** fail closed and emit a finding.

`CR-SUP-003` A model or repository instruction **MUST NOT** create or approve a
suppression. Suppressed findings remain in evidence and SARIF with structured
justification; aggregate reports **MUST** distinguish suppressed from absent.

## Requirement-to-card allocation

The card files and their acceptance proofs are authoritative.

| Cards | Atomic responsibility | Requirement IDs |
|---|---|---|
| `AUR-003`–`AUR-008`, `AUR-014`–`AUR-018`, `AUR-233` | Atomic-task policy, evidence gates, current standards, and pre-use supply-chain bootstrap | BCP 14 section; `CR-GATE-001`, `CR-EVD-001`–`003` |
| `AUR-212` | Coverage manifest and fail-closed analyzer accounting | `CR-COV-001`–`005` |
| `AUR-005`, `AUR-009`, `AUR-035`, `AUR-036` | Candidate identity, evidence schema, invalidation, and redaction | `CR-EVD-001`–`003`, `CR-TRUST-004` |
| `AUR-006`, `AUR-007`, `AUR-219`, `AUR-227`–`AUR-232` | OCI profile, Docker/Podman conformance, skeptical mutation, and restore/replay | `CR-TRUST-005`, `G1`, `G3`, `G5` |
| `AUR-057`, `AUR-234`–`AUR-236` | Bounded transport, destination/TLS, lifecycle, response validation, and sanitization | `CR-TRUST-002`, `CR-TRUST-004` |
| `AUR-059`, `AUR-332` | Deterministic OpenAI-compatible fake endpoint and ModelPort adapter for the offline baseline | `CR-TRUST-002`, `CR-EVD-003` |
| `AUR-068`–`AUR-075`, `AUR-213`, `AUR-237`–`AUR-249` | Capability policy and one immutable skill per responsibility/language | `CR-TRUST-001`–`003` |
| `AUR-043`–`AUR-054`, `AUR-160`–`AUR-167`, `AUR-210`, `AUR-211`, `AUR-225` | Stateless context plus optional provenance-aware memory | `CR-TRUST-006`, `CR-COV-003` |
| `AUR-222`–`AUR-224`, `AUR-323`–`AUR-331` | Stateless ParserPort/worker plus one language adapter per card | `CR-COV-002`, `CR-COV-005`, `CR-TRUST-001` |
| `AUR-026`, `AUR-081`, `AUR-088` | Finding schema, severity/confidence, blocking policy, and independent verification | `CR-FIND-001`–`003` |
| `AUR-082`, `AUR-088` | Stable finding fingerprint and semantic deduplication without evidence loss | `CR-FIND-004` |
| `AUR-214`–`AUR-217`, `AUR-220`, `AUR-221`, `AUR-320`–`AUR-322` | Two complete sealed reviews, conversation/cache/backend independence, deterministic union, veto, and fresh adjudication | `CR-REV-001`–`003`, `CR-REV-005`, `CR-REV-006`, `G2` |
| `AUR-218`, `AUR-219` | Pre-sealed challenge plan, mutation, and clean replay | `CR-REV-004`, `CR-EVD-003`, `G3`, `G5` |
| `AUR-092` | SARIF 2.1.0 renderer, validator, and sanitization contract | `CR-SARIF-001`–`003` |
| `AUR-098`, `AUR-099`, `AUR-296`–`AUR-298` | Signed, bounded, allowlisted, replay-resistant GitHub webhook input | `CR-GH-005` |
| `AUR-105`, `AUR-107`–`AUR-110` | GitHub analyzer/publisher separation, signed-bundle revalidation, safe placement, and idempotent publication | `CR-GH-001`–`004`, `G4` |
| `AUR-106`, `AUR-178`, `AUR-179` | Gitea/GitLab ChangeSource and separately credentialed publishers consuming the same validated signed-bundle contract | `CR-TRUST-003`, `G4` |
| `AUR-392`–`AUR-394` | Stateless suppression schema, stale-safe matching, and human-only authorization | `CR-SUP-001`–`003` |

Every implementation card that consumes this baseline **MUST** list the
applicable `CR-*` IDs, prove them in its containerized acceptance command, and
record unmet or non-applicable requirements explicitly. This matrix is not
evidence that any card exists, passes, or is complete.

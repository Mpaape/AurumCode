// Package feedback is the feedback loop of the gate (AUR-532): it turns what
// already happens on GitHub into signals about the policy and proposes, in a
// single pull request to the policy repository, changes to the skills and
// candidate cases for the AUR-523 corpus. It never changes the policy by
// itself: a human merges the pull request or nothing happens.
//
// Signals come from data that already exists and is maintained by
// automation, never from a database of its own:
//
//   - a code scanning alert dismissed as a false positive;
//   - a blocking finding of one audit record (AUR-521) that is gone in the
//     next audit record of the same pull request, whose diff changed the
//     flagged lines (a true positive that got fixed);
//   - a `/aurum perdeu <descricao>` comment by a repository member (a defect
//     that passed the gate).
//
// Every free-text field is redacted (AUR-009) when the signal is built, so
// neither the model nor the pull request ever sees a secret. Text from
// GitHub is data: it is quoted, never obeyed. The model only groups signals
// into proposals; a proposal that cites no known signal is discarded.
package feedback

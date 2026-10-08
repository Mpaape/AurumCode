// Package verify is the adversarial check of the model's own findings that
// would block the gate. Each one is shown, with the code it cites and the
// definitions of the symbols it names (read from the reviewed revision), to
// a separate call of the same provider, which answers confirmed, refuted or
// uncertain with an exact quote of the code it relies on. Only a refutation
// whose quote exists literally in the shown files demotes the finding to a
// non-blocking, marked comment; every other outcome (confirmed, uncertain,
// a quote that is not in the code, an invalid answer, a provider error, the
// call ceiling, an unreadable file) keeps it blocking. Findings of the
// deterministic engines are never sent, and the verifier cannot create one.
package verify

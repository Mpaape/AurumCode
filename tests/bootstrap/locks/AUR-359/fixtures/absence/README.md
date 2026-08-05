# Fixture: absence

Deliberately empty of `.board/bootstrap/locks/trust-root.yml`. Used by the
`absence` row of `tests/bootstrap/locks/AUR-359/cases.tsv` to prove
`verify_trust_root_dir` returns `trust_root_mismatch` when the lock this card
owns is simply not there. This file exists only so git tracks the directory;
`tests/acceptance/AUR-359.sh` never reads it.

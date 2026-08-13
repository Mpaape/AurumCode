# Declarative source of truth for the AUR-442 hardcoded-secret fixture.
#
# `tests/fixtures/repos/git-demo/build-fixture.sh` turns this file plus the
# `content/` overlay into the byte-identical bare repository under
# `repo.git/`, exactly as it does for the git-demo and AUR-435 vuln
# fixtures it was written for (same schema, same limits, same determinism
# guarantees). Rebuild:
#
#   bash tests/fixtures/repos/git-demo/build-fixture.sh \
#     tests/fixtures/review/vuln/hardcoded-secret/history.spec <fresh-output-dir>
#
# This fixture is deliberately separate from
# tests/fixtures/review/vuln/repo.git (AUR-435's own fixture): that
# repository's `HEAD~1..HEAD` diff is pinned byte for byte by AUR-435's
# frozen acceptance and integration tests, so AUR-442 must never add a
# commit to it -- doing so would shift what `HEAD~1` resolves to and
# rewrite the diff those tests already assert on. This is a sibling
# repository instead.
#
# The second commit introduces two hardcoded, fully synthetic secrets in
# two different shapes: an unquoted `KEY=VALUE` env-style line
# (config/secrets.env, the same shape tests/fixtures/repos/git-demo's own
# planted secret uses) and a quoted Python assignment
# (src/config.py, the shape more common in application source). Each file
# also carries one deliberately benign line -- a plain service name, a
# plain timeout constant -- that AUR-442's hardcoded-secret pattern must
# NOT match, so `aurumcode review --base HEAD~1 --seguranca` reports
# exactly two findings, not four. Nothing in this fixture is a real
# application, credential, or database.
schema aurum.git-demo-history
version 1
branch main
author Aurum Demo Author <demo-author@aurum.invalid>
committer Aurum Demo Bot <demo-bot@aurum.invalid>
timezone +0000
limit max-commits 16
limit max-files-per-commit 16
limit max-object-bytes 65536
limit max-total-bytes 4194304
content-root content

commit 01 1700000000 seed: add the hardcoded-secret fixture skeleton
add 01 README.md
add 01 src/app.py

commit 02 1700000060 chore: plant two synthetic hardcoded secrets
add 02 config/secrets.env
add 02 src/config.py

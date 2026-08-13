# Declarative source of truth for the AUR-435 vulnerability fixture.
#
# `tests/fixtures/repos/git-demo/build-fixture.sh` turns this file plus the
# `content/` overlay into the byte-identical bare repository under
# `repo.git/`, exactly as it does for the git-demo fixture it was written
# for (same schema, same limits, same determinism guarantees). Rebuild:
#
#   bash tests/fixtures/repos/git-demo/build-fixture.sh \
#     tests/fixtures/review/vuln/history.spec <fresh-output-dir>
#
# The second commit introduces src/db.py, which builds a SQL query by
# string concatenation: a deliberately planted, fully synthetic SQL
# injection so `aurumcode review --base HEAD~1 --seguranca` has a real
# vulnerability to find (AUR-435). Nothing in this fixture is a real
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

commit 01 1700000000 seed: add the vuln demo skeleton
add 01 README.md
add 01 src/app.py

commit 02 1700000060 feat: look users up by name
add 02 src/db.py

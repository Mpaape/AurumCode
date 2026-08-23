# Declarative source of truth for the AUR-466 Node placeholder-vs-secret
# fixture. tests/fixtures/repos/git-demo/build-fixture.sh turns this file
# plus the content/ overlay into the byte-identical bare repository under
# repo.git/, exactly as it does for the git-demo, AUR-435/AUR-442/AUR-462
# fixtures it was written for. Rebuild:
#
#   bash tests/fixtures/repos/git-demo/build-fixture.sh \
#     tests/fixtures/review/vuln/node-placeholder-vs-secret/history.spec \
#     <fresh-output-dir>
#
# The second commit adds two files planting every AC-001 false-positive
# shape the 2026-08-14 measurement found (a doc placeholder, a help-text
# string teaching the export, two test-fixture assignments whose values
# are digit-free placeholder/label words) beside every AC-002 true-positive
# shape (a real digit-bearing secret, SQL concatenating a variable, a
# shell command concatenating a variable). Nothing here is a real
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

commit 01 1700000000 seed: add the placeholder-vs-secret fixture skeleton
add 01 NOTES.md

commit 02 1700000060 chore: plant placeholder shapes beside real secret and variable concats
add 02 README.md
add 02 src/app.js

# Declarative source of truth for the AUR-462 Node command-injection/xss
# fixture. tests/fixtures/repos/git-demo/build-fixture.sh turns this file
# plus the content/ overlay into the byte-identical bare repository under
# repo.git/, exactly as it does for the git-demo, AUR-435 vuln, and
# AUR-442 hardcoded-secret fixtures it was written for. Rebuild:
#
#   bash tests/fixtures/repos/git-demo/build-fixture.sh \
#     tests/fixtures/review/vuln/node-xss-command-injection/history.spec \
#     <fresh-output-dir>
#
# The second commit adds a single Node source file planting every shape
# this card's security/command-injection and security/xss patterns must
# find (exec/execSync concatenation, spawn with shell:true, a direct
# innerHTML write) beside every shape they must NOT find (argv-form exec,
# a comment mentioning "exec", an innerHTML literal, a sanitizer call, an
# uppercase module constant, an escaping-helper call). Nothing here is a
# real application, credential, or database.
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

commit 01 1700000000 seed: add the node fixture skeleton
add 01 README.md

commit 02 1700000060 chore: plant node exec and innerHTML shapes, safe unsafe
add 02 src/app.js

# Declarative source of truth for the AurumCode demo Git fixture.
#
# `build-fixture.sh` turns this file plus the `content/` overlay into the
# byte-identical bare repository under `repo.git/`. Nothing here is read from
# the clock, the environment or a remote, so the same spec always produces the
# same object ids and the same fixture digest.
#
# An operation names its content version explicitly (`add <version> <path>`),
# so a content file is bound to the change that introduces it and not to the
# position of the commit that carries it. Commits 02 and 03 touch disjoint
# paths on purpose: swapping the two bodies below yields a history that is
# still valid and still has the same three messages, but in a different order
# and therefore with different commit ids. That is exactly what MUT-001 does,
# and the acceptance must notice it.
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

commit 01 1700000000 seed: add the demo project skeleton
add 01 README.md
add 01 NOTES.txt
add 01 config/app.yaml

commit 02 1700000060 feat: add the greeter and point the config at it
add 02 src/greeter.py
modify 02 config/app.yaml

commit 03 1700000120 chore: plant synthetic tokens and drop the scratch notes
add 03 config/demo-tokens.txt
delete NOTES.txt

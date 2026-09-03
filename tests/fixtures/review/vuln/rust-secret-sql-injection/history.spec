# Declarative source of truth for the AUR-481 Rust fixture.
# tests/fixtures/repos/git-demo/build-fixture.sh turns this file plus the
# content/ overlay into the byte-identical bare repository under repo.git/,
# exactly as it does for the git-demo, AUR-435/442/462/466 vuln fixtures it
# was written for. Rebuild:
#
#   bash tests/fixtures/repos/git-demo/build-fixture.sh \
#     tests/fixtures/review/vuln/rust-secret-sql-injection/history.spec \
#     <fresh-output-dir>
#
# The second commit adds a single Rust source file planting every idiomatic
# Rust shape this card's security/hardcoded-secret and security/sql-injection
# patterns must find (a typed const literal, a String::from-wrapped literal,
# .to_owned()-then-+ SQL concatenation, a format! SQL query) beside every
# shape they must NOT find (a numeric constant, a digit-free string
# constant, a $1-parametrized query). Rust command-injection
# (`Command::new("sh").arg("-c")...`) is a separate, documented gap: see
# docs/specs/AUR-481.md for why it is out of this card's scope. Nothing
# here is a real application, credential, or database.
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

commit 01 1700000000 seed: add the rust fixture skeleton
add 01 README.md

commit 02 1700000060 chore: plant rust secret, sql and command shapes
add 02 src/main.rs

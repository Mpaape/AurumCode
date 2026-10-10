#!/usr/bin/env bash
# AUR-611 acceptance: the image stamps the AurumCode version into the binary
# (Dockerfile ARG AURUMCODE_VERSION, default dev); the review workflow
# resolves it from the tool's SHA (exact release tag, else the short SHA)
# and passes it to the build; the parecer's details and the audit record
# name it only when it is not dev; the SARIF carries it.
#
# Selectors:
#   all      AC-001..AC-004, MUT-001
#   AC-001   the Dockerfile's own build line with AURUMCODE_VERSION=v9.9.9
#            gives a binary whose --version prints it; the SARIF carries it
#   AC-002   the parecer names the version in the details when it is not
#            dev, and is byte-identical to before when it is dev
#   AC-003   the audit record writes tool_version only outside dev
#   AC-004   review.yml (both builds) and providers-smoke.yml resolve the
#            version (exact tag or short SHA) and pass it with --build-arg
#   MUT-001  dropping the dev condition turns AC-002 red
# Exit: 0 pass, 1 behavioral failure, 64 unknown selector, 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C

readonly card='AUR-611'
selector="${1:-all}"
known='all AC-001 AC-002 AC-003 AC-004 MUT-001'
if [[ " $known " != *" $selector "* ]]; then
  printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2
  exit 64
fi

fail() {
  printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2
  exit 1
}
infra() {
  printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2
  exit 79
}

script_dir="${0%/*}"
[[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
for tool in awk sed grep sort; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing_$tool"
done
: "${GOCACHE:=$(mktemp -d)}"
export GOCACHE

work="$(mktemp -d)"
trap 'chmod -R u+w -- "$work" 2>/dev/null || true; rm -rf -- "$work"' EXIT

readonly pkgs=(./internal/version ./internal/render ./internal/i18n ./cmd/aurumcode)

# stage copies the module (whole cmd, internal, pkg) into a fresh root.
stage() {
  local root="$1"
  mkdir -p "$root"
  for item in go.mod go.sum cmd internal pkg; do
    [[ -e "$repo_root/$item" ]] || infra "missing-$item"
    cp -R "$repo_root/$item" "$root/"
  done
  chmod -R u+w -- "$root"
}

# run_test runs the named tests of the packages in root.
run_test() {
  local root="$1" pattern="$2" log="$3"
  (cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "${pkgs[@]}") >"$log" 2>&1
}

base="$work/base"
stage "$base"

# ac id pattern tests...: every named test must pass.
ac() {
  local id="$1" pattern="$2" log="$work/$1.log" name
  shift 2
  if ! run_test "$base" "$pattern" "$log"; then
    tail -n 30 "$log" >&2
    fail "test-failed"
  fi
  for name in "$@"; do
    grep -Eq -- "^--- PASS: $name " "$log" || { tail -n 30 "$log" >&2; fail "$id-missing-pass:$name"; }
  done
  printf '%s/%s/pass\n' "$card" "$id"
}

mutate() {
  local id="$1" file="$2" anchor="$3" expr="$4" pattern="$5" assertion="$6"
  local root="$work/$id" log="$work/$id.log"
  stage "$root"
  grep -Fq -- "$anchor" "$root/$file" || infra "$id-anchor-missing"
  sed -i "$expr" "$root/$file"
  if grep -Fq -- "$anchor" "$root/$file"; then
    infra "$id-not-applied"
  fi
  if run_test "$root" "$pattern" "$log"; then
    fail "$id-survived"
  fi
  if grep -Eq 'build failed|setup failed' "$log"; then
    tail -n 20 "$log" >&2
    infra "$id-did-not-compile"
  fi
  grep -Eq -- "^--- FAIL: $assertion" "$log" || {
    tail -n 20 "$log" >&2
    infra "$id-unexpected-failure"
  }
  rm -rf -- "$root"
  printf '%s/%s/rejected (%s)\n' "$card" "$id" "$assertion"
}

# need <file> <literal> fails unless the file carries the literal.
need() {
  [[ -f "$repo_root/$1" ]] || infra "missing-$1"
  grep -Fq -- "$2" "$repo_root/$1" || fail "$1/lacks:$2"
}

# count <file> <literal> prints how many lines of the file carry the literal.
count() {
  grep -Fc -- "$2" "$repo_root/$1" || true
}

# docker_build <version|-> <out> runs the Dockerfile's own go build line in
# the staged module, with AURUMCODE_VERSION set to version, or left to the
# Dockerfile's ARG default when version is "-".
docker_build() {
  local version="$1" out="$2" line default
  line="$(sed -n 's|^RUN \(CGO_ENABLED=0 go build .*\) -o /aurumcode \./cmd/aurumcode$|\1|p' "$repo_root/Dockerfile")"
  [[ -n "$line" ]] || fail AC-001/dockerfile-build-line-missing
  default="$(sed -n 's|^ARG AURUMCODE_VERSION=\(.*\)$|\1|p' "$repo_root/Dockerfile")"
  [[ "$default" == 'dev' ]] || fail AC-001/dockerfile-default-is-not-dev
  [[ "$version" != '-' ]] || version="$default"
  (cd "$base" && GO_TAGS='' AURUMCODE_VERSION="$version" GOFLAGS=-buildvcs=false \
    bash -c "$line -o \"\$1\" ./cmd/aurumcode" _ "$out") >"$work/build.log" 2>&1 || {
    tail -n 20 "$work/build.log" >&2
    infra AC-001-build-failed
  }
}

ac001() {
  local got
  need Dockerfile 'ARG AURUMCODE_VERSION=dev'
  need Dockerfile '-X main.version=${AURUMCODE_VERSION}'
  docker_build v9.9.9 "$work/aurumcode-stamped"
  got="$("$work/aurumcode-stamped" --version)" || fail AC-001/version-flag-failed
  [[ "$got" == 'aurumcode v9.9.9' ]] || fail "AC-001/stamped-version:$got"
  docker_build - "$work/aurumcode-default"
  got="$("$work/aurumcode-default" --version)" || fail AC-001/version-flag-failed
  [[ "$got" == 'aurumcode dev' ]] || fail "AC-001/default-version:$got"
  ac AC-001 '^TestAUR611(SARIFCarriesStampedVersion|VersionFlagPrintsStampedVersion|StampedVersionIsNotDev|UnstampedVersionIsDev)$' \
    TestAUR611SARIFCarriesStampedVersion TestAUR611VersionFlagPrintsStampedVersion \
    TestAUR611StampedVersionIsNotDev TestAUR611UnstampedVersionIsDev
}
ac002() {
  ac AC-002 '^(TestAUR611DetailsCarryToolVersionWhenNotDev|TestAUR611DetailsOmitToolVersionWhenDev|TestEveryKeyExistsInEveryLocale)$' \
    TestAUR611DetailsCarryToolVersionWhenNotDev TestAUR611DetailsOmitToolVersionWhenDev TestEveryKeyExistsInEveryLocale
}
ac003() {
  ac AC-003 '^TestAUR611Audit(RecordsToolVersionOnlyWhenNotDev|FileCarriesToolVersionOnlyWhenNotDev)$' \
    TestAUR611AuditRecordsToolVersionOnlyWhenNotDev TestAUR611AuditFileCarriesToolVersionOnlyWhenNotDev
}

# resolver_blocks <file> prints the file's marked version resolvers.
resolver_blocks() {
  sed -n '/# AUR-611-VERSION-BEGIN/,/# AUR-611-VERSION-END/p' "$repo_root/$1"
}

# resolve <sha> <exit> <ls-remote output> runs the workflow's resolver with a
# stub git and prints the version it wrote to GITHUB_OUTPUT; it fails when
# the resolver exits non-zero.
resolve() {
  local sha="$1" status="$2" listing="$3" stub="$work/stub"
  mkdir -p "$stub"
  printf '#!/usr/bin/env bash\nprintf "%%s\\n" "$*" >>"$STUB_CALLS"\nprintf "%%b" "$STUB_LISTING"\nexit "$STUB_EXIT"\n' >"$stub/git"
  chmod +x "$stub/git"
  : >"$work/github_output"
  PATH="$stub:$PATH" STUB_CALLS="$work/calls" STUB_LISTING="$listing" STUB_EXIT="$status" \
    TOOL_SHA="$sha" TOOL_DIR=.aurumcode-tool GITHUB_OUTPUT="$work/github_output" \
    bash -c "set -euo pipefail; $resolver" >/dev/null 2>&1 || return 1
  sed -n 's/^version=//p' "$work/github_output"
}

ac004() {
  local file n sha other got
  need Dockerfile 'ARG AURUMCODE_VERSION=dev'
  # A product build without the argument is gone (the gitleaks layer added
  # FROM aurumcode-review, built from stdin, keeps the stamped binary).
  grep -Eq 'docker build --tag [a-z-]+ (\.aurumcode-tool|\.)$' "$repo_root/.github/workflows/review.yml" "$repo_root/.github/workflows/providers-smoke.yml" &&
    fail AC-004/build-without-version
  for file in .github/workflows/review.yml:2 .github/workflows/providers-smoke.yml:1; do
    n="${file##*:}"
    file="${file%:*}"
    [[ "$(count "$file" 'docker build --build-arg AURUMCODE_VERSION="$AURUMCODE_VERSION" --tag')" == "$n" ]] ||
      fail "AC-004/$file/build-arg-count"
    [[ "$(count "$file" 'AURUMCODE_VERSION: ${{ steps.tool_version.outputs.version }}')" == "$n" ]] ||
      fail "AC-004/$file/version-not-from-resolver"
    [[ "$(count "$file" 'id: tool_version')" == "$n" ]] || fail "AC-004/$file/resolver-count"
    [[ "$(count "$file" '# AUR-611-VERSION-BEGIN')" == "$n" ]] || fail "AC-004/$file/marker-count"
  done
  [[ "$(count .github/workflows/review.yml 'docker build --build-arg AURUMCODE_VERSION="$AURUMCODE_VERSION" --tag aurumcode-review .aurumcode-tool')" == 2 ]] ||
    fail AC-004/review-builds-not-the-tool-checkout
  # The three resolvers are one text, so the one run below is all of them.
  resolver="$(resolver_blocks .github/workflows/providers-smoke.yml)"
  [[ -n "$resolver" ]] || fail AC-004/resolver-missing
  [[ "$(resolver_blocks .github/workflows/review.yml)" == "$resolver"$'\n'"$resolver" ]] || fail AC-004/resolvers-differ

  sha=0123456789abcdef0123456789abcdef01234567
  other=fedcba9876543210fedcba9876543210fedcba98
  got="$(resolve "$sha" 0 "1111111111111111111111111111111111111111\trefs/tags/v9.9.9\n$sha\trefs/tags/v9.9.9^{}\n$other\trefs/tags/v9.9.8\n")" ||
    fail AC-004/annotated-tag-failed
  [[ "$got" == v9.9.9 ]] || fail "AC-004/annotated-tag:$got"
  grep -Fxq -- '-C .aurumcode-tool ls-remote --tags origin' "$work/calls" || fail AC-004/lookup-not-in-tool-checkout
  got="$(resolve "$sha" 0 "$sha\trefs/tags/v1.9.0\n$sha\trefs/tags/v1.10.0\n$other\trefs/tags/v2.0.0\n")" || fail AC-004/lightweight-tag-failed
  [[ "$got" == v1.10.0 ]] || fail "AC-004/highest-exact-tag:$got"
  got="$(resolve "$sha" 0 "$other\trefs/tags/v9.9.9\n")" || fail AC-004/untagged-failed
  [[ "$got" == 0123456789ab ]] || fail "AC-004/untagged-short-sha:$got"
  got="$(resolve "$sha" 0 "$sha\trefs/tags/latest\n$sha\trefs/tags/v1.0.0;id\n$sha\trefs/tags/v2.0.0-rc.1\n")" || fail AC-004/unsafe-tag-failed
  [[ "$got" == 0123456789ab ]] || fail "AC-004/unsafe-tag-not-ignored:$got"
  got="$(resolve "$sha" 128 '')" || fail AC-004/lookup-failure-failed-the-job
  [[ "$got" == 0123456789ab ]] || fail "AC-004/lookup-failure-short-sha:$got"
  if resolve main 0 '' >/dev/null; then fail AC-004/malformed-sha-accepted; fi

  need docs/releases.md 'AURUMCODE_VERSION'
  need docs/releases.md 'tool_version'
  printf '%s/AC-004/pass\n' "$card"
}

mut001() {
  mutate MUT-001 cmd/aurumcode/tool_version.go 'if info.IsDev() {' \
    's/if info.IsDev() {/if false {/' '^TestAUR611DetailsOmitToolVersionWhenDev$' 'TestAUR611DetailsOmitToolVersionWhenDev'
}

# One function per selector; all runs every one in order.
if [[ "$selector" == 'all' ]]; then
  for step in ac001 ac002 ac003 ac004 mut001; do
    "$step"
  done
  printf '%s/all/pass\n' "$card"
else
  step="${selector//-/}"
  step="${step,,}"
  "$step"
fi

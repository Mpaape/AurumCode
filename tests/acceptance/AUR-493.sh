#!/usr/bin/env bash
# Run the workflow's guard and assert checkout bindings to GitHub's documented
# callee identity. Real runner semantics must additionally be verified in CI.
set -euo pipefail
selector="${1:-all}"
case "$selector" in all|AC-001|AC-002|AC-003|MUT-001|MUT-002) ;; *) exit 64 ;; esac
repo_root="$(cd -- "${BASH_SOURCE[0]%/*}/../.." && pwd -P)"
repo_root="${AURUM_A493_REPO_ROOT:-$repo_root}"
workflow="$repo_root/.github/workflows/review.yml"
fail() { printf 'AUR-493/%s/%s\n' "$selector" "$1" >&2; exit 1; }
guard() {
  local body
  body="$(sed -n '/# AUR-493-VERIFY-BEGIN/,/# AUR-493-VERIFY-END/p' "$workflow")"
  [[ -n "$body" ]] || fail missing-guard
  TOOL_REPOSITORY="$1" TOOL_SHA="$2" GITHUB_SHA=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
    GITHUB_WORKFLOW_REF=consumer/repo/.github/workflows/pr.yml@refs/heads/main \
    bash -eu -c "$body"
}
ac001() {
  grep -Fxq '          repository: ${{ job.workflow_repository }}' "$workflow" || fail wrong-repository
  grep -Fxq '          ref: ${{ job.workflow_sha }}' "$workflow" || fail wrong-ref
  grep -Fxq '          TOOL_REPOSITORY: ${{ job.workflow_repository }}' "$workflow" || fail wrong-repository-input
  grep -Fxq '          TOOL_SHA: ${{ job.workflow_sha }}' "$workflow" || fail wrong-sha-input
  guard Mpaape/AurumCode bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb || fail valid-version-rejected
  if guard Mpaape/AurumCode ''; then fail missing-version-accepted; fi
  if guard '' bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb; then fail missing-repository-accepted; fi
  if guard Mpaape/AurumCode main; then fail mutable-version-accepted; fi
  if guard '../consumer/repo' bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb; then fail invalid-repository-accepted; fi
}
ac002() {
  local file
  for file in .github/workflows/examples/code-review.yml docs/site/workflow.yml; do
    grep -Fq 'uses: Mpaape/AurumCode/.github/workflows/review.yml@v2' "$repo_root/$file" || fail missing-v2
    if grep -Fq '@main' "$repo_root/$file"; then fail mutable-example; fi
  done
  cmp "$repo_root/.github/workflows/examples/code-review.yml" "$repo_root/docs/site/workflow.yml" || fail divergent-example
}
ac003() {
  grep -Fxq '    uses: ./.github/workflows/review.yml' "$repo_root/.github/workflows/code-review.yml" || fail nonlocal-self-review
  guard Mpaape/AurumCode cccccccccccccccccccccccccccccccccccccccc || fail self-review-rejected
  if grep -Eq 'github\.(workflow_ref|sha)' "$workflow"; then fail caller-fallback; fi
}
mutation() {
  local staged rc=0
  staged="$(mktemp -d)"
  trap 'rm -rf -- "$staged"' RETURN
  mkdir -p "$staged/.github/workflows/examples" "$staged/docs/site"
  cp "$workflow" "$staged/.github/workflows/review.yml"
  cp "$repo_root/.github/workflows/code-review.yml" "$staged/.github/workflows/code-review.yml"
  cp "$repo_root/.github/workflows/examples/code-review.yml" "$staged/.github/workflows/examples/code-review.yml"
  cp "$repo_root/docs/site/workflow.yml" "$staged/docs/site/workflow.yml"
  if [[ "$selector" == MUT-001 ]]; then
    sed -i '/^          ref:/c\          ref: main' "$staged/.github/workflows/review.yml"
    AURUM_A493_REPO_ROOT="$staged" bash "${BASH_SOURCE[0]}" AC-001 || rc=$?
  else
    sed -i 's/@v2/@main/' "$staged/docs/site/workflow.yml"
    AURUM_A493_REPO_ROOT="$staged" bash "${BASH_SOURCE[0]}" AC-002 || rc=$?
  fi
  [[ "$rc" == 1 ]] || fail mutation-survived-or-infrastructure-failed
}
case "$selector" in
  all) ac001; ac002; ac003 ;;
  AC-001) ac001 ;;
  AC-002) ac002 ;;
  AC-003) ac003 ;;
  MUT-*) mutation ;;
esac
printf 'AUR-493/%s/pass\n' "$selector"

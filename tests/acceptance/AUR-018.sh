#!/usr/bin/env bash
set -euo pipefail

f='.board/research/scm-ci.md'
[[ -s "$f" ]] || { printf 'AUR-018/AC-001: SCM/CI baseline absent\n' >&2; exit 1; }
for needle in 'GitHub' 'Gitea' 'GitLab' 'analyzer' 'publisher' 'least privilege'; do
  grep -Fiq "$needle" "$f" || exit 1
done
printf '{"card":"AUR-018","scenario":"AC-001","result":"pass"}\n'

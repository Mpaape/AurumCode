#!/usr/bin/env bash
set -euo pipefail

f='.board/research/scm-ci.md'
[[ -s "$f" ]] || { printf 'AUR-018/AC-001: SCM/CI baseline absent\n' >&2; exit 1; }
for needle in 'GitHub' 'Gitea' 'GitLab' 'analyzer' 'publisher' 'least privilege'; do
  grep -Fiq "$needle" "$f" || { printf 'AUR-018/AC-001: missing term %s\n' "$needle" >&2; exit 1; }
done

std='standards/scm'
[[ -d "$std" ]] || { printf 'AUR-018/AC-001: standards/scm baseline absent\n' >&2; exit 1; }
find "$std" -type f -print0 | xargs -0 grep -Fq . 2>/dev/null || {
  printf 'AUR-018/AC-001: standards/scm has no content\n' >&2
  exit 1
}

# MUT-001 guard (repository boundary): no forge/role may grant write access
# to a credential presented while a hostile head is checked out.
#
# r3 and r4 both tried to make this guard understand a YAML *value* -- first
# by extracting the text after a same-line colon (r3's bypass: a block-style
# value on the following line never reaches that extraction), then by also
# rejecting an empty same-line extraction (r4's bypass: YAML's explicit-key
# syntax, `? allow_write_on_hostile_head` on one line and `: true` on the
# next, never puts the key text and a colon on the same line at all, so it
# never triggers the awk filter in the first place). Each fix closed exactly
# the one syntax form it was shown; YAML has more ways to split a key from
# its value than any such enumeration will anticipate, so this whole
# approach -- reimplementing YAML's grammar one form at a time in awk -- is
# retired here, not patched again.
#
# This guard does not parse YAML and does not ask "what value does this key
# have". It asks a narrower, exactly-decidable question instead: for every
# line anywhere under standards/scm whose trimmed text contains the key name
# "allow_write_on_hostile_head" (case-insensitive, so a case-spelling dodge
# is still caught) and is not itself a whole-line comment (a line whose
# first non-blank character is "#" -- prose that only *discusses* the key,
# such as this file's own header and standards/scm/capabilities.yaml's
# documentation of this rule, is not a YAML key occurrence and must stay
# legal), that line's trimmed text MUST be byte-identical to the one
# canonical line this repository is allowed to contain:
#
#   allow_write_on_hostile_head: false
#
# Any deviation is a typed failure by construction, independent of why it
# deviates: a different value (true, True, TRUE, yes, Yes, YES, on, On, ON,
# 1, or anything else -- this is not a truthy-word denylist, so a spelling
# nobody enumerated still fails), a quoted key or value, different case, a
# flow-map form (`{allow_write_on_hostile_head: true}`), a YAML anchor or
# alias on the key or value (`allow_write_on_hostile_head: &a false`), a
# block-style split (`allow_write_on_hostile_head:` then an indented value
# on the next line -- r3's bypass), or the explicit-key split (`? key` /
# `: value` -- r4's bypass). None of those forms can equal the canonical
# line after trimming surrounding whitespace, so none of them can pass; the
# whole class of "a YAML form this guard's author did not enumerate" is
# closed structurally, by requiring exact equality to the one allowed
# spelling, not by growing a list of forbidden ones.
mapfile -t std_files < <(find "$std" -type f | sort)
canon_line='allow_write_on_hostile_head: false'
canon_key='allow_write_on_hostile_head'
mut001_output="$(awk -v canon="$canon_line" -v key="$canon_key" '
  {
    trimmed = $0
    gsub(/^[ \t]+/, "", trimmed)
    gsub(/[ \t]+$/, "", trimmed)
    if (trimmed ~ /^#/) next
    if (index(tolower(trimmed), key) > 0) {
      total++
      if (trimmed == canon) {
        canon_total++
      } else {
        printf "BAD\t%s:%d: %s\n", FILENAME, FNR, trimmed
      }
    }
  }
  END {
    printf "TOTAL\t%d\t%d\n", total + 0, canon_total + 0
  }
' "${std_files[@]}")"
bad_lines="$(printf '%s\n' "$mut001_output" | grep '^BAD' || true)"
if [[ -n "$bad_lines" ]]; then
  while IFS= read -r bad; do
    [[ -z "$bad" ]] && continue
    printf 'AUR-018/AC-001/MUT-001: non-canonical allow_write_on_hostile_head form -- %s\n' "${bad#BAD$'\t'}" >&2
  done <<< "$bad_lines"
  exit 1
fi
canon_total="$(printf '%s\n' "$mut001_output" | awk -F'\t' '$1 == "TOTAL" { print $3 }')"
if [[ -z "$canon_total" || "$canon_total" -lt 1 ]]; then
  printf 'AUR-018/AC-001/MUT-001: no canonical allow_write_on_hostile_head line found in standards/scm\n' >&2
  exit 1
fi
find "$std" -type f -print0 | xargs -0 grep -Fq 'hostile_head_read_only' 2>/dev/null || {
  printf 'AUR-018/AC-001: standards/scm missing the hostile_head_read_only rule\n' >&2
  exit 1
}

# MUT-002 guard (network-source boundary): every URL cited anywhere in the
# card's citation-bearing artifacts must resolve to the exact host of an
# allowlisted forge-doc site, and the research file's citation rows must
# still carry a version or a date.
#
# r5's guard only considered a URL a "citation" at all if it matched
# `https://[a-z0-9._-]*(github|gitlab|gitea)\.com[^) ]*` -- i.e. only if the
# literal substring "<forge>.com" appeared immediately after the forge name.
# That extraction regex, not the allowlist `case` below it, was the actual
# hole: a host such as raw.githubusercontent.com contains the substring
# "github" but never the substring "github.com" (the character right after
# "github" is "u", from "usercontent", not "."), so the URL never matched
# the extraction pattern in the first place, `cited_urls` came back empty,
# the `while` loop body never ran, and the allowlist `case` was never
# reached -- exit stayed 0. Restricting the extraction to forge-named hosts
# was itself the bypass: any citation host that doesn't spell out
# "github"/"gitlab"/"gitea" as a substring (an unrelated third-party mirror,
# a URL shortener, a raw-content CDN under a different name) was invisible
# to this guard too, before it ever reached a host comparison.
#
# This guard is retired in favor of one that extracts every http(s) URL in
# each citation-bearing file unconditionally (no forge-name prefilter),
# resolves each one to a bare host (stripping scheme, userinfo, path, query,
# fragment, and port), and requires that host to be byte-identical to one of
# the three allowlisted names via a `case` exact match -- not "contains",
# not "starts with", not "ends with". A host confusion trick (a subdomain
# prefix like raw.githubusercontent.com, a lookalike suffix domain like
# docs.github.com.evil.example, userinfo like docs.github.com@evil.example,
# or an unrelated third-party host) cannot pass this check by construction,
# because none of them can equal the canonical host string after parsing.
url_re='https?://[^)<>"'\''[:space:]]+'

aur018_host_of() {
  local url="$1" rest authority host
  rest="${url#*://}"
  authority="${rest%%[/?#]*}"
  authority="${authority##*@}"
  host="${authority%%:*}"
  printf '%s' "$host" | tr '[:upper:]' '[:lower:]'
}

aur018_is_allowlisted_host() {
  case "$1" in
    docs.github.com|docs.gitlab.com|docs.gitea.com) return 0 ;;
    *) return 1 ;;
  esac
}

# Every artifact this card can carry a citation in: the dated research file,
# everything materialized under the machine-readable standard (its per-forge
# `source:` field mirrors the research citations), and the human-readable
# spec, if it cites anything of its own. tests/acceptance/AUR-018.sh itself
# is deliberately excluded: it is code containing this guard's own regex
# fragments, not a citation artifact, and scanning it would flag its own
# pattern literals as malformed hosts.
cited_files=("$f")
while IFS= read -r -d '' stdfile; do
  cited_files+=("$stdfile")
done < <(find "$std" -type f -print0 | sort -z)
[[ -f docs/specs/AUR-018.md ]] && cited_files+=(docs/specs/AUR-018.md)

allowlisted_count=0
for cf in "${cited_files[@]}"; do
  [[ -f "$cf" ]] || continue
  while IFS= read -r grepline; do
    [[ -z "$grepline" ]] && continue
    lineno="${grepline%%:*}"
    url="${grepline#*:}"
    host="$(aur018_host_of "$url")"
    if ! aur018_is_allowlisted_host "$host"; then
      printf 'AUR-018/AC-001/MUT-002: non-allowlisted host cited as a source -- %s:%s: %s (host=%s)\n' "$cf" "$lineno" "$url" "$host" >&2
      exit 1
    fi
    if [[ "$cf" == "$f" ]]; then
      row="$(sed -n "${lineno}p" "$cf")"
      printf '%s' "$row" | grep -Eq '[0-9]{4}-[0-9]{2}-[0-9]{2}|[0-9]+\.[0-9]+' || {
        printf 'AUR-018/AC-001/MUT-002: source line missing a version or date -- %s:%s\n' "$cf" "$lineno" >&2
        exit 1
      }
      allowlisted_count=$((allowlisted_count + 1))
    fi
  # Captured into a variable rather than streamed into the `while` through a
  # live pipe: a short-circuiting reader on the far end can SIGPIPE the
  # producer before it finishes writing (observed under this profile's
  # BusyBox grep, not under GNU grep), which would make this guard silently
  # pass under `pipefail`. Process substitution avoids depending on a second
  # process's read timing altogether, matching the MUT-001 guard's approach.
  done < <(grep -noE "$url_re" "$cf" 2>/dev/null || true)
done
[[ "$allowlisted_count" -ge 3 ]] || {
  printf 'AUR-018/AC-001/MUT-002: fewer than 3 allowlisted forge-doc sources\n' >&2
  exit 1
}

printf '{"card":"AUR-018","scenario":"AC-001","result":"pass"}\n'

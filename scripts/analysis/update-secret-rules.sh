#!/usr/bin/env bash
#
# Refreshes the embedded secret-detection catalog of internal/analysis from
# the public gitleaks default configuration pinned in the scanners lock.
#
# 1. Reads the pinned version and sha256 from .board/bootstrap/locks/scanners.yml
#    (secrets_scanner_version, secrets_rulebase_sha256).
# 2. Downloads config/gitleaks.toml at that tag and refuses it unless its
#    sha256 equals the locked digest.
# 3. Derives internal/analysis/rules/secrets.json from it with python3's
#    stdlib tomllib (no TOML parser enters go.mod) and checks that every
#    derived rule keeps the source id and regex.
#
# The TOML itself is never committed: the repository is public and the base
# allowlists carry credential-shaped example keys. Every allowlist literal
# without regex metacharacters that the rule's own regex matches is written
# as its sha256 (literal_sha256) and matched by exact hash equality; the
# script refuses to finish if a converted literal does not match its rule.
# The derived JSON records the source digest (source.sha256, equal to the
# lock) and secrets.json.sha256 records the JSON's own digest, which the Go
# loader checks. Set AURUMCODE_GITLEAKS_TOML_OUT=<path> to keep a copy of the
# verified TOML for the source-gated Go test.
# Requires network, curl, sha256sum and python3 >= 3.11. Runs on the host,
# never inside the offline sealed container.
set -euo pipefail
export LC_ALL=C

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd "$script_dir/../.." && pwd -P)"
lock="$repo_root/.board/bootstrap/locks/scanners.yml"
out_dir="$repo_root/internal/analysis/rules"

lock_value() { sed -n "s/^$1: //p" "$lock"; }
version="$(lock_value secrets_scanner_version)"
rulebase_id="$(lock_value secrets_rulebase_id)"
want="$(lock_value secrets_rulebase_sha256)"
[[ -n "$version" && -n "$rulebase_id" && "$want" =~ ^sha256:[0-9a-f]{64}$ ]] || { echo "lock fields missing in $lock" >&2; exit 1; }

url="https://raw.githubusercontent.com/gitleaks/gitleaks/${version}/config/gitleaks.toml"
tmp="$(mktemp -d)"
trap 'rm -rf -- "$tmp"' EXIT
curl -fsSL --proto '=https' -o "$tmp/gitleaks.toml" "$url"
got="sha256:$(sha256sum "$tmp/gitleaks.toml" | awk '{print $1}')"
[[ "$got" == "$want" ]] || { echo "digest mismatch: got $got, lock pins $want" >&2; exit 1; }

python3 - "$tmp/gitleaks.toml" "$tmp/secrets.json" "$rulebase_id" "$url" "$want" <<'PY'
import hashlib, json, re, sys, tomllib

src, dst, rulebase_id, url, digest = sys.argv[1:6]
with open(src, "rb") as fh:
    cfg = tomllib.load(fh)

META = set("\\.+*?()|[]{}^$")

def credential_literal(x, rule_regex):
    if rule_regex is None or set(x) & META:
        return False
    try:
        return re.search(rule_regex, x) is not None
    except re.error:
        return False

def allowlist(a, rule_regex=None):
    out = {}
    for key in ("description", "condition", "regexTarget", "paths", "stopwords"):
        if key in a:
            out[key] = a[key]
    plain, hashed = [], []
    for x in a.get("regexes", []):
        if credential_literal(x, rule_regex):
            hashed.append(hashlib.sha256(x.encode()).hexdigest())
        else:
            plain.append(x)
    if plain:
        out["regexes"] = plain
    if hashed:
        out["literal_sha256"] = hashed
    return out

def rule(r):
    out = {"id": r["id"], "description": r["description"]}
    for key in ("regex", "path", "secretGroup", "entropy", "keywords"):
        if key in r:
            out[key] = r[key]
    lists = r.get("allowlists", [])
    if "allowlist" in r:
        lists = [r["allowlist"]] + list(lists)
    if lists:
        out["allowlists"] = [allowlist(a, r.get("regex")) for a in lists]
    return out

doc = {
    "schema": "aurumcode-secret-rules-v1",
    "source": {"rulebase_id": rulebase_id, "url": url, "sha256": digest},
    "allowlist": allowlist(cfg.get("allowlist", {})),
    "rules": [rule(r) for r in cfg["rules"]],
}
for src_rule, got in zip(cfg["rules"], doc["rules"], strict=True):
    if src_rule["id"] != got["id"] or src_rule.get("regex") != got.get("regex"):
        sys.exit("derived rule differs from source: " + src_rule["id"])
with open(dst, "w", encoding="utf-8") as fh:
    json.dump(doc, fh, indent=1, sort_keys=True, ensure_ascii=False)
    fh.write("\n")
PY

if [[ -n "${AURUMCODE_GITLEAKS_TOML_OUT:-}" ]]; then
  install -m 0600 "$tmp/gitleaks.toml" "$AURUMCODE_GITLEAKS_TOML_OUT"
fi
install -m 0644 "$tmp/secrets.json" "$out_dir/secrets.json"
printf 'sha256:%s\n' "$(sha256sum "$out_dir/secrets.json" | awk '{print $1}')" >"$out_dir/secrets.json.sha256"
printf 'source %s\nartifact %s\n' "$want" "$(cat "$out_dir/secrets.json.sha256")"

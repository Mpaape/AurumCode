#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C
[[ "${1:-E2EAUR408}" == E2EAUR408 ]] || { printf 'AUR-408/AC-001/unknown-selector\n' >&2; exit 64; }
script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)"
test -s "$repo_root/.board/schemas/docs-tool-offline-profile.schema.json"
test -s "$repo_root/.board/oci/profiles/docs-tool-offline-v1.json"
test -s "$repo_root/.board/locks/oci/docs-tool-offline-v1.lock.json"
grep -Fqx '    "key": "docs-tool-offline-v1",' "$repo_root/.board/oci/profiles/registry.v1.json"
grep -Fqx '    "schema": ".board/schemas/docs-tool-offline-profile.schema.json",' "$repo_root/.board/oci/profiles/registry.v1.json"
grep -Fqx '    "lock": ".board/locks/oci/docs-tool-offline-v1.lock.json",' "$repo_root/.board/oci/profiles/registry.v1.json"
# The Go-capable image is admitted only by immutable digest.
grep -Fqx '"image": "aurum-bootstrap-go-bash@sha256:503ac356fa6bca4bad56fade87b5479ff371bd446ffb9c7db91f211323c7c73e"' "$repo_root/.board/locks/oci/docs-tool-offline-v1.lock.json"
# Generator, renderer and plugin set are admitted only content-addressed, and the
# generator is cgo-free, so no native site tool (pandoc, mkdocs, jekyll, hugo,
# asciidoctor) and no native library is ever required by the plan.
grep -Fqx '"generator": "digest-pinned",' "$repo_root/.board/oci/profiles/docs-tool-offline-v1.json"
grep -Fqx '"generator_cgo": false,' "$repo_root/.board/oci/profiles/docs-tool-offline-v1.json"
grep -Fqx '"renderer": "digest-pinned",' "$repo_root/.board/oci/profiles/docs-tool-offline-v1.json"
grep -Fqx '"plugin_set": "digest-pinned",' "$repo_root/.board/oci/profiles/docs-tool-offline-v1.json"
# Fixtures are local and the rendered output is ephemeral, inside the bounded tmpfs and
# never in the checkout.
grep -Fqx '"fixture_source": "local",' "$repo_root/.board/oci/profiles/docs-tool-offline-v1.json"
grep -Fqx '"fixture_root": "/tmp/aurum-docs-fixtures",' "$repo_root/.board/oci/profiles/docs-tool-offline-v1.json"
grep -Fqx '"generator_root": "/tmp/aurum-docs-tool",' "$repo_root/.board/oci/profiles/docs-tool-offline-v1.json"
grep -Fqx '"output_mode": "ephemeral",' "$repo_root/.board/oci/profiles/docs-tool-offline-v1.json"
grep -Fqx '"output_root": "/tmp/aurum-docs-output",' "$repo_root/.board/oci/profiles/docs-tool-offline-v1.json"
# A native site tool, a dynamic plugin, a remote fetch, snippet execution, host output,
# the host filesystem and a subprocess are denied in the published bytes, and so is every
# host-reaching container capability.
for denied in native_site_tool dynamic_plugin remote_fetch snippet_execution host_output host_filesystem subprocess; do
  grep -Fqx "\"$denied\": \"denied\"," "$repo_root/.board/oci/profiles/docs-tool-offline-v1.json"
done
for none_key in network mounts devices sockets cap_add; do
  grep -Fqx "\"$none_key\": \"none\"," "$repo_root/.board/oci/profiles/docs-tool-offline-v1.json"
done
grep -Fqx '"privileged": false,' "$repo_root/.board/oci/profiles/docs-tool-offline-v1.json"
grep -Fqx '"user": "65534:65534",' "$repo_root/.board/oci/profiles/docs-tool-offline-v1.json"
# The snippet allowlist is closed and published verbatim.
grep -Fqx '"snippet_languages": ["bash", "go", "json"],' "$repo_root/.board/oci/profiles/docs-tool-offline-v1.json"
# Corpus, output and wall-clock bounds are present and non-zero in the published bytes.
grep -Fqx '"tmpfs_mb": 128,' "$repo_root/.board/oci/profiles/docs-tool-offline-v1.json"
grep -Fqx '"max_documents": 64,' "$repo_root/.board/oci/profiles/docs-tool-offline-v1.json"
grep -Fqx '"max_document_bytes": 1048576,' "$repo_root/.board/oci/profiles/docs-tool-offline-v1.json"
grep -Fqx '"max_output_bytes": 33554432,' "$repo_root/.board/oci/profiles/docs-tool-offline-v1.json"
grep -Fqx '"timeout_seconds": 60,' "$repo_root/.board/oci/profiles/docs-tool-offline-v1.json"
grep -Fqx '"render_timeout_seconds": 5,' "$repo_root/.board/oci/profiles/docs-tool-offline-v1.json"
! grep -Fq 'latest' "$repo_root/.board/locks/oci/docs-tool-offline-v1.lock.json"
printf 'AUR-408/AC-001/E2EAUR408/ok\n'

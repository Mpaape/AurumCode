#!/usr/bin/env bash
set -euo pipefail
unset AURUMCODE_MODULE_ONLY
case "${1:-all}" in all) ;; *) exit 64 ;; esac
cd "$(dirname "$0")/../../cmd/aurumcode"
go test -buildvcs=false -count=20 -v -run '^(TestFetchPRRange|TestWorkflowsFetchOnlyThePullRequestHistory|TestActionManifest)' fetch_pr_range_test.go fetch_pr_range_workflow_test.go action_manifest_test.go

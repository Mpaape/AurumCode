#!/usr/bin/env bash
set -euo pipefail

# A first run needs Docker only. The fixture exercises the real review binary
# without a network call, while the image is the same one used by the Action.

image="aurumcode-demo:local"
docker build --tag "$image" .
docker run --rm \
  -v "$PWD:/workspace" \
  -w /workspace \
  -e AURUMCODE_LLM_FIXTURE=/workspace/tests/fixtures/review/known-problem-response.json \
  "$image" review --base HEAD~1

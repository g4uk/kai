#!/bin/bash
# Run an arbitrary command against project code INSIDE the sandbox.
# Usage: harness/docker/exec.sh <project-dir> <command...>
set -eu
DIR=$(cd "$1" && pwd); shift
IMG="${HARNESS_IMAGE:-harness-runner:latest}"
docker run --rm -v "$DIR":/work -w /work \
  --memory=4g --cpus=2 --pids-limit=512 \
  --cap-drop=ALL --security-opt no-new-privileges \
  "$IMG" bash -c "$*"

#!/bin/bash
# Build the Yocto build container image with the calling user's UID/GID.
#
#   scripts/container-build.sh [extra docker build args...]
#
# IMAGE overrides the image tag (default: espressobin-yocto:wrynose).
set -euo pipefail

TOP=$(cd "$(dirname "$0")/.." && pwd)
IMAGE=${IMAGE:-espressobin-yocto:wrynose}

exec docker build \
    --build-arg USER_UID="$(id -u)" \
    --build-arg USER_GID="$(id -g)" \
    -t "$IMAGE" \
    "$@" \
    "$TOP"

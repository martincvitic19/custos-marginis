#!/bin/bash
# Run the build container with the project directory bind-mounted at the same
# absolute path as on the host (so paths in build/conf stay valid on both sides).
#
#   scripts/container-run.sh                  # interactive shell
#   scripts/container-run.sh <cmd> [args...]  # run a command
#
# Environment:
#   IMAGE        image tag (default: espressobin-yocto:wrynose)
#   NET_HOST=1   use --network host (netboot servers)
#   SERIAL=dev   pass a serial device into the container (e.g. /dev/ttyUSB0)
#   AS_ROOT=1    run as container root (only for tftpd binding port 69)
set -euo pipefail

TOP=$(cd "$(dirname "$0")/.." && pwd)
IMAGE=${IMAGE:-espressobin-yocto:wrynose}

args=(--rm --init
      -v "$TOP:$TOP"
      -w "$TOP"
      --hostname espressobin-build)

[ -t 0 ] && [ -t 1 ] && args+=(-it)
[ "${NET_HOST:-0}" = 1 ] && args+=(--network host)
[ -n "${SERIAL:-}" ] && args+=(--device "$SERIAL" --group-add dialout)
[ "${AS_ROOT:-0}" = 1 ] && args+=(--user 0:0)

exec docker run "${args[@]}" "$IMAGE" "$@"

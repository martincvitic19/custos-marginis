#!/bin/sh
# S17: Stolen box or SD card (needs TPM). See lib.sh.
. "$(dirname "$0")/lib.sh"
scenario S17 "Stolen box or SD card (needs TPM)" live <<'PROBE'
[ -e /dev/tpm0 ] || exit 77
awk '$2 == "/data" && $1 ~ /mapper|dm-/ {f=1} END {exit !f}' /proc/mounts || exit 1
PROBE

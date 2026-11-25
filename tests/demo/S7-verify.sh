#!/bin/sh
# S7: Modified root file system. See lib.sh.
. "$(dirname "$0")/lib.sh"
scenario S7 "Modified root file system" live <<'PROBE'
grep -qE '^[^ ]+ / [^ ]+ ro[, ]' /proc/mounts || exit 1
/usr/lib/custos/custos-verity-check 2>&1 | grep -q matches || exit 1
PROBE

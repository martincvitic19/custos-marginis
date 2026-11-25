#!/bin/sh
# S10: Curfew bypass. See lib.sh.
. "$(dirname "$0")/lib.sh"
scenario S10 "Curfew bypass" config <<'PROBE'
nft list table inet custos | grep '@curfew_block' | grep -q drop || exit 1
PROBE

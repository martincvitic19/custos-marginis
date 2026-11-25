#!/bin/sh
# S9: Unknown device gets adult rights. See lib.sh.
. "$(dirname "$0")/lib.sh"
scenario S9 "Unknown device gets adult rights" config <<'PROBE'
nft list table inet custos | grep '!= @dev_known' | grep -q 'jump child_egress' || exit 1
PROBE

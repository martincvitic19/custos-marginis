#!/bin/sh
# S12: VPN tunnel from a child device. See lib.sh.
. "$(dirname "$0")/lib.sh"
scenario S12 "VPN tunnel from a child device" config <<'PROBE'
nft list chain inet custos child_egress | grep -q drop || exit 1
systemctl is-active --quiet custos-watch || exit 1
PROBE

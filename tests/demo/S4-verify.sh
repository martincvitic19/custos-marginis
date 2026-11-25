#!/bin/sh
# S4: Lateral movement between VLANs. See lib.sh.
. "$(dirname "$0")/lib.sh"
scenario S4 "Lateral movement between VLANs" config <<'PROBE'
c=$(nft list chain inet custos forward) || exit 1
echo "$c" | grep -q 'policy drop' || exit 1
# An accept may sit in a chain that forward jumps to: search the whole table.
t=$(nft list table inet custos) || exit 1
[ -n "$t" ] || exit 1
echo "$t" | grep 'accept' | grep -E 'iifname.*br-lan\.(10|30)' | grep -q 'oifname.*br-lan\.20' && exit 1
exit 0
PROBE

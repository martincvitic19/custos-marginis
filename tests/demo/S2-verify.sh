#!/bin/sh
# S2: Management from child, guest or WAN. See lib.sh.
. "$(dirname "$0")/lib.sh"
scenario S2 "Management from child, guest or WAN" config <<'PROBE'
c=$(nft list chain inet custos input) || exit 1
echo "$c" | grep -q 'policy drop' || exit 1
echo "$c" | grep 'management' | grep -q 'br-lan.20' || exit 1
echo "$c" | grep 'management' | grep -qE 'br-lan\.(10|30)|"wan"' && exit 1
exit 0
PROBE

#!/bin/sh
# S11: DNS filter bypass (DNS, DoT, DoH). See lib.sh.
. "$(dirname "$0")/lib.sh"
scenario S11 "DNS filter bypass (DNS, DoT, DoH)" config <<'PROBE'
nft list chain inet custos dstnat | grep -q 'th dport 53 .*redirect to :53' || exit 1
g=$(nft list chain inet custos dns_guard)
echo "$g" | grep -q '853.*reject' || exit 1
echo "$g" | grep -q '@doh_v4 tcp dport 443 .*reject' || exit 1
PROBE

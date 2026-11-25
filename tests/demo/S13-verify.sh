#!/bin/sh
# S13: WAN flood and spoofed sources. See lib.sh.
. "$(dirname "$0")/lib.sh"
scenario S13 "WAN flood and spoofed sources" config <<'PROBE'
ip -d link show eth0 | grep -q 'prog/xdp' && exit 0
ip -d link show wan | grep -q 'prog/xdp' || exit 1
PROBE

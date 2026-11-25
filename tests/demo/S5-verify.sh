#!/bin/sh
# S5: Forged update bundle. See lib.sh.
. "$(dirname "$0")/lib.sh"
scenario S5 "Forged update bundle" config <<'PROBE'
c=$(cat /etc/rauc/system.conf /usr/lib/rauc/system.conf 2>/dev/null)
echo "$c" | grep -q '^\[keyring\]' || exit 1
echo "$c" | grep -q 'check-purpose=codesign' || exit 1
PROBE

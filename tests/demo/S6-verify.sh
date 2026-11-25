#!/bin/sh
# S6: Broken update, no way back. See lib.sh.
. "$(dirname "$0")/lib.sh"
scenario S6 "Broken update, no way back" live <<'PROBE'
systemctl is-enabled --quiet custos-health.timer || exit 1
[ "$(rauc status 2>/dev/null | grep -c 'boot status:.*good')" -ge 2 ] || exit 1
PROBE

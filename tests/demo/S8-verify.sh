#!/bin/sh
# S8: Malicious kernel module. See lib.sh.
. "$(dirname "$0")/lib.sh"
scenario S8 "Malicious kernel module" live <<'PROBE'
[ "$(cat /sys/module/module/parameters/sig_enforce)" = Y ] || exit 1
grep -qE '\[(integrity|confidentiality)\]' /sys/kernel/security/lockdown || exit 1
PROBE

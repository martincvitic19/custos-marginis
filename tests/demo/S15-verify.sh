#!/bin/sh
# S15: Silent change of configuration or keys. See lib.sh.
. "$(dirname "$0")/lib.sh"
scenario S15 "Silent change of configuration or keys" live <<'PROBE'
systemctl is-active --quiet auditd || exit 1
auditctl -l | grep -q custos_wg_keys || exit 1
systemctl is-active --quiet custos-aide.timer || exit 1
test -s /data/aide/aide.db || exit 1
PROBE

#!/bin/sh
# S3: Compromised custosd reads WireGuard keys. See lib.sh.
. "$(dirname "$0")/lib.sh"
scenario S3 "Compromised custosd reads WireGuard keys" config <<'PROBE'
[ "$(getenforce)" = Enforcing ] || exit 1
command -v compute_av >/dev/null || exit 77
a=$(compute_av system_u:system_r:custosd_t:s0 system_u:object_r:custos_wg_key_t:s0 file) || exit 1
echo "$a" | grep -q '^allowed=' || exit 1
# Positive control: the same query must show that networkd may read the keys.
compute_av system_u:system_r:systemd_networkd_t:s0 system_u:object_r:custos_wg_key_t:s0 file |
    grep '^allowed=' | grep -qw read || exit 1
echo "$a" | sed -n 's/^allowed= *//p' | grep -qw read && exit 1
exit 0
PROBE

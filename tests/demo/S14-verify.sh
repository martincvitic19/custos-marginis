#!/bin/sh
# S14: ARP spoofing in the LAN. See lib.sh.
. "$(dirname "$0")/lib.sh"
scenario S14 "ARP spoofing in the LAN" live <<'PROBE'
for v in 10 20 30; do systemctl is-active --quiet custos-arpwatch@$v || exit 1; done
PROBE

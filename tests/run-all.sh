#!/bin/sh
# Run every acceptance test from a PC on lan1 (admin VLAN 20):
#   tests/run-all.sh                    # board tests + host-admin tests
#   CUSTOS_PASSWORD=... tests/run-all.sh   # also the authenticated API part
# Copies the scripts to the board (/tmp/custos-tests) and runs their device
# modes over SSH, then the host side from this PC. Prints a summary table.
# Host-user tests (PC on lan0) must be run separately: they need the cable
# in another port.
B=${BOARD:-192.168.20.1}
KEY=${SSH_KEY:-keys/espressobin_ed25519}
SSH="ssh -i $KEY -o BatchMode=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR root@$B"
cd "$(dirname "$0")/.." || exit 1
$SSH 'mkdir -p /tmp/custos-tests' && scp -O -q -i "$KEY" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
    tests/*.sh root@$B:/tmp/custos-tests/ || { echo "cannot reach the board at $B" >&2; exit 1; }

results=""
failed=0
run() { # run <label> <command...>
    l=$1; shift
    printf '\n################ %s ################\n' "$l"
    "$@"; r=$?
    [ $r -eq 0 ] || failed=$((failed+1))
    results="$results$(printf '%-28s %s' "$l" "$([ $r -eq 0 ] && echo PASS || echo "FAIL ($r)")")
"
}
run "A2 router (board)"     $SSH sh /tmp/custos-tests/A2-router.sh device
run "S11 DNS (board)"       $SSH sh /tmp/custos-tests/S11-dns.sh device
run "A4 apply (board)"      $SSH sh /tmp/custos-tests/A4-apply.sh
run "A5 API (board)"        $SSH sh /tmp/custos-tests/A5-api.sh device
run "A7 VPN (board)"        $SSH sh /tmp/custos-tests/A7-vpn.sh
run "A9 bypass (board)"     $SSH sh /tmp/custos-tests/A9-bypass.sh
run "A10 integrity (board)" $SSH sh /tmp/custos-tests/A10-integrity.sh
run "A11 SELinux (board)"   $SSH sh /tmp/custos-tests/A11-selinux.sh
run "A12 IDS/XDP (board)"   $SSH sh /tmp/custos-tests/A12.sh
if $SSH 'ls /sys/class/ieee80211/ 2>/dev/null | grep -q phy'; then
    run "W-WIFI (board)"    $SSH sh /tmp/custos-tests/W-WIFI.sh
fi
run "A2 host-admin (PC)"    tests/A2-router.sh host-admin
run "S11 host (PC)"         tests/S11-dns.sh host
run "A5 host-admin (PC)"    tests/A5-api.sh host-admin
run "A8 UI (PC)"            tests/A8-ui.sh

printf '\n################ Summary ################\n%s' "$results"
echo "A6 curfew changes custos.json for a few minutes: run it by hand:"
echo "  $SSH sh /tmp/custos-tests/A6-curfew.sh '<device name>'"
[ $failed -eq 0 ]

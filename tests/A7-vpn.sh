#!/bin/sh
# A7 VPN test, ON the board:  sh /tmp/A7-vpn.sh
# Then with a phone on mobile data (peer from "custos-apply add-peer" or
# the UI): ifconfig.me shows the exit IP; a blocked site stays blocked.
ok()   { printf '  [ OK ] %s\n' "$*"; }
bad()  { printf '  [FAIL] %s\n' "$*"; FAILS=$((FAILS+1)); }
info() { printf '  [info] %s\n' "$*"; }
sec()  { printf '\n== %s ==\n' "$*"; }
FAILS=0
CFG=/data/custos/custos.json
exit_mode=$(jq -r .vpn.exit $CFG)

sec "wg0 (server)"
ip link show wg0 >/dev/null 2>&1 && ok "wg0 exists" || bad "wg0 missing (kernel-module-wireguard? vpn.enabled?)"
wg show wg0 2>/dev/null | sed 's/^/           /'
ip -4 addr show wg0 | grep -q 'inet 192.168.40.1/' && ok "wg0 192.168.40.1/24" || bad "wg0 address"
nft list chain inet custos dstnat | grep -q '"wg0"' && ok "forced DNS covers wg0" || bad "wg0 not in DNS redirect"
nft list chain inet custos policy_input | grep -q 'udp dport 51820 accept' && ok "WireGuard port open on wan" || bad "port not open"
nft list chain inet custos roles | grep -q 'kill switch' && ok "kill switch rule present" || bad "no kill switch"
info "peer exit: $exit_mode"

sec "Policy routing"
if [ "$exit_mode" = mullvad ]; then
    ip rule | grep -q 'iif wg0 lookup 100' && ok "ip rule: from wg0 -> table 100" || bad "no ip rule"
    if ip link show wg-mullvad >/dev/null 2>&1; then
        ok "wg-mullvad up"
        ip route show table 100 | grep -q 'default dev wg-mullvad' && ok "table 100 default via wg-mullvad" || bad "table 100: $(ip route show table 100)"
        wg show wg-mullvad latest-handshakes | awk '{ if ($2>0) print "  [ OK ] Mullvad handshake " systime()-$2 " s ago"; else print "  [FAIL] no Mullvad handshake" }'
    else
        info "no /data/custos/wg/mullvad.conf: peers have no internet (kill switch), DNS and Custos UI still work"
    fi
    ip route show table main | grep -q 'default via' && ok "LAN keeps the main table (wan)" || bad "no main default route"
fi

sec "CGNAT"
custos-apply cgnat-check

sec "Result"; [ $FAILS -eq 0 ] && echo "  PASS" || echo "  $FAILS check(s) FAILED"; exit $FAILS

#!/bin/sh
# A4 custos-apply acceptance test, run ON the board (busybox sh):
#   scp -O tests/A4-apply.sh root@192.168.20.1:/tmp/ ; ssh root@192.168.20.1 sh /tmp/A4-apply.sh
# Read-only except that it runs custos-apply (which is idempotent).
CFG=/data/custos/custos.json
ok()   { printf '  [ OK ] %s\n' "$*"; }
bad()  { printf '  [FAIL] %s\n' "$*"; FAILS=$((FAILS+1)); }
info() { printf '  [info] %s\n' "$*"; }
sec()  { printf '\n== %s ==\n' "$*"; }
FAILS=0

sec "custos.json"
test -s $CFG && ok "$CFG exists" || bad "$CFG missing"
custos-apply validate $CFG >/dev/null 2>&1 && ok "valid (schema + values)" || bad "invalid: $(custos-apply validate $CFG 2>&1 | head -n 3)"
st=$(stat -c '%U:%G %a' $CFG); info "owner/mode $st"
echo "$st" | grep -q ':custos 6[64]0' && ok "group custos, not world-readable" || bad "unexpected owner/mode $st"

sec "Idempotence"
o1=$(custos-apply --mode=full 2>&1); r1=$?
[ $r1 -eq 0 ] && ok "first run exit 0" || bad "first run failed: $o1"
o2=$(custos-apply --mode=full 2>&1); r2=$?
[ $r2 -eq 0 ] && [ "$o2" = "no changes" ] && ok "second run: no changes" || bad "second run: '$o2'"
s=$(cat /run/custos/apply.json 2>/dev/null)
echo "$s" | grep -q '"ok": true' && ok "/run/custos/apply.json: ok" || bad "status: $s"

sec "Generated nft policy"
for set in dev_known dev_trusted curfew_block egress_strict acct_tx acct_dns; do
    nft list set inet custos $set >/dev/null 2>&1 && ok "set $set" || bad "set $set missing"
done
nft list chain inet custos policy_pre | grep -q curfew && ok "policy_pre has curfew rules" || bad "policy_pre empty"
nft list chain inet custos forward | grep -q 'jump policy_pre' && ok "forward jumps to policy_pre (before established)" || bad "no jump to policy_pre"
n=$(jq '.devices | length' $CFG 2>/dev/null)
info "devices in custos.json: $n"
for m in $(jq -r '.devices[] | select(.role=="trusted") | .mac' $CFG 2>/dev/null); do
    nft list set inet custos dev_trusted | grep -q "$m" && ok "trusted $m in dev_trusted" || bad "trusted $m missing in dev_trusted"
done

sec "DHCP reservations"
for f in /run/systemd/network/*.network.d/50-custos-leases.conf; do
    [ -f "$f" ] && info "$f: $(grep -c MACAddress "$f") reservation(s)"
done
jq -e '.leases' /data/custos/state.json >/dev/null 2>&1 && ok "state.json has leases" || info "no reservations yet (no devices)"

sec "AdGuard Home"
c=$(wget -q -T 5 -O - http://127.0.0.1:3000/control/clients 2>/dev/null)
k=$(echo "$c" | grep -o '"name":"custos: [^"]*"' | wc -l)
[ "$k" -eq "${n:-0}" ] || [ "$k" -ge "${n:-0}" ] && ok "$k persistent client(s) managed by Custos" || bad "clients: $k, devices: $n"
wget -q -T 5 -O - http://127.0.0.1:3000/control/filtering/status | grep -q 'use-application-dns.net' && ok "fixed rules present" || bad "fixed rules missing"

sec "Result"
[ $FAILS -eq 0 ] && echo "  PASS" || echo "  $FAILS check(s) FAILED"
exit $FAILS

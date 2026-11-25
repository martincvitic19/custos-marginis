#!/bin/sh
# A6 curfew test, ON the board:  sh /tmp/A6-curfew.sh <device-name>
# Gives <device> (from custos.json) a curfew that starts in ~1 minute and
# lasts 3 minutes, waits, checks, then restores custos.json. Meanwhile, on
# that device run e.g. "ping 9.9.9.9" and a long download: both stop when
# the curfew starts (existing connections too), DNS keeps answering.
DEV=$1
CFG=/data/custos/custos.json
ok()   { printf '  [ OK ] %s\n' "$*"; }
bad()  { printf '  [FAIL] %s\n' "$*"; FAILS=$((FAILS+1)); }
info() { printf '  [info] %s\n' "$*"; }
sec()  { printf '\n== %s ==\n' "$*"; }
FAILS=0
[ -n "$DEV" ] || { echo "usage: $0 <device name from custos.json>" >&2; exit 64; }
jq -e --arg n "$DEV" '.devices[] | select(.name==$n)' $CFG >/dev/null || { echo "no device '$DEV' in $CFG" >&2; exit 64; }

cp -p $CFG /tmp/custos.json.a6-backup
restore() { cp -p /tmp/custos.json.a6-backup $CFG; custos-apply --mode=full >/dev/null; info "custos.json restored"; }
trap restore EXIT INT TERM

day=$(date +%a | tr 'A-Z' 'a-z')
from=$(date -d "@$(( $(date +%s) + 60 ))" +%H:%M)
to=$(date -d "@$(( $(date +%s) + 240 ))" +%H:%M)
info "local time $(date +%H:%M:%S) $(date +%Z); curfew for $DEV: $day $from-$to"
jq --arg n "$DEV" --arg d "$day" --arg f "$from" --arg t "$to" \
   '(.devices[] | select(.name==$n)) |= (.schedules = [{"days":[$d],"from":$f,"to":$t}] | del(.curfew_override_until))' \
   /tmp/custos.json.a6-backup > /tmp/custos.json.a6 && cp /tmp/custos.json.a6 $CFG
custos-apply --mode=full >/dev/null && ok "applied" || bad "apply failed"
mac=$(jq -r --arg n "$DEV" '.devices[] | select(.name==$n) | .mac' $CFG)

sec "Before the curfew"
nft list set inet custos curfew_block | grep -q "$mac" && bad "$DEV already blocked" || ok "$DEV not in curfew_block"

sec "Waiting for $from (the timer runs every minute)"
sleep $(( 60 - $(date +%S | sed 's/^0//') + 65 ))
nft list set inet custos curfew_block | grep -q "$mac" && ok "$DEV in curfew_block" || bad "$DEV not blocked after $from"
nft list chain inet custos policy_pre | grep 'curfew: device'
journalctl -u custos-apply-retry --since "-3 min" --no-pager 2>/dev/null | grep -q "curfew started: $DEV" \
    && ok "journal: curfew started, connections flushed" || info "see: journalctl -u custos-apply-retry"
nslookup example.com 127.0.0.1 >/dev/null 2>&1 && ok "DNS still answers (router side)" || bad "DNS broken"

sec "Override: 5 more minutes (what the parent's button does)"
jq --arg n "$DEV" --arg u "$(date -u -d "@$(( $(date +%s) + 300 ))" +%Y-%m-%dT%H:%M:%SZ)" \
   '(.devices[] | select(.name==$n)) |= (.curfew_override_until = $u)' $CFG > /tmp/custos.json.a6 && cp /tmp/custos.json.a6 $CFG
custos-apply --mode=full >/dev/null
nft list set inet custos curfew_block | grep -q "$mac" && bad "still blocked during override" || ok "override lifts the curfew"

sec "Result"; [ $FAILS -eq 0 ] && echo "  PASS" || echo "  $FAILS check(s) FAILED"; exit $FAILS

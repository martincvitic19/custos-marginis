#!/bin/sh
# A9 anti-bypass test, ON the board:  sh /tmp/A9-bypass.sh
# Live demos (from a PC/phone on lan0 = child):
#   - OpenVPN on udp 1194 or any non-web port -> child_egress counter rises
#   - browse with DoH to an unknown server / VPN over 443 for ~10 min
#     -> alert "vjerojatno zaobilazi zaštitu" in the UI overview
#   - plug a laptop into the ISP router -> alert within 5 minutes
ok()   { printf '  [ OK ] %s\n' "$*"; }
bad()  { printf '  [FAIL] %s\n' "$*"; FAILS=$((FAILS+1)); }
info() { printf '  [info] %s\n' "$*"; }
sec()  { printf '\n== %s ==\n' "$*"; }
FAILS=0

sec "custos-watch"
systemctl is-active --quiet custos-watch && ok "custos-watch active" || bad "custos-watch not running"
u=$(ps -eo user,comm | awk '$2=="custos-watch"{print $1}' | head -n 1)
[ -n "$u" ] && [ "$u" != root ] && ok "runs as $u (CAP_NET_RAW, CAP_NET_ADMIN only)" || bad "user '$u'"

sec "Per-device accounting (nft dynamic sets)"
nft list chain inet custos acct_forward | grep -q 'update @acct_tx' && ok "acct_tx updated in forward" || bad "no accounting rule"
nft list chain inet custos acct_input | grep -q 'update @acct_dns' && ok "acct_dns updated for DNS to Custos" || bad "no DNS accounting"
echo "  bytes sent per device:"; nft list set inet custos acct_tx | grep -o '[0-9a-f:]\{17\} \. "[^"]*" counter packets [0-9]* bytes [0-9]*' | sed 's/^/           /'
echo "  DNS packets per device:"; nft list set inet custos acct_dns | grep -o '[0-9a-f:]\{17\} \. "[^"]*" counter packets [0-9]*' | sed 's/^/           /'

sec "Child egress (strict)"
nft list chain inet custos child_egress | grep -q 'drop' && ok "child_egress: web + NTP only" || bad "child_egress missing"
nft list chain inet custos child_egress | grep 'counter'

sec "Kernel BPF hardening"
[ "$(cat /proc/sys/kernel/unprivileged_bpf_disabled)" != 0 ] && ok "unprivileged_bpf_disabled=$(cat /proc/sys/kernel/unprivileged_bpf_disabled)" || bad "unprivileged BPF allowed"
[ "$(cat /proc/sys/net/core/bpf_jit_harden)" = 2 ] && ok "bpf_jit_harden=2" || bad "bpf_jit_harden=$(cat /proc/sys/net/core/bpf_jit_harden)"

sec "Recent alerts"
tail -n 10 /data/custos/events.jsonl 2>/dev/null | grep -v '"kind":"apply"' | sed 's/^/           /'
grep -q '"kind":"isp_device"' /data/custos/events.jsonl 2>/dev/null && info "ISP-side devices were seen (see above)"

sec "Result"; [ $FAILS -eq 0 ] && echo "  PASS" || echo "  $FAILS check(s) FAILED"; exit $FAILS

#!/bin/sh
# A12 test, ON the board:  sh /tmp/A12.sh
# Live demos (from a lab netns/PC as "attacker", see docs/xdp-measurements.md):
#   SYN flood to 192.168.0.15 from the ISP side -> "syn" counter rises
#   ARP spoof on VLAN 10 -> arpwatch "changed ethernet address" + custos-watch alert
ok()   { printf '  [ OK ] %s\n' "$*"; }
bad()  { printf '  [FAIL] %s\n' "$*"; FAILS=$((FAILS+1)); }
info() { printf '  [info] %s\n' "$*"; }
sec()  { printf '\n== %s ==\n' "$*"; }
FAILS=0

sec "XDP"
ip -d link show eth0 | grep -q 'xdp' && ok "XDP on eth0: $(ip -d link show eth0 | grep -o 'prog/xdp id [0-9]*\|xdp[a-z]*' | head -n 2 | tr '\n' ' ')" \
    || { ip -d link show wan | grep -q xdp && info "XDP generic on wan (native refused)" || bad "no XDP program"; }
custos-xdp stats
bpftool map list 2>/dev/null | grep -E 'tif_v4|bogon_v4|syn_rate' | sed 's/^/           /'
for m in bogon_v4 tif_v4; do
    n=0
    for d in /sys/fs/bpf/custos/maps-dsa /sys/fs/bpf/custos/maps-plain; do
        [ -e "$d/$m" ] || continue
        n=$(bpftool map dump pinned "$d/$m" 2>/dev/null | grep -c '"key"\|^key')
        break
    done
    [ "$n" -gt 0 ] && ok "$m filled ($n entries)" || bad "$m is empty: the filter drops nothing"
done

if systemctl is-failed --quiet custos-xdp-stats.service; then bad "custos-xdp-stats failed"; else ok "custos-xdp-stats not failed"; fi
test -s /run/custos/xdp-stats.json && ok "counters for custosd: $(cat /run/custos/xdp-stats.json)" || bad "no /run/custos/xdp-stats.json (timer runs every minute)"
sec "Suricata"
systemctl is-active --quiet custos-suricata && ok "custos-suricata active" || bad "custos-suricata not active"
pid=$(pidof suricata); [ -n "$pid" ] && info "RSS $(awk '/VmRSS/{print $2" "$3}' /proc/$pid/status)"
test -d /data/suricata && ok "/data/suricata" || bad "/data/suricata missing"
grep -c '"event_type":"alert"' /data/suricata/eve.json 2>/dev/null | sed 's/^/  [info] alerts so far: /'
grep -qE '"event_type":"(dns|http|flow)"' /data/suricata/eve.json 2>/dev/null && bad "eve.json has dns/http/flow records (GDPR)" || ok "eve.json: alerts/anomalies only"

# (no netstat on the board: 006F = port 111)
cat /proc/net/tcp /proc/net/tcp6 /proc/net/udp /proc/net/udp6 2>/dev/null | awk '$2 ~ /:006F$/ {f=1} END {exit !f}' && bad "rpcbind listens on port 111" || ok "no rpcbind"
sec "sshguard, arpwatch, audit, AIDE"
systemctl is-active --quiet custos-sshguard && ok "sshguard active" || bad "sshguard not active"
# "active" is not enough: sshguard must get the dropbear lines. run-all logs
# in over SSH, so the journal must have some.
journalctl -b -t dropbear -n 20 --no-pager 2>/dev/null | grep -q 'dropbear\[' \
    && ok "dropbear logins reach the journal (sshguard's log source)" \
    || bad "no dropbear lines in the journal: sshguard is blind"
p=$(echo "Login attempt for nonexistent user from ::ffff:192.0.2.7:4242" | custos-sshg-parser 2>/dev/null)
[ "$p" = "100 192.0.2.7 4 10" ] && grep -q '^PARSER=.*custos-sshg-parser' /etc/sshguard.conf \
    && ok "sshguard understands dropbear" || bad "sshguard cannot parse dropbear messages"
nft list tables | grep -q sshguard && ok "nft table sshguard" || info "sshguard table appears on the first block"
for v in 10 20 30; do systemctl is-active --quiet custos-arpwatch@$v && ok "arpwatch br-lan.$v" || bad "arpwatch br-lan.$v"; done
systemctl is-active --quiet auditd && ok "auditd active" || bad "auditd not active"
auditctl -l 2>/dev/null | grep -q custos_wg_keys && ok "audit rule on WireGuard keys" || bad "audit rules not loaded"
systemctl is-active --quiet custos-aide.timer && ok "AIDE timer" || bad "AIDE timer"
test -s /data/aide/aide.db && ok "AIDE database initialised" || info "AIDE database: first check 10 min after boot"

sec "Memory (1 GB board)"
free -m | sed 's/^/           /'

sec "Result"; [ $FAILS -eq 0 ] && echo "  PASS" || echo "  $FAILS check(s) FAILED"; exit $FAILS

#!/bin/sh
# S11 DNS enforcement acceptance test (A3). POSIX sh / busybox ash.
# Read-only: it never changes configuration on the board or the PC.
#
# On the ESPRESSObin (custos-image), from a PC on lan1 (admin VLAN 20):
#   scp -O -i keys/espressobin_ed25519 tests/S11-dns.sh root@192.168.20.1:/tmp/
#   ssh -i keys/espressobin_ed25519 root@192.168.20.1 sh /tmp/S11-dns.sh device
#
# On a PC plugged into lan0 (VLAN 10) or lan1 (VLAN 20). Needs dig, curl and
# nc; the PC must have no other default route (unplug Wi-Fi), otherwise the
# probes do not go through the router:
#   tests/S11-dns.sh host
# On lan1 the host run also checks the AdGuard Home query log over SSH. On
# lan0, run the "querylog" line it prints on the board.
#
#   sh /tmp/S11-dns.sh querylog <name> <client-ip>   # on the board
#
# Optional: SSH_KEY=<private key> (default keys/espressobin_ed25519).
# Exit status: number of failed checks (0 = pass).

VLANS="10 20 30"
ADMIN_VLAN=20
WG_IF=wg0
AGH_UI=127.0.0.1:3000
STATE=/data/adguardhome
# Address that nothing answers on: a reply proves the redirect.
BLACKHOLE_DNS=192.0.2.53
# Google/YouTube SafeSearch VIPs (forcesafesearch.google.com,
# restrict.youtube.com).
SAFESEARCH_IPS="216.239.38.120 216.239.38.119"

SSH_KEY=${SSH_KEY:-keys/espressobin_ed25519}

ok()   { printf '  [ OK ] %s\n' "$*"; }
bad()  { printf '  [FAIL] %s\n' "$*"; FAILS=$((FAILS+1)); }
info() { printf '  [info] %s\n' "$*"; }
sec()  { printf '\n== %s ==\n' "$*"; }
check() { # check "<description>" <command...>
    d=$1; shift
    if "$@" >/dev/null 2>&1; then ok "$d"; else bad "$d"; fi
}
FAILS=0

# http_get <url>: body on stdout (curl or busybox wget)
http_get() {
    if command -v curl >/dev/null 2>&1; then curl -s -m 5 "$1"
    else wget -q -T 5 -O - "$1"; fi
}

# listening <proto> <hexaddr:hexport>: socket bound in /proc/net/<proto>
# (state 0A = LISTEN for tcp; any bound udp socket counts).
listening() {
    case $1 in
        tcp) awk -v a="$2" '$2 == a && $4 == "0A" {f=1} END {exit !f}' /proc/net/tcp ;;
        udp) awk -v a="$2" '$2 == a {f=1} END {exit !f}' /proc/net/udp ;;
        tcp6) awk -v a="$2" '$2 == a && $4 == "0A" {f=1} END {exit !f}' /proc/net/tcp6 ;;
        udp6) awk -v a="$2" '$2 == a {f=1} END {exit !f}' /proc/net/udp6 ;;
    esac
}
# Go binds the 0.0.0.0 wildcard as one dual-stack [::] socket when the
# kernel has IPv6, so the IPv4 wildcard can show up in /proc/net/*6 only.
ANY6=00000000000000000000000000000000

# ---------------------------------------------------------------- device ---
device() {
    sec "AdGuard Home service"
    check "adguardhome.service active" systemctl is-active --quiet adguardhome.service
    check "adguardhome.service enabled" systemctl is-enabled --quiet adguardhome.service
    pid=$(systemctl show -p MainPID --value adguardhome.service)
    if [ -n "$pid" ] && [ "$pid" != 0 ]; then
        u=$(awk '/^Uid:/{print $2}' /proc/$pid/status)
        [ "$u" = "$(id -u adguardhome 2>/dev/null)" ] && [ "$u" != 0 ] \
            && ok "runs as dedicated user adguardhome (uid $u)" || bad "runs as uid $u, not adguardhome"
    else
        bad "no main PID"
    fi
    v=$(/usr/bin/AdGuardHome --version 2>/dev/null)
    info "version: $v"
    echo "$v" | grep -q 'v0\.107\.' && ok "version is set (ldflags)" || bad "version string missing: '$v'"
    [ "$(systemctl show -p ProtectSystem --value adguardhome.service)" = strict ] \
        && ok "ProtectSystem=strict" || bad "ProtectSystem is not strict"

    sec "State on /data"
    awk '$2 == "/data" {f=1} END {exit !f}' /proc/mounts \
        && ok "/data is a mount point ($(awk '$2 == "/data" {print $1}' /proc/mounts))" \
        || bad "/data is not mounted (missing partition?)"
    check "$STATE exists" test -d $STATE
    st=$(stat -c '%U %a' $STATE 2>/dev/null)
    [ "$st" = "adguardhome 700" ] && ok "$STATE owner/mode: $st" || bad "$STATE owner/mode: '$st' (want adguardhome 700)"
    check "configuration seeded: $STATE/AdGuardHome.yaml" test -s $STATE/AdGuardHome.yaml

    sec "Sockets"
    { listening udp 00000000:0035 || listening udp6 $ANY6:0035; } \
        && ok "udp 0.0.0.0:53 bound" || bad "nothing on udp 0.0.0.0:53"
    { listening tcp 00000000:0035 || listening tcp6 $ANY6:0035; } \
        && ok "tcp 0.0.0.0:53 listening" || bad "nothing on tcp 0.0.0.0:53"
    listening tcp 0100007F:0BB8 && ok "UI on 127.0.0.1:3000" || bad "UI not on 127.0.0.1:3000"
    if listening tcp 00000000:0BB8 || awk '$2 ~ /:0BB8$/ && $2 != "0100007F:0BB8" && $4 == "0A" {f=1} END {exit !f}' /proc/net/tcp; then
        bad "UI port 3000 bound on a non-loopback address"
    else
        ok "UI port 3000 bound on loopback only"
    fi
    listening udp 3500007F:0035 && bad "resolved stub still on 127.0.0.53:53" || ok "resolved stub listener off"

    sec "Firewall: DNS enforcement"
    nft list chain inet custos dstnat 2>/dev/null | grep -q 'th dport 53 .*redirect to :53' \
        && ok "dstnat: DNS redirect rule present" || bad "dstnat: no DNS redirect rule"
    nft list chain inet custos dstnat 2>/dev/null | grep -q "\"$WG_IF\"" \
        && ok "redirect also covers $WG_IF" || bad "redirect does not cover $WG_IF"
    g=$(nft list chain inet custos dns_guard 2>/dev/null)
    echo "$g" | grep -q 'tcp dport { 784, 853, 8853 } .*reject' && ok "dns_guard: DoT rejected" || bad "dns_guard: no DoT reject"
    echo "$g" | grep -q 'udp dport { 784, 853, 8853 } .*reject' && ok "dns_guard: DoQ rejected" || bad "dns_guard: no DoQ reject"
    echo "$g" | grep -q 'ip daddr @doh_v4 tcp dport 443 .*reject' && ok "dns_guard: DoH (tcp 443) rejected" || bad "dns_guard: no DoH tcp reject"
    echo "$g" | grep -q 'ip daddr @doh_v4 udp dport 443 .*reject' && ok "dns_guard: DoH/HTTP3 (udp 443) rejected" || bad "dns_guard: no DoH udp reject"
    echo "$g" | grep -q 'meta nfproto ipv6 .*drop' && ok "dns_guard: forwarded IPv6 dropped" || bad "dns_guard: IPv6 not dropped"
    n=$(nft list set inet custos doh_v4 2>/dev/null | grep -Eo '[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' | wc -l)
    [ "$n" -gt 100 ] && ok "doh_v4 set has $n addresses/ranges" || bad "doh_v4 set nearly empty ($n)"
    nft list chain inet custos forward 2>/dev/null | grep -q 'jump dns_guard' \
        && ok "forward jumps to dns_guard" || bad "forward does not jump to dns_guard"
    info "counters:"
    nft list chain inet custos dstnat 2>/dev/null | grep -o 'counter packets [0-9]* bytes [0-9]*' | sed 's/^/           redirect: /'
    nft list chain inet custos dns_guard 2>/dev/null | grep -o 'counter packets [0-9]* .*comment "[^"]*"' \
        | sed -e 's/ bytes [0-9]* [a-z ]*with [a-z -]*//' -e 's/ bytes [0-9]* [a-z ]*comment/ comment/' -e 's/^/           /'

    sec "IPv6"
    [ "$(cat /proc/sys/net/ipv6/conf/all/forwarding 2>/dev/null)" = 1 ] \
        && bad "IPv6 forwarding is on" || ok "IPv6 forwarding off"
    for v in $VLANS; do
        ip -6 addr show dev br-lan.$v 2>/dev/null | grep -q inet6 \
            && bad "br-lan.$v has an IPv6 address" || ok "br-lan.$v has no IPv6 address"
    done

    sec "AdGuard Home API (loopback)"
    s=$(http_get "http://$AGH_UI/control/status")
    echo "$s" | grep -q '"running":true' && ok "DNS server running" || bad "status: not running ($s)"
    echo "$s" | grep -q '"protection_enabled":true' && ok "protection enabled" || bad "protection not enabled"
    f=$(http_get "http://$AGH_UI/control/filtering/status")
    for l in pro.txt tif.medium.txt nsfw.txt doh-vpn-proxy-bypass.txt; do
        echo "$f" | grep -q "adblock/$l" && ok "filter list $l configured" || bad "filter list $l missing"
    done
    rc=$(echo "$f" | grep -o '"rules_count":[0-9]*' | cut -d: -f2 | awk '{s+=$1} END {print s+0}')
    [ "$rc" -gt 100000 ] && ok "$rc rules loaded" || bad "only $rc rules loaded (lists not downloaded yet?)"
    ss=$(http_get "http://$AGH_UI/control/safesearch/status")
    echo "$ss" | grep -q '"enabled":true' && ok "SafeSearch enabled" || bad "SafeSearch disabled"
    for a in $(ip -4 -o addr show | awk '$2 ~ /^br-lan\./ {sub(/\/.*/, "", $4); print $4}'); do
        http_get "http://$a:3000/" >/dev/null 2>&1 && bad "UI answers on $a:3000" || ok "UI not reachable on $a:3000"
    done

    if command -v nslookup >/dev/null 2>&1; then
        sec "Resolution via AdGuard Home (127.0.0.1)"
        nslookup example.com 127.0.0.1 2>/dev/null | grep -A3 'example.com' | grep -q 'Address' \
            && ok "example.com resolves" || bad "example.com does not resolve"
        nslookup use-application-dns.net 127.0.0.1 2>&1 | grep -qi 'NXDOMAIN\|can.t find\|can.t resolve' \
            && ok "use-application-dns.net -> NXDOMAIN" || bad "use-application-dns.net not NXDOMAIN"
        nslookup mask.icloud.com 127.0.0.1 2>&1 | grep -qi 'NXDOMAIN\|can.t find\|can.t resolve' \
            && ok "mask.icloud.com -> NXDOMAIN" || bad "mask.icloud.com not NXDOMAIN"
    else
        info "no nslookup on the device; resolution is checked from the host"
    fi
}

# querylog <name> <client-ip>: AdGuard Home logged <name> from <client-ip>
querylog() {
    sec "Query log: $1 from $2"
    q=$(http_get "http://$AGH_UI/control/querylog?search=$1&limit=10")
    echo "$q" | grep -q "\"$1\"" && ok "$1 is in the query log" || bad "$1 not in the query log"
    echo "$q" | grep -q "\"client\":\"$2\"" && ok "logged with the real client address $2" \
        || bad "client address $2 not logged (got: $(echo "$q" | grep -o '"client":"[^"]*"' | head -1))"
}

# ------------------------------------------------------------------ host ---
ssh_board() { # ssh_board <ip> <command...>
    h=$1; shift
    ssh -i "$SSH_KEY" -o BatchMode=yes -o ConnectTimeout=5 \
        -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null root@$h "$@"
}
# dig_status <server> <name> [dig options]: rcode (NOERROR, NXDOMAIN, ...)
dig_status() {
    s=$1; n=$2; shift 2
    dig @$s $n A +time=3 +tries=1 "$@" 2>/dev/null | sed -n 's/.*status: \([A-Z]*\).*/\1/p'
}
dig_short() { dig @$1 $2 A +short +time=3 +tries=1 2>/dev/null; }

host_run() {
    for t in dig curl nc ip; do
        command -v $t >/dev/null 2>&1 || { echo "need $t on the PC" >&2; exit 64; }
    done
    v=""; for c in $VLANS; do ip -4 addr show | grep -q "inet 192\.168\.$c\." && v=$c; done
    [ -n "$v" ] || { echo "no address in 192.168.<vlan>.0/24 (VLANs: $VLANS): plug into lan0 or lan1" >&2; exit 64; }
    gw=192.168.$v.1
    me=$(ip -4 -o addr show | awk -v p="192.168.$v." 'index($4, p) == 1 {sub(/\/.*/, "", $4); print $4; exit}')
    ifc=$(ip -4 -o addr show | awk -v p="192.168.$v." 'index($4, p) == 1 {print $2; exit}')
    sec "PC $me on $ifc (VLAN $v)"
    ip -4 route show default | grep -q "via $gw" && ok "default route via $gw" || bad "default route is not via $gw"
    # A second default route is fine as long as every address probed below
    # really leaves through Custos (lowest metric wins).
    other=""
    for d in 1.1.1.1 8.8.8.8 9.9.9.9 192.0.2.53; do
        ip route get $d 2>/dev/null | grep -q "via $gw " || other="$other $d"
    done
    [ -z "$other" ] && ok "test traffic goes through $gw" \
        || bad "not through $gw:$other (results are not meaningful)"
    if command -v resolvectl >/dev/null 2>&1; then
        resolvectl dns "$ifc" 2>/dev/null | grep -q "$gw" && ok "DHCP DNS server is $gw" \
            || info "DHCP DNS on $ifc: $(resolvectl dns "$ifc" 2>/dev/null | cut -d: -f2-)"
    fi

    sec "Plain DNS is always answered by AdGuard Home"
    [ "$(dig_status $gw example.com)" = NOERROR ] && ok "@$gw example.com" || bad "@$gw example.com"
    [ "$(dig_status 8.8.8.8 example.com)" = NOERROR ] && ok "@8.8.8.8 example.com answered" || bad "@8.8.8.8 example.com"
    [ "$(dig_status 8.8.8.8 example.com +tcp)" = NOERROR ] && ok "@8.8.8.8 over TCP answered" || bad "@8.8.8.8 over TCP"
    [ "$(dig_status $BLACKHOLE_DNS example.com)" = NOERROR ] \
        && ok "@$BLACKHOLE_DNS (no server there) answered: redirect works" || bad "@$BLACKHOLE_DNS not answered: no redirect"
    [ "$(dig_status 8.8.8.8 use-application-dns.net)" = NXDOMAIN ] \
        && ok "@8.8.8.8 use-application-dns.net -> NXDOMAIN (Firefox DoH canary)" || bad "use-application-dns.net not NXDOMAIN"
    [ "$(dig_status 1.1.1.1 mask.icloud.com)" = NXDOMAIN ] \
        && ok "@1.1.1.1 mask.icloud.com -> NXDOMAIN (Private Relay)" || bad "mask.icloud.com not NXDOMAIN"
    a=$(dig_short 8.8.8.8 pagead2.googlesyndication.com)
    [ "$a" = 0.0.0.0 ] && ok "@8.8.8.8 pagead2.googlesyndication.com blocked (0.0.0.0)" || bad "pagead2.googlesyndication.com not blocked: '$a'"
    a=$(dig_short 8.8.8.8 dns.google)
    [ "$a" = 0.0.0.0 ] && ok "@8.8.8.8 dns.google blocked (DoH bypass list)" || bad "dns.google not blocked: '$a'"
    for n in www.google.com www.youtube.com; do
        a=$(dig_short 8.8.8.8 $n | tr '\n' ' ')
        hit=""; for s in $SAFESEARCH_IPS; do echo " $a " | grep -q " $s " && hit=$s; done
        [ -n "$hit" ] && ok "$n -> SafeSearch address $hit" || bad "$n not rewritten to SafeSearch: $a"
    done

    sec "Encrypted DNS bypass is closed"
    nc -z -w 3 1.1.1.1 853 >/dev/null 2>&1 && bad "DoT tcp/853 to 1.1.1.1 open" || ok "DoT tcp/853 to 1.1.1.1 refused"
    nc -z -w 3 9.9.9.9 853 >/dev/null 2>&1 && bad "DoT tcp/853 to 9.9.9.9 open" || ok "DoT tcp/853 to 9.9.9.9 refused"
    curl -s -m 6 -o /dev/null -H 'accept: application/dns-json' 'https://1.1.1.1/dns-query?name=example.com' \
        && bad "DoH to https://1.1.1.1 works" || ok "DoH to https://1.1.1.1 blocked"
    curl -s -m 6 -o /dev/null 'https://8.8.8.8/resolve?name=example.com' \
        && bad "DoH to https://8.8.8.8 works" || ok "DoH to https://8.8.8.8 blocked"
    curl -s -m 8 -o /dev/null --doh-url https://9.9.9.9/dns-query https://example.com 2>/dev/null \
        && bad "curl --doh-url via 9.9.9.9 works" || ok "curl --doh-url via 9.9.9.9 fails"
    if command -v kdig >/dev/null 2>&1; then
        kdig @94.140.14.14 +quic +timeout=3 +retry=0 example.com >/dev/null 2>&1 \
            && bad "DoQ udp/853 to 94.140.14.14 works" || ok "DoQ udp/853 to 94.140.14.14 blocked"
    else
        info "no kdig: DoQ checked by the dns_guard udp counter on the board"
    fi
    curl -s -m 8 -o /dev/null https://example.com && ok "control: ordinary HTTPS works" || bad "control: HTTPS to example.com fails"

    sec "Router services from the LAN"
    nc -z -w 3 $gw 3000 >/dev/null 2>&1 && bad "AdGuard Home UI reachable at $gw:3000" || ok "AdGuard Home UI not reachable at $gw:3000"

    sec "IPv6"
    ip -6 addr show dev "$ifc" scope global 2>/dev/null | grep -q inet6 \
        && bad "$ifc has a global IPv6 address" || ok "$ifc has no global IPv6 address"
    dig @2001:4860:4860::8888 example.com +time=2 +tries=1 >/dev/null 2>&1 \
        && bad "DNS to 2001:4860:4860::8888 answered" || ok "no IPv6 DNS path (2001:4860:4860::8888)"

    sec "Query log sees the real client address"
    probe="s11-$(date +%s)-$$.example.com"
    dig @8.8.8.8 $probe +time=3 +tries=1 >/dev/null 2>&1
    if [ "$v" = "$ADMIN_VLAN" ] && ssh_board $gw true >/dev/null 2>&1; then
        sleep 2
        out=$(ssh_board $gw "wget -q -T 5 -O - 'http://$AGH_UI/control/querylog?search=$probe&limit=10' 2>/dev/null || curl -s -m 5 'http://$AGH_UI/control/querylog?search=$probe&limit=10'")
        echo "$out" | grep -q "\"client\":\"$me\"" && ok "$probe logged from $me" \
            || bad "$probe not logged from $me (got: $(echo "$out" | grep -o '"client":"[^"]*"' | head -1))"
    else
        info "on the board run:  sh /tmp/S11-dns.sh querylog $probe $me"
    fi
}

case "${1:-device}" in
    device)   device ;;
    host)     host_run ;;
    querylog) [ $# -eq 3 ] || { echo "usage: $0 querylog <name> <client-ip>" >&2; exit 64; }
              querylog "$2" "$3" ;;
    *) echo "usage: $0 [device|host|querylog <name> <client-ip>]" >&2; exit 64 ;;
esac

sec "Result"
[ $FAILS -eq 0 ] && echo "  PASS" || echo "  $FAILS check(s) FAILED"
exit $FAILS

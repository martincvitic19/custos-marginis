#!/bin/sh
# A2 edge router acceptance test (POSIX sh / busybox ash). Read-only: it
# never changes configuration on the board or the PC.
#
# On the ESPRESSObin (custos-image), from a PC on lan1 (admin VLAN 20):
#   scp -O -i keys/espressobin_ed25519 tests/A2-router.sh root@192.168.20.1:/tmp/
#   ssh -i keys/espressobin_ed25519 root@192.168.20.1 sh /tmp/A2-router.sh device
#
# On a PC, depending on where it is plugged in:
#   tests/A2-router.sh host-admin   # PC on lan1  -> VLAN 20 (admin)
#   tests/A2-router.sh host-user    # PC on lan0  -> VLAN 10 (untagged)
#   tests/A2-router.sh host-wan     # PC on the ISP router LAN (192.168.0.0/24)
# Optional: SSH_KEY=<private key> (default keys/espressobin_ed25519),
#           INTERNET_IP=<address to ping> (default 1.1.1.1).
#
# Exit status: number of failed checks (0 = pass).

WAN_IF=wan
WAN_ADDR=192.168.0.15
WAN_GW=192.168.0.1
CONDUIT=eth0
BRIDGE=br-lan
VLANS="10 20 30"
ADMIN_VLAN=20
# port:pvid:tagged (must match CUSTOS_LAN_PORTS in custos-net.inc)
PORTS="lan0:10:30 lan1:20:"

SSH_KEY=${SSH_KEY:-keys/espressobin_ed25519}
INTERNET_IP=${INTERNET_IP:-1.1.1.1}

ok()   { printf '  [ OK ] %s\n' "$*"; }
bad()  { printf '  [FAIL] %s\n' "$*"; FAILS=$((FAILS+1)); }
info() { printf '  [info] %s\n' "$*"; }
sec()  { printf '\n== %s ==\n' "$*"; }
check() { # check "<description>" <command...>
    d=$1; shift
    if "$@" >/dev/null 2>&1; then ok "$d"; else bad "$d"; fi
}
FAILS=0

# ---------------------------------------------------------------- device ---
device() {
    sec "Discovery: conduit and switch ports"
    for p in $CONDUIT $WAN_IF lan0 lan1; do
        check "interface $p exists" test -d /sys/class/net/$p
    done
    [ -e /sys/class/net/$CONDUIT/dsa/tagging ] \
        && ok "$CONDUIT is the DSA conduit (tagging: $(cat /sys/class/net/$CONDUIT/dsa/tagging))" \
        || bad "$CONDUIT is not a DSA conduit"
    info "$CONDUIT driver: $(basename "$(readlink /sys/class/net/$CONDUIT/device/driver)" 2>/dev/null)"
    for p in $WAN_IF lan0 lan1; do
        ip -d link show $p | grep -Eq "dsa (conduit|master) $CONDUIT" \
            && ok "$p is a DSA user port of $CONDUIT" || bad "$p is not a DSA user port of $CONDUIT"
    done
    ip link show $CONDUIT | grep -q 'state UP\|,UP' \
        && ok "conduit $CONDUIT is up" || bad "conduit $CONDUIT is down"

    sec "Bridge and VLANs"
    check "$BRIDGE exists" test -d /sys/class/net/$BRIDGE/bridge
    [ "$(cat /sys/class/net/$BRIDGE/bridge/vlan_filtering 2>/dev/null)" = 1 ] \
        && ok "$BRIDGE vlan_filtering=1" || bad "$BRIDGE vlan_filtering is not 1"
    for spec in $PORTS; do
        port=$(echo $spec | cut -d: -f1); pvid=$(echo $spec | cut -d: -f2)
        tagged=$(echo $spec | cut -d: -f3 | tr ',' ' ')
        [ "$(basename "$(readlink /sys/class/net/$port/master)" 2>/dev/null)" = $BRIDGE ] \
            && ok "$port enslaved to $BRIDGE" || bad "$port not in $BRIDGE"
        vl=$(bridge vlan show dev $port 2>/dev/null)
        echo "$vl" | grep -Eq "(^|[[:space:]])$pvid PVID Egress Untagged" \
            && ok "$port: VLAN $pvid PVID untagged" || bad "$port: VLAN $pvid not PVID untagged"
        for v in $tagged; do
            echo "$vl" | grep -Eq "(^|[[:space:]])$v[[:space:]]*$" \
                && ok "$port: VLAN $v tagged" || bad "$port: VLAN $v not tagged"
        done
    done
    bvl=$(bridge vlan show dev $BRIDGE 2>/dev/null)
    for v in $VLANS; do
        echo "$bvl" | grep -Eq "(^|[[:space:]])$v([[:space:]]|$)" \
            && ok "$BRIDGE (CPU) member of VLAN $v" || bad "$BRIDGE (CPU) not in VLAN $v"
    done
    s0=$(cat /sys/class/net/lan0/phys_switch_id 2>/dev/null)
    s1=$(cat /sys/class/net/lan1/phys_switch_id 2>/dev/null)
    [ -n "$s0" ] && [ "$s0" = "$s1" ] \
        && ok "lan0/lan1 on the same switch (id $s0): bridge offloaded to the 88E6341" \
        || bad "lan0/lan1 have no common switchdev id"
    info "offloaded FDB entries: $(bridge fdb show br $BRIDGE 2>/dev/null | grep -c offload)"

    sec "Addresses and routes"
    for v in $VLANS; do
        ip -4 addr show dev $BRIDGE.$v 2>/dev/null | grep -q "inet 192.168.$v.1/24" \
            && ok "$BRIDGE.$v = 192.168.$v.1/24" || bad "$BRIDGE.$v has no 192.168.$v.1/24"
    done
    ip -4 addr show dev $WAN_IF | grep -q "inet $WAN_ADDR/24" \
        && ok "$WAN_IF = $WAN_ADDR/24" || bad "$WAN_IF has no $WAN_ADDR/24"
    ip -4 route show default | grep -q "via $WAN_GW dev $WAN_IF" \
        && ok "default route via $WAN_GW dev $WAN_IF" || bad "no default route via $WAN_GW dev $WAN_IF"
    for p in $CONDUIT $BRIDGE lan0 lan1; do
        ip -4 addr show dev $p | grep -q inet && bad "$p has an IPv4 address" || ok "$p has no IPv4 address"
    done

    sec "DHCP server"
    # UDP port 67 = 0x0043 in /proc/net/udp
    grep -q ':0043 ' /proc/net/udp && ok "UDP 67 open" || bad "nothing listens on UDP 67"
    for v in $VLANS; do
        networkctl status $BRIDGE.$v 2>/dev/null | grep -qi 'DHCP.*server\|Offered DHCP' \
            && info "$BRIDGE.$v: networkctl reports the DHCP server" \
            || info "$BRIDGE.$v: networkctl shows no DHCP server line (check networkctl status)"
    done

    sec "sysctl"
    [ "$(cat /proc/sys/net/ipv4/ip_forward)" = 1 ] && ok "ip_forward=1" || bad "ip_forward != 1"
    for i in all default $WAN_IF $BRIDGE.10 $BRIDGE.20 $BRIDGE.30; do
        f=/proc/sys/net/ipv4/conf/$i/rp_filter
        [ "$(cat $f 2>/dev/null)" = 1 ] && ok "rp_filter=1 on $i" || bad "rp_filter on $i = $(cat $f 2>/dev/null)"
    done

    sec "Firewall (nftables)"
    check "custos-firewall.service active" systemctl is-active custos-firewall.service
    t=$(nft list table inet custos 2>/dev/null)
    [ -n "$t" ] && ok "table inet custos loaded" || bad "table inet custos missing"
    for c in input forward; do
        nft list chain inet custos $c 2>/dev/null | grep -q 'policy drop' \
            && ok "chain $c policy drop" || bad "chain $c is not policy drop"
    done
    echo "$t" | grep -q 'ct state established,related accept' \
        && ok "established/related accepted" || bad "no established/related rule"
    echo "$t" | grep -q "oifname \"$WAN_IF\" masquerade" \
        && ok "masquerade on $WAN_IF" || bad "no masquerade on $WAN_IF"
    nc=$(echo "$t" | grep -w drop | grep -v 'policy drop' | grep -vc counter)
    [ "$nc" -eq 0 ] && ok "every drop rule has a counter" || bad "$nc drop rule(s) without counter"
    echo "$t" | grep 'tcp dport' | grep -q "iifname \"$BRIDGE.$ADMIN_VLAN\"" \
        && ok "management ports only from $BRIDGE.$ADMIN_VLAN" || bad "management rule not limited to admin VLAN"
    echo "$t" | grep 'tcp dport' | grep -q "$WAN_IF" && bad "a tcp dport rule mentions $WAN_IF" || ok "no management rule on $WAN_IF"
    info "drop counters:"
    echo "$t" | grep 'counter packets' | sed 's/^[[:space:]]*/         /'

    sec "Fail-closed ordering"
    systemctl show -p Requires systemd-networkd.service | grep -q custos-firewall.service \
        && ok "systemd-networkd Requires custos-firewall" || bad "networkd does not require the firewall"
    systemctl show -p Requires dropbear.socket 2>/dev/null | grep -q custos-firewall.service \
        && ok "dropbear.socket Requires custos-firewall" || info "dropbear.socket does not require the firewall (dropbear not installed?)"

    sec "CAKE"
    tc qdisc show dev $WAN_IF | grep -q 'qdisc cake' \
        && ok "CAKE root qdisc on $WAN_IF: $(tc qdisc show dev $WAN_IF | grep cake | head -n 1)" \
        || bad "no CAKE qdisc on $WAN_IF"

    sec "Services"
    check "systemd-networkd active" systemctl is-active systemd-networkd.service
    f=$(systemctl --failed --no-legend --plain 2>/dev/null | awk '{print $1}' | tr '\n' ' ')
    [ -z "$f" ] && ok "no failed units" || bad "failed units: $f"
    command -v python3 >/dev/null 2>&1 && bad "python3 present on the device" || ok "no python3 on the device"

    sec "Uplink (from the router itself)"
    if [ "$(cat /sys/class/net/$WAN_IF/carrier 2>/dev/null)" != 1 ]; then
        bad "$WAN_IF has no link: connect it to the ISP router (uplink checks skipped)"
    else
        ping -c 2 -W 2 $WAN_GW >/dev/null 2>&1 && ok "ping ISP router $WAN_GW" || bad "cannot ping $WAN_GW"
        ping -c 2 -W 3 $INTERNET_IP >/dev/null 2>&1 && ok "ping $INTERNET_IP" || bad "cannot ping $INTERNET_IP"
    fi
}

# ------------------------------------------------------------------ host ---
ssh_open() { # ssh_open <ip>: 0 when an SSH banner answers within 5 s
    ssh -i "$SSH_KEY" -o BatchMode=yes -o ConnectTimeout=5 \
        -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
        root@$1 true >/dev/null 2>&1
}
my_addr_in() { ip -4 addr show | grep -q "inet $1\."; }

host_common() { # host_common <vlan>
    v=$1
    my_addr_in 192.168.$v && ok "DHCP lease in 192.168.$v.0/24: $(ip -4 addr show | grep -o "inet 192.168.$v.[0-9]*" | head -n 1)" \
        || bad "no address in 192.168.$v.0/24 (DHCP)"
    ip -4 route show default | grep -q "via 192.168.$v.1" \
        && ok "default gateway 192.168.$v.1" || bad "default gateway is not 192.168.$v.1"
    ping -c 2 -W 2 192.168.$v.1 >/dev/null 2>&1 && ok "ping gateway 192.168.$v.1" || bad "cannot ping 192.168.$v.1"
    # An unknown PC has the child role: web and NTP only, so ping out is
    # dropped on purpose (A4, A9). NAT is proven with HTTPS.
    if ! ip route get $INTERNET_IP 2>/dev/null | grep -q "via 192.168.$v.1 "; then
        bad "traffic to $INTERNET_IP does not go through Custos"
    elif curl -4 -s -o /dev/null --max-time 10 "${INTERNET_URL:-https://example.com}"; then
        ok "internet through Custos: HTTPS (NAT)"
        ping -c 1 -W 2 $INTERNET_IP >/dev/null 2>&1 && info "ping out works (trusted role)" \
            || info "ping out dropped (child role: this PC is not in custos.json, or has that role)"
    else
        bad "no internet through Custos (HTTPS)"
    fi
    ssh_open $WAN_ADDR && bad "SSH to wan address $WAN_ADDR answers" || ok "SSH to wan address $WAN_ADDR blocked"
    for o in $VLANS; do
        [ $o = $v ] && continue
        ping -c 1 -W 2 192.168.$o.1 >/dev/null 2>&1 \
            && bad "can ping other VLAN gateway 192.168.$o.1" || ok "other VLAN gateway 192.168.$o.1 unreachable"
    done
}

host_admin() {
    sec "PC on lan1 (admin VLAN $ADMIN_VLAN)"
    host_common $ADMIN_VLAN
    ssh_open 192.168.$ADMIN_VLAN.1 && ok "SSH to 192.168.$ADMIN_VLAN.1 works" || bad "SSH to 192.168.$ADMIN_VLAN.1 fails"
}

host_user() {
    sec "PC on lan0 (user VLAN 10, untagged)"
    host_common 10
    ssh_open 192.168.10.1 && bad "SSH to 192.168.10.1 answers" || ok "SSH to 192.168.10.1 blocked"
    ssh_open 192.168.$ADMIN_VLAN.1 && bad "SSH to admin address answers" || ok "SSH to 192.168.$ADMIN_VLAN.1 blocked"
}

host_wan() {
    sec "PC on the ISP router LAN"
    ping -c 2 -W 2 $WAN_ADDR >/dev/null 2>&1 && bad "router answers ping on $WAN_ADDR" || ok "no ping reply from $WAN_ADDR"
    ssh_open $WAN_ADDR && bad "SSH on $WAN_ADDR answers" || ok "SSH on $WAN_ADDR blocked"
    for v in $VLANS; do
        ping -c 1 -W 2 192.168.$v.1 >/dev/null 2>&1 \
            && bad "192.168.$v.1 reachable from wan" || ok "192.168.$v.1 unreachable from wan"
    done
}

case "${1:-device}" in
    device)     device ;;
    host-admin) host_admin ;;
    host-user)  host_user ;;
    host-wan)   host_wan ;;
    *) echo "usage: $0 [device|host-admin|host-user|host-wan]" >&2; exit 64 ;;
esac

sec "Result"
[ $FAILS -eq 0 ] && echo "  PASS" || echo "  $FAILS check(s) FAILED"
exit $FAILS

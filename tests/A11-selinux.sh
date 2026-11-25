#!/bin/sh
# A11 SELinux test, ON the board:  sh /tmp/A11-selinux.sh [collect]
#   (no argument)  state, domains, labels, AVC count since boot
#   collect        also print every distinct AVC (for the policy module)
ok()   { printf '  [ OK ] %s\n' "$*"; }
bad()  { printf '  [FAIL] %s\n' "$*"; FAILS=$((FAILS+1)); }
info() { printf '  [info] %s\n' "$*"; }
sec()  { printf '\n== %s ==\n' "$*"; }
FAILS=0

sec "SELinux state"
command -v sestatus >/dev/null && sestatus | sed 's/^/           /'
m=$(getenforce 2>/dev/null)
case "$m" in
    Enforcing) ok "enforcing" ;;
    Permissive) info "permissive (AVC collection phase; enforcing once the count below is 0)" ;;
    *) bad "SELinux not active: '$m'" ;;
esac

sec "Domains"
for p in custos-apply custosd custos-watch AdGuardHome; do
    pid=$(pidof $p | cut -d' ' -f1)
    [ -n "$pid" ] || { info "$p not running"; continue; }
    ctx=$(cat /proc/$pid/attr/current 2>/dev/null | tr -d '\0')
    case "$ctx" in
        *custos*_t*|*adguardhome_t*) ok "$p: $ctx" ;;
        *) bad "$p: $ctx (not in its own domain)" ;;
    esac
done

sec "File labels"
for f in /data/custos /data/custos/wg /data/custosd /data/adguardhome /run/custos /usr/bin/custosd; do
    [ -e $f ] && info "$(ls -dZ $f 2>/dev/null)"
done
ls -dZ /data/custos/wg 2>/dev/null | grep -q custos_wg_key_t && ok "WireGuard keys: custos_wg_key_t" || bad "wg dir not custos_wg_key_t"

sec "S3: who may read the WireGuard keys (kernel policy decision)"
# compute_av asks the loaded policy, so this holds for any code running as
# custosd_t, including a compromised custosd (the S3 scenario). The live
# demo with the vulnerable_demo build is the AVC record of the same denial.
if command -v compute_av >/dev/null; then
    av() { compute_av "system_u:system_r:$1:s0" system_u:object_r:custos_wg_key_t:s0 file 2>/dev/null | sed -n 's/^allowed= *//p'; }
    a=$(av custosd_t)
    case "$a" in *read*|*open*) bad "custosd_t may access custos_wg_key_t: $a" ;; *) ok "custosd_t: no access to WireGuard keys ($a)" ;; esac
    a=$(av systemd_networkd_t)
    case "$a" in *write*) bad "networkd may write the keys: $a" ;; *read*) ok "systemd-networkd: read only" ;; *) bad "networkd cannot read the keys: $a" ;; esac
else
    info "compute_av not installed: S3 policy check skipped"
fi

sec "AVC denials since boot"
log() { journalctl -b --no-pager 2>/dev/null; cat /var/log/audit/audit.log 2>/dev/null; }
n=$(log | grep -c 'avc:  denied')
[ "$n" -eq 0 ] && ok "0 AVC denials" || bad "$n AVC denial(s)"
if [ "${1:-}" = collect ] || [ "$n" -gt 0 ]; then
    log | grep 'avc:  denied' | sed -e 's/.*avc:  denied  //' -e 's/ pid=[0-9]*//' -e 's/ ino=[0-9]*//' \
        -e 's/ dev="[^"]*"//' -e 's/ permissive=[01]//' | sort | uniq -c | sort -rn | head -n 60 | sed 's/^/           /'
fi

sec "Result"; [ $FAILS -eq 0 ] && echo "  PASS" || echo "  $FAILS check(s) FAILED"; exit $FAILS

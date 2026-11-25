#!/bin/sh
# A10 integrity test, ON the board:  sh /tmp/A10-integrity.sh
# What is enforced today: kernel lockdown (integrity) from boot, module
# signatures, BPF and kernel pointer hardening, sandboxed services, a
# read-only root file system checked against its dm-verity root hash.
# What is NOT yet: booting from the dm-verity device, signed FIT, locked
# U-Boot (docs/boot-chain.md).
ok()   { printf '  [ OK ] %s\n' "$*"; }
bad()  { printf '  [FAIL] %s\n' "$*"; FAILS=$((FAILS+1)); }
info() { printf '  [info] %s\n' "$*"; }
sec()  { printf '\n== %s ==\n' "$*"; }
FAILS=0

sec "Kernel lockdown"
l=$(cat /sys/kernel/security/lockdown 2>/dev/null)
echo "$l" | grep -q '\[integrity\]\|\[confidentiality\]' && ok "lockdown: $l" || bad "lockdown: '$l' (custos-hardening missing = baseline image?)"
dd if=/dev/mem of=/dev/null bs=1 count=1 2>/dev/null && bad "/dev/mem readable" || ok "/dev/mem blocked"

sec "Sysctl hardening"
for k in kernel.kptr_restrict=2 kernel.dmesg_restrict=1 kernel.unprivileged_bpf_disabled=1 net.core.bpf_jit_harden=2 kernel.yama.ptrace_scope=2; do
    n=${k%=*}; want=${k#*=}; got=$(cat /proc/sys/$(echo $n | tr . /) 2>/dev/null)
    [ "$got" = "$want" ] || { [ "$n" = kernel.unprivileged_bpf_disabled ] && [ "$got" = 2 ]; } && ok "$n=$got" || bad "$n=$got (want $want)"
done

sec "Service sandboxes"
for u in adguardhome custosd custos-watch; do
    s=$(systemctl show -p User -p ProtectSystem -p NoNewPrivileges -p CapabilityBoundingSet $u.service | tr '\n' ' ')
    echo "$s" | grep -q 'ProtectSystem=strict' && echo "$s" | grep -q 'NoNewPrivileges=yes' && ! echo "$s" | grep -q 'User= ' \
        && ok "$u: $s" || bad "$u: $s"
done
info "custos-apply runs as root by design (it changes nft/networkd); it only reads custos.json, validated."

sec "Module signatures"
[ "$(cat /sys/module/module/parameters/sig_enforce 2>/dev/null)" = Y ] && ok "sig_enforce=Y" || bad "module signatures not enforced"

sec "Read-only root file system"
grep -qE '^[^ ]+ / [^ ]+ ro[, ]' /proc/mounts && ok "/ mounted read-only" || bad "/ is read-write: $(grep ' / ' /proc/mounts)"
grep -qw ro /proc/cmdline && ok "kernel command line: ro" || info "kernel command line has no 'ro' (older boot.scr: superblock written at mount)"
touch /usr/.custos-ro-test 2>/dev/null && { rm -f /usr/.custos-ro-test; bad "/usr writable"; } || ok "/usr not writable"
[ -x /usr/lib/custos/custos-verity-check ] && {
    out=$(/usr/lib/custos/custos-verity-check 2>&1); rc=$?
    case "$rc:$out" in
        0:*matches*) ok "dm-verity: $out" ;;
        0:*) info "dm-verity: $out" ;;
        *) bad "dm-verity: $out" ;;
    esac
} || info "custos-verity-check not installed"
# The clock file is empty by design: timesyncd only uses its mtime.
[ -e /var/lib/systemd/timesync/clock ] && grep -q ' /var/lib/systemd/timesync ' /proc/mounts && ok "timesyncd clock on /data" || info "timesyncd clock not on /data yet"

sec "Not enforced yet (see docs/boot-chain.md)"
info "boot from the dm-verity device: $(grep -o 'root=/dev/dm-[0-9]*' /proc/cmdline || echo no)"
info "U-Boot: factory U-Boot in SPI NOR, bootdelay unchanged"

sec "Result"; [ $FAILS -eq 0 ] && echo "  PASS" || echo "  $FAILS check(s) FAILED"; exit $FAILS

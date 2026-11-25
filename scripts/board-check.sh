#!/bin/sh
# Quick BSP + rootfs health check, run ON the ESPRESSObin (busybox sh).
#   scp -O -i keys/espressobin_ed25519 scripts/board-check.sh root@192.168.50.2:/tmp/
#   ssh -i keys/espressobin_ed25519 root@192.168.50.2 sh /tmp/board-check.sh
# Read-only: it only reads /proc, /sys and logs.

ok()   { printf '  [ OK ] %s\n' "$*"; }
bad()  { printf '  [FAIL] %s\n' "$*"; FAILS=$((FAILS+1)); }
info() { printf '  [info] %s\n' "$*"; }
sec()  { printf '\n== %s ==\n' "$*"; }
FAILS=0

sec "System"
info "model: $(tr -d '\0' < /proc/device-tree/model)"
info "kernel: $(uname -r)  $(uname -m)"
info "os: $(. /etc/os-release; echo "$PRETTY_NAME")"
info "cmdline: $(cat /proc/cmdline)"
grep -q 'espressobin' /proc/device-tree/compatible && ok "DT compatible espressobin" || bad "DT compatible"

sec "CPU / memory"
n=$(grep -c ^processor /proc/cpuinfo); [ "$n" -eq 2 ] && ok "2 CPU cores" || bad "CPU cores: $n (expected 2)"
m=$(awk '/MemTotal/{print int($2/1024)}' /proc/meminfo); info "MemTotal ${m} MiB"
[ "$m" -gt 700 ] && ok "RAM size plausible" || bad "RAM only ${m} MiB"
f=/sys/devices/system/cpu/cpu0/cpufreq
[ -d $f ] && ok "cpufreq: $(cat $f/scaling_driver) cur=$(($(cat $f/scaling_cur_freq)/1000)) MHz avail=$(cat $f/scaling_available_frequencies 2>/dev/null)" \
          || info "no cpufreq (not critical)"
for t in /sys/class/thermal/thermal_zone*/temp; do [ -r "$t" ] && info "temp $(dirname $t | xargs basename): $(($(cat $t)/1000)) C"; done

sec "Storage"
[ -b /dev/mmcblk0 ] && ok "SD/eMMC block devices: $(ls /dev/mmcblk? | tr '\n' ' ')" || bad "no mmcblk devices"
for d in /sys/block/mmcblk?; do
  [ -e "$d" ] || continue
  info "$(basename $d): $(($(cat $d/size)/2048)) MiB  type=$(cat $d/device/type 2>/dev/null)  name=$(cat $d/device/name 2>/dev/null)"
done
ls /sys/block | grep -q 'mmcblk.*boot0' && ok "eMMC visible (boot partitions present)" || info "eMMC not visible (check DTB is the -emmc variant)"
[ -e /proc/mtd ] && grep -q mtd /proc/mtd && ok "SPI NOR: $(tail -n +2 /proc/mtd | tr '\n' ' ')" || info "SPI NOR not exposed as MTD"
info "root: $(awk '$2=="/"{print $1, $3, $4}' /proc/mounts)"
awk '$2=="/"{print $4}' /proc/mounts | grep -q '^rw' && ok "rootfs mounted rw" || bad "rootfs not rw"
df -h / | tail -1 | awk '{print "  [info] / size="$2" used="$3" avail="$4}'
dmesg | grep -qiE 'EXT4-fs (error|warning)' && bad "ext4 errors in dmesg" || ok "no ext4 errors"

sec "Network (DSA 88E6341)"
for i in eth0 wan lan0 lan1; do
  if [ -d /sys/class/net/$i ]; then
    ok "$i present, state=$(cat /sys/class/net/$i/operstate) carrier=$(cat /sys/class/net/$i/carrier 2>/dev/null || echo -)"
  else bad "$i missing"; fi
done
ip -br addr 2>/dev/null || ip -o -4 addr 2>/dev/null | sed 's/^/  [info] /'

sec "USB / PCIe / SATA"
u=$(ls /sys/bus/usb/devices 2>/dev/null | grep -c '^usb'); [ "$u" -ge 1 ] && ok "$u USB root hubs" || bad "no USB root hubs"
ls /sys/bus/usb/devices | grep -v -e '^usb' -e ':' | while read d; do info "USB device $d: $(cat /sys/bus/usb/devices/$d/product 2>/dev/null)"; done
[ -d /sys/bus/pci/devices ] && info "PCI devices: $(ls /sys/bus/pci/devices | wc -l)" || info "no PCI bus (ok if mini-PCIe empty/disabled)"
ls /sys/class/ata_port 2>/dev/null | grep -q ata && ok "SATA (ahci) port present" || info "no SATA port (ok if disabled in DT)"

sec "Misc peripherals"
[ -e /dev/rtc0 ] && ok "RTC: $(cat /sys/class/rtc/rtc0/name) time=$(cat /sys/class/rtc/rtc0/date 2>/dev/null) $(cat /sys/class/rtc/rtc0/time 2>/dev/null)" || info "no RTC"
[ -d /sys/class/watchdog/watchdog0 ] && ok "watchdog: $(cat /sys/class/watchdog/watchdog0/identity 2>/dev/null || basename "$(readlink /sys/class/watchdog/watchdog0/device/driver)")" || info "no watchdog"
[ -e /dev/hwrng ] && ok "hwrng present" || info "no hwrng"
ls /sys/class/leds 2>/dev/null | sed 's/^/  [info] led /'

sec "Kernel log"
e=$(dmesg -l err,crit,alert,emerg 2>/dev/null)
[ -z "$e" ] && ok "no err/crit messages in dmesg" || { bad "dmesg errors:"; echo "$e" | sed 's/^/         /'; }
dmesg | grep -iE 'probe.*fail|failed to|deferred' | head -n 15 | sed 's/^/  [warn] /'

sec "systemd"
st=$(systemctl is-system-running 2>/dev/null); [ "$st" = running ] && ok "system state: running" || bad "system state: $st"
systemctl --failed --no-legend 2>/dev/null | sed 's/^/  [FAIL] unit /'
info "boot time: $(systemd-analyze 2>/dev/null | head -n 1)"

sec "Summary"
[ "$FAILS" -eq 0 ] && echo "  all checks passed" || echo "  $FAILS check(s) failed"

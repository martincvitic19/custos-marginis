#!/bin/sh
# Write a new Custos image to the microSD the board is running from, over
# SSH, then reboot into it. Run on the build PC (lan1 / admin VLAN):
#
#   tools/flash-over-ssh.sh [image.wic.bz2] [board-ip]
#
# How it stays safe enough:
#   1. stop the big services (AdGuard Home, Suricata) and unmount /data,
#   2. sync, then sysrq "u": every file system is remounted read-only, so
#      ext4 can no longer write metadata over the new image,
#   3. stream the image into /dev/mmcblk1 (dd reads from the SSH pipe; its
#      code is already in RAM),
#   4. sysrq "b" from the same remote shell: immediate reboot into the new
#      image (a new SSH session would fail: the old programs are gone).
# If anything fails between 2 and 4 the card holds a partial image: write it
# again with bmaptool on the PC (the board then cannot boot until you do).
# /data is overwritten too (new image = fresh /data).
set -eu
IMG=${1:-build/espressobin-wrynose/build/tmp/deploy/images/espressobin/custos-image-espressobin.rootfs.wic.bz2}
B=${2:-192.168.20.1}
KEY=${SSH_KEY:-keys/espressobin_ed25519}
DEV=${DEV:-/dev/mmcblk1}
SSH="ssh -i $KEY -o BatchMode=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ServerAliveInterval=10 root@$B"

[ -r "$IMG" ] || { echo "no image $IMG" >&2; exit 1; }
size=$(bzcat "$IMG" | wc -c)
echo "image: $(readlink -f "$IMG") ($((size / 1048576)) MiB)"
$SSH "test -b $DEV && grep -q '^/dev/root / ' /proc/mounts && cat /sys/block/${DEV#/dev/}/size" >/dev/null \
    || { echo "board $B: $DEV missing or not booted from it" >&2; exit 1; }
card=$($SSH "echo \$((\$(cat /sys/block/${DEV#/dev/}/size) * 512))")
[ "$size" -le "$card" ] || { echo "image larger than the card" >&2; exit 1; }

echo "== preparing the board"
$SSH 'systemctl stop custos-suricata adguardhome custos-watch custosd custos-apply.timer 2>/dev/null; \
      umount /data 2>/dev/null || umount -l /data; sync; echo 1 > /proc/sys/kernel/sysrq; \
      echo s > /proc/sysrq-trigger; sleep 1; echo u > /proc/sysrq-trigger; sleep 1; grep " / " /proc/mounts'

echo "== writing $DEV and rebooting (takes several minutes)"
# Reboot from the same remote shell: after dd nothing on the old root file
# system can be started any more (a new SSH login fails), but this shell
# and its builtins (echo) are already in memory.
bzcat "$IMG" | $SSH "dd of=$DEV bs=1M; sync; echo written; echo b > /proc/sysrq-trigger" || true
echo "done: wait ~2 min, then: ssh -i $KEY root@$B"
echo "if it does not come back: power-cycle the board (the card is fully written)"

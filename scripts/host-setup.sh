#!/bin/bash
# One-time host setup for the ESPRESSObin netboot/build workflow. Run with sudo.
# Idempotent: safe to re-run (e.g. after replugging the USB NIC).
#
#   sudo bash scripts/host-setup.sh
#
# Override defaults via environment: HOST_IFACE, HOST_IP, DISK_UUID, MOUNT_POINT.
set -euo pipefail

HOST_IFACE=${HOST_IFACE:-enxc8a362c00ebd}
HOST_IP=${HOST_IP:-192.168.50.1/24}
DISK_UUID=${DISK_UUID:-ee640056-d929-412b-ac44-ec739f6e527b}
MOUNT_POINT=${MOUNT_POINT:-/mnt/yocto}

[ "$(id -u)" -eq 0 ] || { echo "run with sudo" >&2; exit 1; }

echo "== 1) NetworkManager: leave $HOST_IFACE unmanaged (other interfaces untouched)"
conf=/etc/NetworkManager/conf.d/90-espressobin-unmanaged.conf
if [ -d /etc/NetworkManager/conf.d ]; then
    printf '[keyfile]\nunmanaged-devices+=interface-name:%s\n' "$HOST_IFACE" > "$conf"
    systemctl reload NetworkManager
    sleep 2
fi

echo "== 2) static IP $HOST_IP on $HOST_IFACE"
ip link set "$HOST_IFACE" up
ip addr replace "$HOST_IP" dev "$HOST_IFACE"

echo "== 3) mount UUID=$DISK_UUID at $MOUNT_POINT (no formatting)"
mkdir -p "$MOUNT_POINT"
line="UUID=$DISK_UUID  $MOUNT_POINT  ext4  defaults,nofail,x-systemd.device-timeout=10s  0  2"
if grep -qE "^[^#]*[[:space:]]$MOUNT_POINT[[:space:]]" /etc/fstab; then
    if ! grep -qF "$line" /etc/fstab; then
        cp -n /etc/fstab "/etc/fstab.bak.$(date +%F)"
        sed -i "\|^[^#]*[[:space:]]$MOUNT_POINT[[:space:]]|c\\$line" /etc/fstab
    fi
else
    cp -n /etc/fstab "/etc/fstab.bak.$(date +%F)"
    echo "$line" >> /etc/fstab
fi
systemctl daemon-reload
findmnt "$MOUNT_POINT" >/dev/null || mount "$MOUNT_POINT"

echo "== 4) firewall: allow everything from the isolated board subnet on $HOST_IFACE (TFTP 69/udp, NFS 3048-3049)"
if command -v ufw >/dev/null && ufw status | grep -q 'Status: active'; then
    net=$(ip -4 -o addr show dev "$HOST_IFACE" | awk '{print $4}' | head -1)
    net=$(python3 -c "import ipaddress,sys; print(ipaddress.ip_interface(sys.argv[1]).network)" "$net")
    ufw allow in on "$HOST_IFACE" from "$net"
fi

echo "== result"
ip -br addr show dev "$HOST_IFACE"
grep -n "$MOUNT_POINT" /etc/fstab
findmnt "$MOUNT_POINT"
cat "/run/NetworkManager/devices/$(cat /sys/class/net/$HOST_IFACE/ifindex)" 2>/dev/null || true

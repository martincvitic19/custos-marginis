#!/bin/sh
# S16: Weak Wi-Fi security (RED, EN 18031). See lib.sh.
. "$(dirname "$0")/lib.sh"
scenario S16 "Weak Wi-Fi security (RED, EN 18031)" config <<'PROBE'
ls /sys/class/ieee80211 2>/dev/null | grep -q phy || exit 77
grep -q '^ieee80211w=2' /run/custos/hostapd.conf || exit 1
grep -q '^wpa_key_mgmt=SAE' /run/custos/hostapd.conf || exit 1
PROBE

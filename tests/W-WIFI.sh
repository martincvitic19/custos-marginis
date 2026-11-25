#!/bin/sh
# W-WIFI test, ON the board after plugging in the Compex WLE900VX (QCA9880)
# and its antennas:  sh /tmp/W-WIFI.sh
# Then: phone on "Custos-Djeca" gets 192.168.10.x and child rules; phone on
# "Custos-Doma" gets 192.168.20.x. Passwords: UI -> Postavke -> Wi-Fi, or
#   jq . /data/custos/secrets.json
ok()   { printf '  [ OK ] %s\n' "$*"; }
bad()  { printf '  [FAIL] %s\n' "$*"; FAILS=$((FAILS+1)); }
info() { printf '  [info] %s\n' "$*"; }
sec()  { printf '\n== %s ==\n' "$*"; }
FAILS=0

sec "PCIe and driver"
for d in /sys/bus/pci/devices/*; do
    v=$(cat $d/vendor); p=$(cat $d/device)
    info "$(basename $d) $v:$p $(basename "$(readlink $d/driver 2>/dev/null)")"
done
grep -qx 0x168c /sys/bus/pci/devices/*/vendor && ok "Qualcomm Atheros card on PCIe (vendor 168c)" \
    || bad "no Atheros card: check seating (mPCIe), dmesg | grep -i -e pci -e ath10k"
[ "$(cat /sys/bus/pci/devices/*/device 2>/dev/null | grep -c 0x003c)" -ge 1 ] && ok "QCA988x (device 003c)" || info "device id is not 003c"
dmesg | grep -i ath10k | tail -n 8 | sed 's/^/           /'
dmesg | grep -qi 'ath10k.*firmware.*failed\|ath10k.*could not' && bad "ath10k firmware error (see dmesg)" || ok "no ath10k firmware errors"
ls /sys/class/ieee80211/ 2>/dev/null | grep -q phy && ok "radio: $(ls /sys/class/ieee80211)" || bad "no radio registered"
[ -f /lib/firmware/regulatory.db ] && ok "regulatory.db present" || bad "regulatory.db missing"
iw reg get | head -n 2 | sed 's/^/           /'

sec "Access point"
test -f /run/custos/hostapd.conf && ok "hostapd.conf generated" || bad "no /run/custos/hostapd.conf (custos-apply sees no radio?)"
systemctl is-active --quiet custos-hostapd && ok "custos-hostapd active" || bad "custos-hostapd not active: journalctl -u custos-hostapd"
iw dev | grep -E 'Interface|ssid|type' | sed 's/^/           /'
n=$(iw dev | grep -c 'type AP')
[ "$n" -ge 2 ] && ok "$n AP interfaces (SSIDs)" || bad "$n AP interface(s), want 2"
grep -q '^ieee80211w=2' /run/custos/hostapd.conf && ok "PMF required (802.11w=2)" || bad "PMF not required"
grep -q '^wpa_key_mgmt=SAE' /run/custos/hostapd.conf && ok "WPA3-SAE enabled" || bad "no SAE"
grep -q '^wpa_passphrase=$' /run/custos/hostapd.conf && bad "empty passphrase" || ok "passphrases set (generated, no default)"

sec "SSID -> VLAN"
bridge vlan show | grep -E '^wlan' | sed 's/^/           /'
bridge vlan show dev wlan0 2>/dev/null | grep -q '10 PVID' && ok "wlan0 (Custos-Djeca) -> VLAN 10" || bad "wlan0 not PVID 10"
bridge vlan show dev wlan0-1 2>/dev/null | grep -q '20 PVID' && ok "wlan0-1 (Custos-Doma) -> VLAN 20" || bad "wlan0-1 not PVID 20"

sec "ISP Wi-Fi scan (A9)"
info "custos-watch scans every 30 min; manual scan:"
iw dev wlan0 scan ap-force 2>/dev/null | grep -E '^BSS|SSID:' | head -n 20 | sed 's/^/           /'

sec "Result"; [ $FAILS -eq 0 ] && echo "  PASS" || echo "  $FAILS check(s) FAILED"; exit $FAILS

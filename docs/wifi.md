# Wi-Fi access point (W-WIFI)

Card: **Compex WLE900VX** = Qualcomm Atheros **QCA9880** (802.11ac 3x3,
dual band, mini PCIe). Linux driver `ath10k_pci`, firmware
`/lib/firmware/ath10k/QCA988X/hw2.0/` from `linux-firmware-ath10k-qca988x`.
(The plan named an AR9280/ath9k card; the hardware changed. Everything else
is the same.)

## Hardware

1. Power off. Insert the card into the ESPRESSObin's mini PCIe slot and
   screw it down. The WLE900VX is a full-length card; check that the
   board's slot has a full-length standoff (UNVERIFIED for the V7; if not,
   a half-to-full adapter bracket is needed, as the plan notes).
2. Antennas: connect **at least the main antenna** (connector 1, "MAIN"/
   "1") before powering on; unused connectors are better terminated or
   left with antennas attached. Transmitting without an antenna can damage
   the PA.
3. Boot. The PCIe link is checked by `tests/W-WIFI.sh`.

The card draws up to ~2.5 W at full TX power; the ESPRESSObin's 12 V/2 A
supply is enough. If the card is not detected, re-seat it and check
`dmesg | grep -i -e pci -e ath10k`.

## What happens at boot

```
ath10k_pci loads firmware -> phy0 -> wlan0 appears
custos-apply sees /sys/class/ieee80211/phy0 -> writes /run/custos/hostapd.conf
  and /run/systemd/network/06-custos-wlan*.network
custos-hostapd.service (ConditionPathExists) -> hostapd: two BSS
  wlan0   "Custos-Djeca" -> bridge br-lan, PVID 10 (children)
  wlan0-1 "Custos-Doma"  -> bridge br-lan, PVID 20 (adults)
systemd-networkd puts each into its VLAN; DHCP, forced DNS, roles, curfew
  work exactly as on the wired ports.
```

If the radio is plugged in after boot, run `custos-apply --mode=full` (or
reboot). Without the card nothing Wi-Fi related starts and nothing fails.

## Security

* WPA3-SAE + WPA2 (PSK-SHA256), **PMF required** (`ieee80211w=2`,
  `sae_require_mfp=1`). Old devices without PMF cannot join; that is
  intended (EN 18031 / RED delegated act, scenario S16).
* No default passphrase: 4×5 random characters per SSID, generated on the
  device on first apply, stored in `/data/custos/secrets.json`
  (root:custos 0640), shown in the UI (Postavke → Wi-Fi).
* `ap_isolate=1` on the children's/guest SSIDs: devices there cannot see
  each other.
* Country `HR` (regulatory domain from the signed `regulatory.db`),
  5 GHz channel 36 (no DFS wait), 20/40 MHz.
* The children's SSID is VLAN 10: unknown devices are children anyway,
  and a parent's MAC on VLAN 10 is still a child (MAC + VLAN identity).

## ISP Wi-Fi detection

custos-watch scans every 30 minutes (`iw dev wlan0 scan ap-force`) and
raises `isp_wifi` if it sees an SSID listed in `wifi.isp_ssids` or a BSSID
that differs from the ISP router's MAC only in the last byte. Put the ISP
router's SSID into custos.json:

```
jq '.wifi.isp_ssids = ["UBEE-1234"]' /data/custos/custos.json > /tmp/c && cp /tmp/c /data/custos/custos.json
```

## Test

`tests/W-WIFI.sh` on the board, then a phone on each SSID: Custos-Djeca →
192.168.10.x, blocked lists apply, curfew applies; Custos-Doma →
192.168.20.x.

## UNVERIFIED (first run with the card)

* Two BSS on one QCA9880 radio with the generated BSSIDs (locally
  administered bit + last byte), and SAE with mandatory PMF on the ath10k
  firmware in linux-firmware 20260410. If hostapd refuses: `journalctl -u
  custos-hostapd`; fallback is one SSID (`wifi.ssids` with one entry) or
  `ieee80211w=1` (edit `render/net.go`).

# Anti-bypass (A9)

DNS filtering alone can be bypassed: own DoH server, VPN, a device plugged
into the ISP router, the ISP router's Wi-Fi. Custos sits in the traffic path,
so it can see and limit these. Pi-hole-like DNS filters cannot.

| # | Mechanism | Where | What it catches | Alert |
|---|-----------|-------|-----------------|-------|
| 1 | Per-device accounting: bytes sent to the internet (`acct_tx`) and DNS packets to Custos (`acct_dns`), nft dynamic sets keyed by `MAC . VLAN` with per-element counters | `render/nft.go` (`acct_forward`, `acct_input`) | The data for 1a/1b | - |
| 1a | ≥ `min_bytes` sent and ≤ `max_dns` DNS packets in `window_minutes` | `watch/watch.go` (`Traffic`) | Device resolves names elsewhere: own DoH, VPN over 443 | `bypass`: "X vjerojatno zaobilazi zaštitu" (once per hour per device) |
| 1b | ≥ `upload.max_bytes` sent in the window | same | Exfiltration (S18) | `upload` |
| 2 | Strict egress for children (role or device option): only tcp 80/443, udp 443 (HTTP/3) and udp 123 (NTP); everything else dropped and counted | `render/nft.go` (`child_egress`) | OpenVPN (udp 1194), WireGuard, IPsec, SSH tunnels, Tor on non-web ports | counter `child egress: not web/NTP` |
| 3 | Raw ARP sweep of the ISP router's LAN from wan every `interval_minutes` (works although the firewall drops everything on wan; finds hosts that ignore ping) | `watch/arp.go` (`ARPScan`), `watch.go` (`ISPSide`) | A device connected to the ISP router around Custos | `isp_device` (once per 6 h per MAC) |
| 4 | Wi-Fi scan (`iw dev wlan0 scan ap-force`) every 30 min: a configured ISP SSID, or a BSSID next to the ISP router's MAC | `watch.go` (`ISPWiFi`) | ISP router's Wi-Fi still on | `isp_wifi` |
| 5 | Passive ARP listener on every LAN VLAN: someone else claims the router's address | `watch.go` (`ARPGuard`) | ARP spoofing / man in the middle (A12 L2 part) | `arp_spoof` |

Why "traffic without DNS": a normal device asks DNS before nearly every new
connection; a device that sends megabytes while asking Custos nothing is
using its own resolver or a tunnel. False alarms: a device that only talks to
hard-coded IPs (some IoT, game consoles), a long single download that
started before the window. Hence the window and thresholds (custos.json
`alerts.bypass`), and one alert per hour.

**Deviation from the plan:** the plan proposes tc-BPF for per-MAC
accounting (cilium/ebpf + clang). The build has no clang/meta-clang yet and
the board's nft (1.1.6) supports dynamic sets with per-element counters,
which give the same numbers without a BPF toolchain, loader or bpffs. A BPF
version stays possible for A12 (XDP needs clang anyway).

Kernel hardening for BPF (always on, `91-custos-hardening.conf`):
`kernel.unprivileged_bpf_disabled=1`, `net.core.bpf_jit_harden=2`
(`CONFIG_BPF_UNPRIV_DEFAULT_OFF=y`).

custos-watch runs as user `custoswatch` with only `CAP_NET_RAW` (raw ARP)
and `CAP_NET_ADMIN` (read nft counters, Wi-Fi scan), no root. Alerts go to
`/data/custos/events.jsonl` → UI overview (Pregled) and CSV export.

## Test

`tests/A9-bypass.sh` on the board; live demos are listed in its header.
Already seen on the development line (2026-11-19): the ARP sweep found three
devices on the ISP router's LAN (the developer's laptop over Wi-Fi among
them) and raised `isp_device` alerts.

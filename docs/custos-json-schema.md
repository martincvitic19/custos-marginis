# custos.json

`/data/custos/custos.json` is the only place where the router's policy is
configured. `custos-apply` turns it into everything else; `custosd` (UI/API)
edits it. Full example: [examples/custos.json](examples/custos.json). Code:
`custos/internal/config/config.go` (model, defaults, validation).

Rules that matter:

* **Unknown device = child.** A device that is not in `devices` gets the
  child role: child DNS filtering (AdGuard Home global settings), child
  curfew, child egress rules.
* **A device is MAC + VLAN.** The same MAC on another VLAN is a different,
  unknown device. Role `trusted` is only allowed on the admin VLAN, so a
  child who copies a parent's MAC on VLAN 10 is still a child.
* **Strict validation.** Unknown fields are errors; names, SSIDs, MACs,
  times and URLs are checked against strict patterns before anything is
  written, so generated nft/networkd/hostapd files never need escaping.
* **Atomic writes.** Temporary file + fsync + rename, both for custos.json and
  for every generated file.

| Section | Field | Meaning |
|---------|-------|---------|
| `main` | `timezone` | Curfew times are wall-clock times here (`Europe/Zagreb`); DST is handled by Go's tz database (embedded in the binary). |
| `network` | `bridge`, `wan`, `admin_vlan`, `vlans[]` | Must match the image's network plan (custos-net.inc). VLAN `n` = `192.168.n.0/24`, router `.1`. |
| `filtering` | `safesearch` | Global SafeSearch (child role and unknown devices). |
| | `lists[]` | `name`, `url` (https), `enabled`. Synced to AdGuard Home (added, toggled, removed). |
| | `rules[]` | Extra AdGuard rules. Fixed rules (Firefox canary, iCloud Private Relay) are always added. |
| | `update_interval_hours` | 0, 1, 12, 24, 72 or 168. |
| `roles.<role>` | `filtering`, `safesearch`, `blocked_services[]` | DNS policy of the role (AdGuard Home client settings). |
| | `strict_egress` | Only tcp 80/443, udp 443 and NTP to the internet; everything else (most VPNs) dropped and counted. |
| | `schedules[]` | Curfew: `days` (`sun`..`sat`), `from`, `to` (`HH:MM`). `from > to` runs over midnight and belongs to the day it starts. |
| `devices[]` | `name`, `mac`, `vlan`, `role` | A known device. Gets a DHCP reservation (192.168.vlan.10-99). |
| | `schedules` (optional) | Replaces the role's schedules (`[]` = no curfew). |
| | `strict_egress` (optional) | Overrides the role. |
| | `curfew_override_until` | Set by the "+30 min" button; expires by itself. |
| `vpn` | `enabled`, `port`, `subnet`, `endpoint` | WireGuard server wg0 (router = first address of `subnet`). `endpoint` goes into peer configs. |
| | `exit` | `mullvad`: peers leave through wg-mullvad, kill switch (no Mullvad = no internet, never wan). `wan`: home line. |
| | `peers[]` | `name`, `public_key`, `address`, `role`. Private keys are never stored. |
| `wifi` | `enabled`, `country`, `band` (`a`/`g`), `channel` | hostapd. |
| | `ssids[]` | `name`, `vlan`. Passphrases are generated on first apply (`/data/custos/secrets.json`), never defaults. |
| | `isp_ssids[]` | The ISP router's network names; seeing one on the air raises an alert. |
| `alerts` | `bypass` | `window_minutes`, `min_bytes`, `max_dns`: traffic above `min_bytes` with at most `max_dns` DNS packets in the window = probable bypass. |
| | `upload` | `window_minutes`, `max_bytes`: large upload alert (S18). |
| | `isp_watch` | `enabled`, `interval_minutes`, `ignore_macs[]`: ARP scan of the ISP router's LAN. |

## Files custos-apply derives (never edit)

| File | From |
|------|------|
| `/run/custos/firewall/50-custos-policy.nft` | roles, devices, curfew now, VPN (one `nft -f` transaction; included by the base ruleset on reload) |
| `/run/systemd/network/05-custos-br-lan.<n>.network.d/50-custos-leases.conf` | DHCP reservations |
| `/run/systemd/network/30-custos-wg0.*`, `31-custos-wg-mullvad.*` | VPN |
| `/run/systemd/network/06-custos-wlan*.network`, `/run/custos/hostapd.conf` | Wi-Fi |
| `/data/custos/state.json` | stable DHCP reservations |
| `/data/custos/secrets.json` | Wi-Fi passphrases (0640 root:custos) |
| `/data/custos/wg/wg0.key`, `wg0.pub`, `mullvad.key` | WireGuard keys (0640 root:systemd-network) |
| `/run/custos/apply.json` | status of the last run |
| `/data/custos/events.jsonl` | event log (apply changes, alerts), 30 days |

AdGuard Home is changed through its API (127.0.0.1:3000): filters, user
rules, SafeSearch, blocked services, and persistent clients named
`custos: <device>` (Custos never touches clients without that prefix).

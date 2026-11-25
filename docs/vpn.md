# VPN: WireGuard in, Mullvad out (A7)

A child's phone on mobile data tunnels home into Custos (wg0), gets the same
filtering, curfew and role as at home, and reaches the internet through
Mullvad (wg-mullvad). The home LAN never uses Mullvad.

```
phone (mobile data) --WireGuard--> ISP router :51820 --> Custos wan
   wg0 192.168.40.0/24 -- DNS: forced to AdGuard Home (same as the LAN)
       |  ip rule: iif wg0 lookup 100 (forwarded peer traffic only)
       v
   table 100: default dev wg-mullvad --> Mullvad --> internet
   (no wg-mullvad -> table 100 empty -> nft kill switch drops; never wan)
LAN (br-lan.*) --> main table --> wan (unchanged)
```

## Set up

1. **CGNAT?** On the board: `tools/cgnat-check.sh` (or UI → Postavke →
   Provjeri CGNAT). It asks the ISP router over UPnP for its WAN address and
   compares it with the public address seen from the internet.
   * `public IPv4`: on the ISP router forward **UDP 51820 → 192.168.0.15**
     (or DMZ to 192.168.0.15). Put the public IP or a DynDNS name into
     `vpn.endpoint` (UI → Postavke → VPN).
   * `CGNAT detected`: inbound connections cannot reach home. Relay variant:
     a small VPS runs WireGuard; Custos connects *out* to it (persistent
     keepalive) and the phones connect to the VPS, which forwards to Custos
     inside its tunnel. Not automated yet.
2. **Mullvad** (optional; without it set `vpn.exit` to `wan`): in the
   Mullvad account create a WireGuard configuration (Linux, one server, IPv4)
   and copy the file to the board as `/data/custos/wg/mullvad.conf`
   (`chmod 600`). `custos-apply` extracts the key into
   `/data/custos/wg/mullvad.key` and creates wg-mullvad. The key is never in
   the image or the repository.
3. **Peer**: UI → Postavke → "Novi VPN uređaj" (QR code), or on the board
   `custos-apply add-peer "Ivin mobitel" child` (QR in the terminal). The
   phone's private key is shown once and not stored.
4. **Phone**: WireGuard app → scan. Android: Settings → VPN → WireGuard →
   *Always-on VPN* + *Block connections without VPN*. iOS needs MDM for an
   always-on VPN that the child cannot switch off.

## Where in the code

| What | File |
|------|------|
| wg0 / wg-mullvad netdevs, policy rule, table 100 | `custos/internal/render/net.go` (`VPN`) |
| Kill switch, wan port, NAT behind Mullvad | `custos/internal/render/nft.go` (`roles`, `policy_input`, `policy_snat`) |
| Peer key pair, config | `custos/internal/apply/peer.go`, `custos/internal/wgkeys/` |
| QR code | `custos/internal/qrpng/` |
| CGNAT check | `custos/internal/cgnat/`, `tools/cgnat-check.sh` |
| Kernel | `CONFIG_WIREGUARD=m`, `CONFIG_IP_MULTIPLE_TABLES=y` in `custos-router.cfg` |

## Test

`tests/A7-vpn.sh` on the board. With a phone on mobile data: ifconfig.me shows
the Mullvad address (or the home address with `exit: wan`); a blocked site is
blocked; `ip link set wg-mullvad down` → the phone has no internet (kill
switch), but DNS answers and the UI is reachable through the tunnel.

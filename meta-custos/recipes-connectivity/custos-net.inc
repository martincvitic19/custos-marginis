# Custos edge router network plan, shared by custos-network and
# custos-firewall so both always agree on interface names and VLANs.
# Every value can be overridden from local.conf.
#
# Interface names come from the ESPRESSObin device tree
# (armada-3720-espressobin-v7-emmc.dtb): the conduit is the mvneta
# ethernet@30000 (eth0); the 88E6341 switch user ports are labelled
# lan1 (port 1), lan0 (port 2) and wan (port 3).

CUSTOS_CONDUIT_IF ?= "eth0"

# WAN: static address on the ISP router's LAN (double NAT).
CUSTOS_WAN_IF ?= "wan"
CUSTOS_WAN_ADDRESS ?= "192.168.0.15/24"
CUSTOS_WAN_GATEWAY ?= "192.168.0.1"
# DNS for the router's own lookups (NTP, block list downloads, ...); clients
# always use AdGuard Home. Quad9: the ISP router's DNS proxy is not reliable
# (it does not answer at all on the development line, 2026-11-19).
CUSTOS_WAN_DNS ?= "9.9.9.9 149.112.112.112"
# CAKE shaper rate for wan egress, e.g. "95M" (about 90-95 % of the real
# uplink). Empty: CAKE runs unshaped (flow fairness only).
CUSTOS_WAN_CAKE_BANDWIDTH ?= ""

# LAN: one VLAN-aware bridge over the switch ports. VLAN <n> uses
# 192.168.<n>.0/24 with the router at 192.168.<n>.1, so IDs must be 1-254.
CUSTOS_BRIDGE ?= "br-lan"
CUSTOS_VLANS ?= "10 20 30"
# VLAN that may reach the router's SSH and web UI.
CUSTOS_ADMIN_VLAN ?= "20"
# DNS server handed out by DHCP. Empty: each VLAN gets its own gateway
# address (192.168.<n>.1), where AdGuard Home answers. A client that ignores
# it is redirected there anyway (custos-firewall, 10-dns-enforce.nft).
CUSTOS_LAN_DNS ?= ""

# WireGuard interface for remote clients. It does not exist yet; the DNS
# enforcement rules already match it by name, so its DNS cannot bypass
# AdGuard Home once it is added.
CUSTOS_WG_IF ?= "wg0"

# Bridge ports as <port>:<untagged PVID>:<tagged VLANs, comma separated>.
# lan0: user VLAN 10 untagged, guest VLAN 30 tagged (VLAN-aware AP).
# lan1: admin VLAN 20 untagged only. The admin VLAN is deliberately not
#       carried on lan0, so a device there cannot tag its way into it.
CUSTOS_LAN_PORTS ?= "lan0:10:30 lan1:20:"

#!/bin/sh
# Public IPv4 or CGNAT? Run on the board. Asks the ISP router over UPnP for
# its WAN address and compares it with the address seen from the internet.
# Exit 0: public IPv4 (forward UDP 51820 to 192.168.0.15), 2: CGNAT (use a
# relay, docs/vpn.md), 1: unknown.
exec custos-apply cgnat-check

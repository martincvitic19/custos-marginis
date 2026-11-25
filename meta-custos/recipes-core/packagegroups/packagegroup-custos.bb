SUMMARY = "Custos application packages"
DESCRIPTION = "Application packages for Custos Marginis"

inherit packagegroup

RDEPENDS:${PN} = " \
    custos-network \
    custos-firewall \
    custos-firewall-doh \
    adguardhome \
    custos-apply \
    custosd \
    custos-watch \
    nftables \
    conntrack-tools \
    iproute2-ip \
    iproute2-bridge \
    iproute2-tc \
    jq \
    packagegroup-custos-vpn \
"

PACKAGES += "packagegroup-custos-vpn"

# A7: WireGuard server (wg0) and Mullvad client (wg-mullvad).
RDEPENDS:packagegroup-custos-vpn = "wireguard-tools"
RRECOMMENDS:packagegroup-custos-vpn = "kernel-module-wireguard"

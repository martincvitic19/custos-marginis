SUMMARY = "Custos application packages"
DESCRIPTION = "Application packages for Custos Marginis"

inherit packagegroup

RDEPENDS:${PN} = " \
    custos-network \
    custos-firewall \
    nftables \
    iproute2-ip \
    iproute2-bridge \
    iproute2-tc \
"

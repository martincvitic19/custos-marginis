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
    packagegroup-custos-security \
"

PACKAGES += "packagegroup-custos-vpn packagegroup-custos-security"

# A10-A12: SELinux (permissive until the AVC log is clean), audit, IDS
# (Suricata, sshguard, arpwatch, AIDE) and the XDP edge filter. procps: ps -Z
# (process domains) and ps -o; the busybox ps in this distro has neither.
RDEPENDS:packagegroup-custos-security = " \
    ${@bb.utils.contains('DISTRO_FEATURES', 'selinux', 'packagegroup-selinux-minimal policycoreutils-loadpolicy', '', d)} \
    auditd \
    custos-ids \
    procps \
    ${CUSTOS_SURICATA} \
    ${CUSTOS_XDP} \
"
# Suricata IDS (needs Rust/LLVM to build). CUSTOS_SURICATA = "" leaves it out.
CUSTOS_SURICATA ?= "custos-ids-suricata"
# XDP edge filter (needs clang from meta-clang). CUSTOS_XDP = "" leaves it
# out, e.g. while clang-native is still building.
CUSTOS_XDP ?= "custos-bpf"

# A7: WireGuard server (wg0) and Mullvad client (wg-mullvad).
RDEPENDS:packagegroup-custos-vpn = "wireguard-tools"
RRECOMMENDS:packagegroup-custos-vpn = "kernel-module-wireguard"

SUMMARY = "nftables firewall and routing sysctls for the Custos edge router"
DESCRIPTION = "inet table 'custos': default-drop input and forward, LAN \
VLANs to wan only, management only from the admin VLAN, masquerade on wan, \
counters on every drop. Loaded before networking starts; networkd and the \
SSH socket refuse to start without it."
LICENSE = "MIT"
LIC_FILES_CHKSUM = "file://${COMMON_LICENSE_DIR}/MIT;md5=0835ade698e0bcf8506ecda2f7b4f302"

require recipes-connectivity/custos-net.inc

SRC_URI = " \
    file://custos-firewall.nft.in \
    file://custos-firewall.service \
    file://90-custos-router.conf \
    file://networkd-requires-firewall.conf \
    file://dropbear-requires-firewall.conf \
"

S = "${UNPACKDIR}"

INHIBIT_DEFAULT_DEPS = "1"

inherit systemd

SYSTEMD_SERVICE:${PN} = "custos-firewall.service"
SYSTEMD_AUTO_ENABLE:${PN} = "enable"

do_install[vardeps] += "CUSTOS_WAN_IF CUSTOS_BRIDGE CUSTOS_VLANS CUSTOS_ADMIN_VLAN"

do_install() {
    # "br-lan.10", "br-lan.20", ... for the nft LAN_IFS set
    lan_ifs=""
    for id in ${CUSTOS_VLANS}; do
        lan_ifs="$lan_ifs \"${CUSTOS_BRIDGE}.$id\","
    done
    lan_ifs=$(echo "$lan_ifs" | sed -e 's/^ //' -e 's/,$//')

    install -d ${D}${sysconfdir}/custos
    sed -e 's,@WAN_IF@,${CUSTOS_WAN_IF},g' \
        -e "s|@LAN_IFS@|$lan_ifs|g" \
        -e 's,@ADMIN_IF@,${CUSTOS_BRIDGE}.${CUSTOS_ADMIN_VLAN},g' \
        ${UNPACKDIR}/custos-firewall.nft.in > ${D}${sysconfdir}/custos/firewall.nft
    chmod 0600 ${D}${sysconfdir}/custos/firewall.nft

    install -d ${D}${systemd_system_unitdir}
    install -m 0644 ${UNPACKDIR}/custos-firewall.service ${D}${systemd_system_unitdir}/

    install -d ${D}${systemd_system_unitdir}/systemd-networkd.service.d
    install -m 0644 ${UNPACKDIR}/networkd-requires-firewall.conf \
        ${D}${systemd_system_unitdir}/systemd-networkd.service.d/50-custos-firewall.conf
    install -d ${D}${systemd_system_unitdir}/dropbear.socket.d
    install -m 0644 ${UNPACKDIR}/dropbear-requires-firewall.conf \
        ${D}${systemd_system_unitdir}/dropbear.socket.d/50-custos-firewall.conf

    install -d ${D}${nonarch_libdir}/sysctl.d
    install -m 0644 ${UNPACKDIR}/90-custos-router.conf ${D}${nonarch_libdir}/sysctl.d/
}

FILES:${PN} = " \
    ${sysconfdir}/custos \
    ${systemd_system_unitdir} \
    ${nonarch_libdir}/sysctl.d \
"
CONFFILES:${PN} = "${sysconfdir}/custos/firewall.nft"

RDEPENDS:${PN} = "nftables"
# Loaded on demand by nft; listed so they are in the image even without the
# local.conf "kernel-modules" catch-all.
RRECOMMENDS:${PN} = " \
    kernel-module-nf-conntrack \
    kernel-module-nf-nat \
    kernel-module-nft-ct \
    kernel-module-nft-nat \
    kernel-module-nft-chain-nat \
    kernel-module-nft-masq \
    kernel-module-nft-limit \
"

SUMMARY = "nftables firewall and routing sysctls for the Custos edge router"
DESCRIPTION = "inet table 'custos': default-drop input and forward, LAN \
VLANs to wan only, management only from the admin VLAN, masquerade on wan, \
counters on every drop. Loaded before networking starts; networkd and the \
SSH socket refuse to start without it. DNS enforcement fragment: all client \
DNS redirected to AdGuard Home, DoT/DoQ and known DoH resolvers rejected."
# The rules are MIT. The DoH address set (${PN}-doh) is generated from the
# HaGeZi DNS blocklists, which are GPL-3.0.
LICENSE = "MIT & GPL-3.0-only"
LICENSE:${PN} = "MIT"
LICENSE:${PN}-doh = "GPL-3.0-only"
LIC_FILES_CHKSUM = " \
    file://${COMMON_LICENSE_DIR}/MIT;md5=0835ade698e0bcf8506ecda2f7b4f302 \
    file://hagezi-dns-blocklists-LICENSE;md5=1ebbd3e34237af26da5dc08a4e440464 \
"

require recipes-connectivity/custos-net.inc

# HaGeZi DNS blocklists, pinned to one commit: ips/doh.txt is the list of
# public DoH resolver IPv4 addresses (one address per line). Bump both the
# revision and the checksums together.
HAGEZI_REV = "4e76a04fe84e33cba7ac0b71e21f7c1442350f9d"
HAGEZI_RAW = "https://raw.githubusercontent.com/hagezi/dns-blocklists/${HAGEZI_REV}"

SRC_URI = " \
    file://custos-firewall.nft.in \
    file://10-dns-enforce.nft.in \
    ${HAGEZI_RAW}/ips/doh.txt;name=doh;downloadfilename=hagezi-doh-${HAGEZI_REV}.txt \
    ${HAGEZI_RAW}/LICENSE;name=hagezi-license;downloadfilename=hagezi-dns-blocklists-LICENSE \
    file://custos-firewall.service \
    file://90-custos-router.conf \
    file://networkd-requires-firewall.conf \
    file://dropbear-requires-firewall.conf \
"

SRC_URI[doh.sha256sum] = "85def4c8f7dfce017447fb254c6e9ba38804a6b8f6bcc529804b83dccab4b81e"
SRC_URI[hagezi-license.sha256sum] = "3972dc9744f6499f0f9b2dbf76696f2ae7ad8af9b23dde66d6af86c9dfb36986"

S = "${UNPACKDIR}"

INHIBIT_DEFAULT_DEPS = "1"

inherit systemd

SYSTEMD_SERVICE:${PN} = "custos-firewall.service"
SYSTEMD_AUTO_ENABLE:${PN} = "enable"

do_install[vardeps] += "CUSTOS_WAN_IF CUSTOS_BRIDGE CUSTOS_VLANS CUSTOS_ADMIN_VLAN CUSTOS_WG_IF"

do_install() {
    # "br-lan.10", "br-lan.20", ... for the nft LAN_IFS set
    lan_ifs=""
    for id in ${CUSTOS_VLANS}; do
        lan_ifs="$lan_ifs \"${CUSTOS_BRIDGE}.$id\","
    done
    lan_ifs=$(echo "$lan_ifs" | sed -e 's/^ //' -e 's/,$//')
    dns_ifs="$lan_ifs, \"${CUSTOS_WG_IF}\""

    install -d ${D}${sysconfdir}/custos/firewall.d
    sed -e 's,@WAN_IF@,${CUSTOS_WAN_IF},g' \
        -e "s|@LAN_IFS@|$lan_ifs|g" \
        -e "s|@DNS_IFS@|$dns_ifs|g" \
        -e 's,@ADMIN_IF@,${CUSTOS_BRIDGE}.${CUSTOS_ADMIN_VLAN},g' \
        ${UNPACKDIR}/custos-firewall.nft.in > ${D}${sysconfdir}/custos/firewall.nft
    sed -e "s|@DNS_IFS@|$dns_ifs|g" \
        ${UNPACKDIR}/10-dns-enforce.nft.in > ${D}${sysconfdir}/custos/firewall.d/10-dns-enforce.nft

    # doh_v4 elements: IPv4 addresses/prefixes only, comments and anything
    # else dropped, de-duplicated. An empty list must not produce an empty
    # (invalid) element block.
    doh=${D}${sysconfdir}/custos/firewall.d/11-doh-v4.nft
    addrs=$(sed -e 's/#.*//' -e 's/[[:space:]]//g' ${UNPACKDIR}/hagezi-doh-${HAGEZI_REV}.txt |
        grep -E '^[0-9]{1,3}(\.[0-9]{1,3}){3}(/[0-9]{1,2})?$' | sort -u)
    count=$(printf '%s\n' "$addrs" | grep -c . || true)
    [ "$count" -gt 0 ] || bbfatal "no IPv4 addresses in the HaGeZi DoH list"
    {
        echo "# Public DoH resolver IPv4 addresses for the doh_v4 set ($count entries)."
        echo "# Generated at build time from HaGeZi dns-blocklists ips/doh.txt"
        echo "# @ ${HAGEZI_REV} (GPL-3.0). custos-apply may replace this file."
        echo "add element inet custos doh_v4 {"
        printf '%s\n' "$addrs" | sed -e 's/^/\t/' -e '$!s/$/,/'
        echo "}"
    } > $doh
    chmod 0600 ${D}${sysconfdir}/custos/firewall.nft ${D}${sysconfdir}/custos/firewall.d/*.nft

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

PACKAGES =+ "${PN}-doh"
FILES:${PN}-doh = "${sysconfdir}/custos/firewall.d/11-doh-v4.nft"
CONFFILES:${PN}-doh = "${sysconfdir}/custos/firewall.d/11-doh-v4.nft"

FILES:${PN} = " \
    ${sysconfdir}/custos \
    ${systemd_system_unitdir} \
    ${nonarch_libdir}/sysctl.d \
"
CONFFILES:${PN} = " \
    ${sysconfdir}/custos/firewall.nft \
    ${sysconfdir}/custos/firewall.d/10-dns-enforce.nft \
"

RDEPENDS:${PN} = "nftables"
# Without it the doh_v4 set is empty and DoH to well-known resolvers works.
# Loaded on demand by nft; listed so they are in the image even without the
# local.conf "kernel-modules" catch-all.
# The DoH set: without it doh_v4 stays empty and DoH to well-known
# resolvers is not blocked.
RRECOMMENDS:${PN} = " \
    ${PN}-doh \
    kernel-module-nf-conntrack \
    kernel-module-nf-nat \
    kernel-module-nft-ct \
    kernel-module-nft-nat \
    kernel-module-nft-chain-nat \
    kernel-module-nft-masq \
    kernel-module-nft-limit \
    kernel-module-nft-redir \
    kernel-module-nft-reject \
    kernel-module-nft-reject-inet \
    kernel-module-nf-reject-ipv4 \
    kernel-module-nf-reject-ipv6 \
"

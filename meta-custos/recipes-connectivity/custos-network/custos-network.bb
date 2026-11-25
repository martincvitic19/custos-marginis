SUMMARY = "systemd-networkd configuration for the Custos edge router"
DESCRIPTION = "DSA conduit up without an address, static wan with CAKE, a \
VLAN-aware bridge over the LAN switch ports and one gateway interface with \
a DHCP server per VLAN. The network plan lives in custos-net.inc."
LICENSE = "MIT"
LIC_FILES_CHKSUM = "file://${COMMON_LICENSE_DIR}/MIT;md5=0835ade698e0bcf8506ecda2f7b4f302"

require recipes-connectivity/custos-net.inc

SRC_URI = " \
    file://01-custos-conduit.network.in \
    file://02-custos-wan.network.in \
    file://03-custos-port.network.in \
    file://04-custos-bridge.netdev.in \
    file://04-custos-bridge.network.in \
    file://05-custos-vlan.netdev.in \
    file://05-custos-vlan.network.in \
"

S = "${UNPACKDIR}"

INHIBIT_DEFAULT_DEPS = "1"

# The plan is plain variables, so a changed value must rebuild the files.
do_install[vardeps] += " \
    CUSTOS_CONDUIT_IF CUSTOS_WAN_IF CUSTOS_WAN_ADDRESS CUSTOS_WAN_GATEWAY \
    CUSTOS_WAN_DNS CUSTOS_WAN_CAKE_BANDWIDTH CUSTOS_BRIDGE CUSTOS_VLANS \
    CUSTOS_LAN_DNS CUSTOS_LAN_PORTS CUSTOS_ADMIN_VLAN \
"

# Units go to the vendor directory; /etc/systemd/network stays free for
# local overrides. The 0x- prefixes sort before the ESPRESSObin development
# files (10-/20-/30-*.network from espressobin-network), so networkd always
# picks these first if both are ever installed.
do_install() {
    netdir=${D}${systemd_unitdir}/network
    install -d $netdir

    sed -e 's,@CONDUIT@,${CUSTOS_CONDUIT_IF},g' \
        ${UNPACKDIR}/01-custos-conduit.network.in > $netdir/01-custos-conduit.network

    wan=$netdir/02-custos-wan.network
    sed -e 's,@WAN_IF@,${CUSTOS_WAN_IF},g' \
        -e 's,@WAN_ADDRESS@,${CUSTOS_WAN_ADDRESS},g' \
        -e 's,@WAN_GATEWAY@,${CUSTOS_WAN_GATEWAY},g' \
        -e 's,@WAN_DNS@,${CUSTOS_WAN_DNS},g' \
        ${UNPACKDIR}/02-custos-wan.network.in > $wan
    # [CAKE] is the last section of the template
    if [ -n "${CUSTOS_WAN_CAKE_BANDWIDTH}" ]; then
        echo "Bandwidth=${CUSTOS_WAN_CAKE_BANDWIDTH}" >> $wan
    fi

    for spec in ${CUSTOS_LAN_PORTS}; do
        port=$(echo $spec | cut -d: -f1)
        pvid=$(echo $spec | cut -d: -f2)
        tagged=$(echo $spec | cut -d: -f3 | tr ',' ' ')
        f=$netdir/03-custos-port-$port.network
        sed -e "s,@PORT@,$port,g" \
            -e 's,@BRIDGE@,${CUSTOS_BRIDGE},g' \
            -e "s,@PVID@,$pvid,g" \
            ${UNPACKDIR}/03-custos-port.network.in > $f
        # [BridgeVLAN] is the last section of the template
        for v in $tagged; do
            echo "VLAN=$v" >> $f
        done
    done

    sed -e 's,@BRIDGE@,${CUSTOS_BRIDGE},g' \
        ${UNPACKDIR}/04-custos-bridge.netdev.in > $netdir/04-custos-${CUSTOS_BRIDGE}.netdev
    br=$netdir/04-custos-${CUSTOS_BRIDGE}.network
    sed -e 's,@BRIDGE@,${CUSTOS_BRIDGE},g' \
        ${UNPACKDIR}/04-custos-bridge.network.in > $br
    # [Network] is the last section of the template: VLAN uppers first,
    # then the bridge's own (tagged) VLAN membership.
    for id in ${CUSTOS_VLANS}; do
        echo "VLAN=${CUSTOS_BRIDGE}.$id" >> $br
    done
    printf '\n[BridgeVLAN]\n' >> $br
    for id in ${CUSTOS_VLANS}; do
        echo "VLAN=$id" >> $br
    done

    for id in ${CUSTOS_VLANS}; do
        name=${CUSTOS_BRIDGE}.$id
        sed -e "s,@NAME@,$name,g" -e "s,@ID@,$id,g" \
            ${UNPACKDIR}/05-custos-vlan.netdev.in > $netdir/05-custos-$name.netdev
        dns="${CUSTOS_LAN_DNS}"
        [ -n "$dns" ] || dns=192.168.$id.1
        sed -e "s,@NAME@,$name,g" \
            -e "s,@ADDRESS@,192.168.$id.1/24,g" \
            -e "s,@DNS@,$dns,g" \
            ${UNPACKDIR}/05-custos-vlan.network.in > $netdir/05-custos-$name.network
        # custos.local (resolved mDNS responder) only on the admin VLAN.
        if [ "$id" = "${CUSTOS_ADMIN_VLAN}" ]; then
            sed -i '/^DHCPServer=yes$/a MulticastDNS=yes' $netdir/05-custos-$name.network
        fi
    done
}

FILES:${PN} = "${systemd_unitdir}/network"
RDEPENDS:${PN} = "systemd-networkd"
# The kernel loads sch_cake on demand when networkd installs the qdisc.
RRECOMMENDS:${PN} = "kernel-module-sch-cake"

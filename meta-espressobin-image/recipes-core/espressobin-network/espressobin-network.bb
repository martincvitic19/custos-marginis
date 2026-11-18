SUMMARY = "systemd-networkd configuration for the ESPRESSObin DSA switch"
DESCRIPTION = "Brings up the DSA conduit eth0 without an address, gives the \
management port a static address and brings the other switch ports up \
without an address."
LICENSE = "MIT"
LIC_FILES_CHKSUM = "file://${COMMON_LICENSE_DIR}/MIT;md5=0835ade698e0bcf8506ecda2f7b4f302"

# Set in local.conf
ESPRESSOBIN_BOARD_PORT ?= "lan0"
ESPRESSOBIN_BOARD_IP ?= "192.168.50.2/24"

SRC_URI = " \
    file://10-dsa-conduit.network \
    file://20-mgmt-port.network.in \
    file://30-dsa-ports.network \
"

S = "${UNPACKDIR}"

INHIBIT_DEFAULT_DEPS = "1"

do_install() {
    install -d ${D}${sysconfdir}/systemd/network
    install -m 0644 ${UNPACKDIR}/10-dsa-conduit.network ${D}${sysconfdir}/systemd/network/
    sed -e 's,@PORT@,${ESPRESSOBIN_BOARD_PORT},g' \
        -e 's,@ADDRESS@,${ESPRESSOBIN_BOARD_IP},g' \
        ${UNPACKDIR}/20-mgmt-port.network.in \
        > ${D}${sysconfdir}/systemd/network/20-mgmt-port.network
    install -m 0644 ${UNPACKDIR}/30-dsa-ports.network ${D}${sysconfdir}/systemd/network/
}

FILES:${PN} = "${sysconfdir}/systemd/network"
RDEPENDS:${PN} = "systemd"

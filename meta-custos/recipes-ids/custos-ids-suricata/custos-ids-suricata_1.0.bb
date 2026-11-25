SUMMARY = "Custos Suricata IDS on the LAN bridge (A12)"
DESCRIPTION = "Suricata as IDS (never inline) on br-lan with a small Custos \
rule set, alerts and anomalies only (no browsing history on disk), memory \
limited for the 1 GB board, 14 days of alerts on /data."
LICENSE = "MIT"
LIC_FILES_CHKSUM = "file://${COMMON_LICENSE_DIR}/MIT;md5=0835ade698e0bcf8506ecda2f7b4f302"

SRC_URI = " \
    file://custos-suricata.yaml \
    file://custos.rules \
    file://custos-suricata.service \
"
S = "${UNPACKDIR}"
INHIBIT_DEFAULT_DEPS = "1"

inherit systemd allarch

SYSTEMD_SERVICE:${PN} = "custos-suricata.service"

do_install() {
    install -d ${D}${sysconfdir}/custos/suricata ${D}${systemd_system_unitdir}
    install -m 0644 ${UNPACKDIR}/custos-suricata.yaml ${UNPACKDIR}/custos.rules ${D}${sysconfdir}/custos/suricata/
    install -m 0644 ${UNPACKDIR}/custos-suricata.service ${D}${systemd_system_unitdir}/
}

FILES:${PN} = "${sysconfdir}/custos/suricata ${systemd_system_unitdir}"
CONFFILES:${PN} = "${sysconfdir}/custos/suricata/custos-suricata.yaml ${sysconfdir}/custos/suricata/custos.rules"
# Needs custos-ids for /data/suricata (tmpfiles).
RDEPENDS:${PN} = "suricata custos-ids"

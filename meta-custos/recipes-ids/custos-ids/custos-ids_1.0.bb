SUMMARY = "Custos host detection and integrity (A12): sshguard, arpwatch, AIDE, audit rules"
DESCRIPTION = "Configuration and units that turn the meta-security / meta-oe \
packages into the Custos detection layer: Suricata IDS on br-lan with a \
small rule set and alert-only logging, sshguard (nft) for dropbear, \
arpwatch per LAN VLAN, AIDE checks every 6 h, audit rules for keys and \
policy. Findings go to the journal and, for AIDE, to the Custos event log."
LICENSE = "MIT"
LIC_FILES_CHKSUM = "file://${COMMON_LICENSE_DIR}/MIT;md5=0835ade698e0bcf8506ecda2f7b4f302"

SRC_URI = " \
    file://sshguard.conf \
    file://custos-sshg-parser \
    file://custos-sshguard.service \
    file://custos-arpwatch@.service \
    file://custos-ids-tmpfiles.conf \
    file://custos-aide.conf \
    file://custos-aide-check \
    file://custos-aide.service \
    file://custos-aide.timer \
    file://custos-audit.rules \
"
S = "${UNPACKDIR}"
INHIBIT_DEFAULT_DEPS = "1"

inherit systemd allarch

# Suricata is a separate recipe (custos-ids-suricata): it needs Rust and
# so LLVM to build, and RDEPENDS of any package here would pull it in.
SYSTEMD_SERVICE:${PN} = " \
    custos-sshguard.service \
    custos-aide.timer \
    custos-arpwatch@10.service \
    custos-arpwatch@20.service \
    custos-arpwatch@30.service \
"
SYSTEMD_AUTO_ENABLE = "enable"

do_install() {
    install -d ${D}${sysconfdir}/custos ${D}${systemd_system_unitdir} \
        ${D}${nonarch_libdir}/tmpfiles.d ${D}${sbindir} ${D}${sysconfdir}/audit/rules.d
    # The sshguard 2.5 wrapper always sources /etc/sshguard.conf (no option
    # to point it elsewhere) and the sshguard package installs none.
    install -m 0644 ${UNPACKDIR}/sshguard.conf ${D}${sysconfdir}/sshguard.conf
    install -m 0755 ${UNPACKDIR}/custos-sshg-parser ${D}${sbindir}/
    install -m 0600 ${UNPACKDIR}/custos-aide.conf ${D}${sysconfdir}/custos/aide.conf
    install -m 0755 ${UNPACKDIR}/custos-aide-check ${D}${sbindir}/
    for u in custos-sshguard.service custos-arpwatch@.service \
             custos-aide.service custos-aide.timer; do
        install -m 0644 ${UNPACKDIR}/$u ${D}${systemd_system_unitdir}/
    done
    install -m 0644 ${UNPACKDIR}/custos-ids-tmpfiles.conf ${D}${nonarch_libdir}/tmpfiles.d/custos-ids.conf
    install -m 0640 ${UNPACKDIR}/custos-audit.rules ${D}${sysconfdir}/audit/rules.d/custos.rules
}

FILES:${PN} = "${sysconfdir} ${systemd_system_unitdir} ${nonarch_libdir}/tmpfiles.d ${sbindir}"
CONFFILES:${PN} = "${sysconfdir}/sshguard.conf"
# jq: custos-aide-check reads the booted bundle version (rauc status json);
# rauc itself is optional (baseline image): without it the database is "factory".
RDEPENDS:${PN} = "sshguard arpwatch aide auditd jq"

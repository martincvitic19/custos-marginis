SUMMARY = "Custos run-time hardening: kernel lockdown (A10), SELinux labels on /data (A11)"
LICENSE = "MIT"
LIC_FILES_CHKSUM = "file://${COMMON_LICENSE_DIR}/MIT;md5=0835ade698e0bcf8506ecda2f7b4f302"

SRC_URI = "file://custos-lockdown.service file://custos-relabel-data.service"
S = "${UNPACKDIR}"
INHIBIT_DEFAULT_DEPS = "1"

inherit systemd allarch

SYSTEMD_SERVICE:${PN} = "custos-lockdown.service custos-relabel-data.service"
SYSTEMD_AUTO_ENABLE:${PN} = "enable"

do_install() {
    install -d ${D}${systemd_system_unitdir}
    install -m 0644 ${UNPACKDIR}/custos-lockdown.service ${UNPACKDIR}/custos-relabel-data.service ${D}${systemd_system_unitdir}/
}
FILES:${PN} = "${systemd_system_unitdir}"
RDEPENDS:${PN} = "${@bb.utils.contains('DISTRO_FEATURES', 'selinux', 'policycoreutils-setfiles', '', d)}"

SUMMARY = "Custos A/B update glue for the ESPRESSObin (A13)"
DESCRIPTION = "RAUC custom bootloader backend (slot file on the boot \
partition, read by boot.scr), early-boot counting with fallback, and a \
health check that marks the booted slot good."
LICENSE = "MIT"
LIC_FILES_CHKSUM = "file://${COMMON_LICENSE_DIR}/MIT;md5=0835ade698e0bcf8506ecda2f7b4f302"
SRC_URI = " \
    file://custos-bootchooser \
    file://custos-health-check \
    file://custos-boot-guard.service \
    file://custos-health.service \
    file://custos-health.timer \
    file://custos-rauc-tmpfiles.conf \
    file://custos-rauc-post-install \
    file://custos-verity-check \
    file://custos-verity-check.service \
    file://custos-timesync-data.service \
"
S = "${UNPACKDIR}"
INHIBIT_DEFAULT_DEPS = "1"
inherit systemd allarch
SYSTEMD_SERVICE:${PN} = "custos-boot-guard.service custos-health.timer custos-verity-check.service custos-timesync-data.service"
do_install() {
    install -d ${D}${nonarch_libdir}/custos ${D}${systemd_system_unitdir}
    install -m 0755 ${UNPACKDIR}/custos-bootchooser ${UNPACKDIR}/custos-health-check \
        ${UNPACKDIR}/custos-rauc-post-install ${UNPACKDIR}/custos-verity-check ${D}${nonarch_libdir}/custos/
    install -m 0644 ${UNPACKDIR}/custos-boot-guard.service ${UNPACKDIR}/custos-health.service \
        ${UNPACKDIR}/custos-health.timer ${UNPACKDIR}/custos-verity-check.service \
        ${UNPACKDIR}/custos-timesync-data.service ${D}${systemd_system_unitdir}/
    install -d ${D}${nonarch_libdir}/tmpfiles.d
    install -m 0644 ${UNPACKDIR}/custos-rauc-tmpfiles.conf ${D}${nonarch_libdir}/tmpfiles.d/custos-rauc.conf
}
FILES:${PN} = "${nonarch_libdir}/custos ${nonarch_libdir}/tmpfiles.d ${systemd_system_unitdir}"
# cryptsetup: veritysetup for custos-verity-check (A10).
RDEPENDS:${PN} = "rauc cryptsetup"

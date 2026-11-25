# A13: one SSH host key for both A/B slots. The key lives on /data, so an
# update (a fresh root file system in the other slot) does not change it and
# clients do not see "host key has changed" after every rauc install
# (seen 2026-11-21 on the first switch to slot B).
FILESEXTRAPATHS:prepend := "${THISDIR}/files:"
SRC_URI += "file://dropbearkey-custos.conf file://dropbear-custos.conf"

do_install:append() {
    install -d ${D}${systemd_system_unitdir}/dropbearkey.service.d \
               ${D}${systemd_system_unitdir}/dropbear@.service.d
    install -m 0644 ${UNPACKDIR}/dropbearkey-custos.conf \
        ${D}${systemd_system_unitdir}/dropbearkey.service.d/50-custos-data-key.conf
    install -m 0644 ${UNPACKDIR}/dropbear-custos.conf \
        ${D}${systemd_system_unitdir}/dropbear@.service.d/50-custos-data-key.conf
}

FILES:${PN} += "${systemd_system_unitdir}/dropbearkey.service.d \
                ${systemd_system_unitdir}/dropbear@.service.d"

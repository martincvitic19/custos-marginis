SUMMARY = "Load ath10k_pci only when the Wi-Fi card answers on PCIe"
DESCRIPTION = "Armada 3720 (Aardvark PCIe): if the mini PCIe card's link \
is not up, the ath10k driver's first MMIO access raises an SError and the \
kernel panics. This blacklists automatic loading and loads the driver from \
a guard service after a successful config space read."
LICENSE = "MIT"
LIC_FILES_CHKSUM = "file://${COMMON_LICENSE_DIR}/MIT;md5=0835ade698e0bcf8506ecda2f7b4f302"

SRC_URI = " \
    file://custos-ath10k.conf \
    file://custos-pcie-guard \
    file://custos-pcie-guard.service \
"
S = "${UNPACKDIR}"
INHIBIT_DEFAULT_DEPS = "1"

inherit systemd allarch

SYSTEMD_SERVICE:${PN} = "custos-pcie-guard.service"
SYSTEMD_AUTO_ENABLE:${PN} = "enable"

do_install() {
    install -d ${D}${nonarch_base_libdir}/modprobe.d ${D}${sbindir} ${D}${systemd_system_unitdir}
    install -m 0644 ${UNPACKDIR}/custos-ath10k.conf ${D}${nonarch_base_libdir}/modprobe.d/
    install -m 0755 ${UNPACKDIR}/custos-pcie-guard ${D}${sbindir}/
    install -m 0644 ${UNPACKDIR}/custos-pcie-guard.service ${D}${systemd_system_unitdir}/
}
FILES:${PN} = "${nonarch_base_libdir}/modprobe.d ${sbindir} ${systemd_system_unitdir}"
RDEPENDS:${PN} = "kmod"

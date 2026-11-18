SUMMARY = "U-Boot boot script for the ESPRESSObin factory U-Boot"
DESCRIPTION = "Legacy-format boot.scr sourced by the factory SPI-NOR U-Boot \
from the first (ext4) partition of the microSD card."
LICENSE = "MIT"
LIC_FILES_CHKSUM = "file://${COMMON_LICENSE_DIR}/MIT;md5=0835ade698e0bcf8506ecda2f7b4f302"

COMPATIBLE_MACHINE = "espressobin"

DEPENDS = "u-boot-tools-native"

SRC_URI = "file://boot.cmd.in"

S = "${UNPACKDIR}"

inherit deploy nopackages

PACKAGE_ARCH = "${MACHINE_ARCH}"

do_compile() {
    sed -e 's,@KERNEL_IMAGETYPE@,${KERNEL_IMAGETYPE},g' \
        -e 's,@DTB@,${ESPRESSOBIN_DTB},g' \
        ${UNPACKDIR}/boot.cmd.in > ${B}/boot.cmd
    mkimage -A arm64 -O linux -T script -C none -n "ESPRESSObin boot script" \
        -d ${B}/boot.cmd ${B}/boot.scr
}

do_deploy() {
    install -m 0644 ${B}/boot.scr ${DEPLOYDIR}/boot.scr
}
addtask deploy after do_compile before do_build

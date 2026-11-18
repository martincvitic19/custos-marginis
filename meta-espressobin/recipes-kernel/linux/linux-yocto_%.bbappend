FILESEXTRAPATHS:prepend := "${THISDIR}/files:"

# Start from the in-tree arm64 defconfig and add only what the ESPRESSObin
# needs on top of it.
COMPATIBLE_MACHINE:espressobin = "espressobin"
KMACHINE:espressobin = "espressobin"
KBUILD_DEFCONFIG:espressobin = "defconfig"
KCONFIG_MODE:espressobin = "alldefconfig"

SRC_URI:append:espressobin = " file://espressobin.cfg"

FILESEXTRAPATHS:prepend := "${THISDIR}/files:"

# Kernel options the Custos edge router needs on top of the ESPRESSObin BSP
# fragment (meta-espressobin/recipes-kernel/linux/files/espressobin.cfg).
SRC_URI:append:espressobin = " file://custos-router.cfg"

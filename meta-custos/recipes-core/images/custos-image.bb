SUMMARY = "Secure edge gateway image for Custos Marginis"
DESCRIPTION = "Complete software image for Custos Marginis."

IMAGE_INSTALL = "${CORE_IMAGE_BASE_INSTALL}"

inherit core-image

CORE_IMAGE_EXTRA_INSTALL += "packagegroup-custos"

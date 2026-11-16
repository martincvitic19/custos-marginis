SUMMARY = "Secure edge gateway image for Custos Marginis"
DESCRIPTION = "Complete software image for Custos Marginis."

require recipes-core/images/core-image-minimal.bb

CORE_IMAGE_EXTRA_INSTALL += "packagegroup-custos"

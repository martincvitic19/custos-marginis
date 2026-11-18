SUMMARY = "Secure edge gateway image for Custos Marginis"
DESCRIPTION = "Complete software image for Custos Marginis."

IMAGE_INSTALL = "${CORE_IMAGE_BASE_INSTALL}"

inherit core-image

CORE_IMAGE_EXTRA_INSTALL += "packagegroup-custos"

# custos-network owns the switch ports. The development network setup that
# local.conf adds to every image (lan0 = 192.168.50.2) must not be in the
# router image; manage the board from the admin VLAN (lan1) instead.
IMAGE_INSTALL:remove = "espressobin-network"

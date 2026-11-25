SUMMARY = "Secure edge gateway image for Custos Marginis"
DESCRIPTION = "Complete software image for Custos Marginis."

IMAGE_INSTALL = "${CORE_IMAGE_BASE_INSTALL}"

inherit core-image
# A11: label every file at build time (setfiles), so the first boot already
# runs with the right SELinux contexts (no relabel reboot).
inherit ${@bb.utils.contains('DISTRO_FEATURES', 'selinux', 'selinux-image', '', d)}

CORE_IMAGE_EXTRA_INSTALL += "packagegroup-custos"

# custos-network owns the switch ports. The development network setup that
# local.conf adds to every image (lan0 = 192.168.50.2) must not be in the
# router image; manage the board from the admin VLAN (lan1) instead.
IMAGE_INSTALL:remove = "espressobin-network"

# packagegroup-core-ssh-dropbear recommends the OpenSSH SFTP server. Nothing
# on the device uses it; copy files with "scp -O" (legacy SCP protocol, served
# by dropbear itself) or "ssh ... 'cat > file' < file".
BAD_RECOMMENDATIONS += "openssh-sftp-server"

# A10 run-time hardening (kernel lockdown after boot). Left out of
# custos-image-baseline, the "before" image for the demo comparisons.
CUSTOS_HARDENING ?= "custos-hardening"
IMAGE_INSTALL:append = " ${CUSTOS_HARDENING}"

# mDNS name custos.local (systemd-resolved answers on the admin VLAN).
ROOTFS_POSTPROCESS_COMMAND += "custos_hostname;"
custos_hostname() {
    echo custos > ${IMAGE_ROOTFS}${sysconfdir}/hostname
}

# The vulnerable custosd (conf/custos-demo.conf, scenario S3) may only end up
# in custos-image-demo, and that image only exists with that configuration.
python () {
    demo_conf = d.getVar("CUSTOS_DEMO") == "1"
    demo_image = d.getVar("PN") == "custos-image-demo"
    if demo_conf and not demo_image:
        raise bb.parse.SkipRecipe("CUSTOS_DEMO is set: only custos-image-demo may be built")
    if demo_image and not demo_conf:
        raise bb.parse.SkipRecipe("build with: bitbake -R conf/custos-demo.conf custos-image-demo")
}

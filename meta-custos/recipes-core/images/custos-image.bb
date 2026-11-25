SUMMARY = "Secure edge gateway image for Custos Marginis"
DESCRIPTION = "Complete software image for Custos Marginis."

IMAGE_INSTALL = "${CORE_IMAGE_BASE_INSTALL}"

inherit core-image

CORE_IMAGE_EXTRA_INSTALL += "packagegroup-custos"

# custos-network owns the switch ports. The development network setup that
# local.conf adds to every image (lan0 = 192.168.50.2) must not be in the
# router image; manage the board from the admin VLAN (lan1) instead.
IMAGE_INSTALL:remove = "espressobin-network"

# packagegroup-core-ssh-dropbear recommends the OpenSSH SFTP server. Nothing
# on the device uses it; copy files with "scp -O" (legacy SCP protocol, served
# by dropbear itself) or "ssh ... 'cat > file' < file".
BAD_RECOMMENDATIONS += "openssh-sftp-server"

# mDNS name custos.local (systemd-resolved answers on the admin VLAN).
ROOTFS_POSTPROCESS_COMMAND += "custos_hostname;"
custos_hostname() {
    echo custos > ${IMAGE_ROOTFS}${sysconfdir}/hostname
}

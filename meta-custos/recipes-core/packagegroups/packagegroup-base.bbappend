# Custos does not use Avahi: systemd-resolved answers custos.local on the
# admin VLAN. avahi-daemon would listen for mDNS on every LAN VLAN (child and
# guest too) and compete with resolved for UDP 5353, so drop the zeroconf
# group. Removing "zeroconf" from DISTRO_FEATURES would be the cleaner way,
# but it changes the signature of most target recipes (full rebuild).
RDEPENDS:packagegroup-base:remove = "packagegroup-base-zeroconf"
RDEPENDS:packagegroup-distro-base:remove = "packagegroup-base-zeroconf"

# No NFS on Custos. The group only brings rpcbind, which listens on port 111
# on every interface.
RDEPENDS:packagegroup-base:remove = "packagegroup-base-nfs"
RDEPENDS:packagegroup-distro-base:remove = "packagegroup-base-nfs"

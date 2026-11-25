SUMMARY = "Signed RAUC update bundle with the Custos root file system (A13)"
LICENSE = "MIT"

inherit bundle

RAUC_BUNDLE_COMPATIBLE = "custos-espressobin"
RAUC_BUNDLE_VERSION = "${DISTRO_VERSION}-${DATETIME}"
RAUC_BUNDLE_VERSION[vardepsexclude] = "DATETIME"
RAUC_BUNDLE_DESCRIPTION = "Custos Marginis root file system (kernel in /boot)"
RAUC_BUNDLE_FORMAT = "verity"
RAUC_BUNDLE_SLOTS = "rootfs"
RAUC_SLOT_rootfs = "custos-image"
RAUC_SLOT_rootfs[fstype] = "ext4"

# Signing key: separate from every other key (SSH, FIT), outside the
# repository (keys/rauc/ on the build host).
CUSTOS_RAUC_KEYDIR ?= "${TOPDIR}/../../../keys/rauc"
RAUC_KEY_FILE = "${CUSTOS_RAUC_KEYDIR}/signing.key.pem"
RAUC_CERT_FILE = "${CUSTOS_RAUC_KEYDIR}/signing.cert.pem"
RAUC_KEYRING_FILE = "${CUSTOS_RAUC_KEYDIR}/ca.cert.pem"

# A10: dm-verity parameters of the root file system image (root hash) and
# the shared boot script; custos-rauc-post-install writes both to the boot
# partition. The bundle is signed, so are these files.
RAUC_BUNDLE_EXTRA_DEPENDS += "custos-image:do_image_complete espressobin-boot-script"
RAUC_BUNDLE_EXTRA_FILES += "custos-image-${MACHINE}.rootfs.verity.env boot.scr"

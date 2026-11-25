# Custos A/B layout (custos-espressobin-ab.wks) and the bundle CA. The CA
# certificate is public, but it lives with the keys outside the layers
# (keys/rauc/, created on the build host); the signing key never enters the
# image or the repository.
CUSTOS_RAUC_KEYDIR ?= "${TOPDIR}/../../../keys/rauc"
FILESEXTRAPATHS:prepend := "${THISDIR}/files:${CUSTOS_RAUC_KEYDIR}:"
RAUC_KEYRING_FILE = "ca.cert.pem"

SUMMARY = "Authorized SSH public key for root"
DESCRIPTION = "Installs the public key named by ESPRESSOBIN_SSH_PUBKEY as \
root's ~/.ssh/authorized_keys."
LICENSE = "MIT"
LIC_FILES_CHKSUM = "file://${COMMON_LICENSE_DIR}/MIT;md5=0835ade698e0bcf8506ecda2f7b4f302"

# Absolute path to an OpenSSH public key on the build host (set in local.conf)
ESPRESSOBIN_SSH_PUBKEY ?= ""

INHIBIT_DEFAULT_DEPS = "1"

python () {
    key = d.getVar('ESPRESSOBIN_SSH_PUBKEY')
    if not key:
        raise bb.parse.SkipRecipe("ESPRESSOBIN_SSH_PUBKEY is not set")
    d.appendVarFlag('do_install', 'file-checksums', ' %s:True' % key)
}

do_install() {
    install -d -m 0700 ${D}${ROOT_HOME}/.ssh
    install -m 0600 ${ESPRESSOBIN_SSH_PUBKEY} ${D}${ROOT_HOME}/.ssh/authorized_keys
}

FILES:${PN} = "${ROOT_HOME}/.ssh"

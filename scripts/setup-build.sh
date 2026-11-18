#!/bin/bash
# Fetch the pinned layers with bitbake-setup and prepare the build directory.
# Run inside the build container:
#
#   scripts/container-run.sh scripts/setup-build.sh
#
# Result: build/espressobin-wrynose/{layers,build}. Afterwards:
#
#   source build/espressobin-wrynose/build/init-build-env
#   bitbake core-image-minimal
set -euo pipefail

TOP=$(cd "$(dirname "$0")/.." && pwd)
CONFIG=$TOP/espressobin-wrynose.conf.json
SETUP=$TOP/build/espressobin-wrynose

# bitbake-setup ships with bitbake; PyPI only carries newer releases, so use
# the tool from the same bitbake release the configuration pins.
BB_SETUP_TAG=yocto-6.0.4
BB_SETUP_DIR=$TOP/build/bitbake-setup-tool

if [ ! -x "$BB_SETUP_DIR/bin/bitbake-setup" ]; then
    git clone --depth 1 -b "$BB_SETUP_TAG" https://git.openembedded.org/bitbake "$BB_SETUP_DIR"
fi

bbsetup() {
    "$BB_SETUP_DIR/bin/bitbake-setup" \
        --global-settings "$TOP/build/bitbake-setup-global.conf" \
        --setting default top-dir-prefix "$TOP" \
        --setting default top-dir-name build \
        --setting default dl-dir "$TOP/downloads/bitbake-setup" \
        "$@"
}

if [ ! -d "$SETUP" ]; then
    bbsetup init --non-interactive --no-init-vscode "$CONFIG" espressobin
else
    bbsetup status --setup-dir "$SETUP" || true
fi

# bitbake-setup writes an empty local.conf; install the project template.
# MACHINE and DISTRO come from the machine/ and distro/ fragments here, and
# bitbake refuses a second assignment, so leave those lines out.
local_conf=$SETUP/build/conf/local.conf
if ! grep -q ESPRESSOBIN_SSH_PUBKEY "$local_conf"; then
    grep -vE '^(MACHINE|DISTRO) \?=' "$TOP/conf-templates/local.conf.sample" >> "$local_conf"
fi

echo
echo "Build directory ready. Next:"
echo "  source $SETUP/build/init-build-env"
echo "  bitbake core-image-minimal"

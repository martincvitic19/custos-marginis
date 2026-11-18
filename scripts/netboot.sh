#!/bin/bash
# Serve the latest core-image-minimal to the ESPRESSObin over the network:
# kernel + DTB over TFTP, root filesystem over NFSv3. Both servers run in
# userspace inside the build container with --network host (no kernel NFS
# server on the host, no privileged container), the same way runqemu does it:
#
#   - tftpd-hpa binds 69/udp on $HOST_IP, chroots to build/netboot/tftp and
#     drops to the build user (the container starts as root only for port 69)
#   - unfsd (unfs3-native) runs as the build user under pseudo on ports
#     3049 (nfsd) / 3048 (mountd), without portmapper, so files in the
#     export appear root-owned to the board while staying yours on disk
#
# Run on the host:
#
#   scripts/netboot.sh            # prepare + serve (Ctrl-C stops the servers)
#   scripts/netboot.sh prepare    # only refresh build/netboot from the deploy dir
#   scripts/netboot.sh serve      # only start the servers
#
# Environment: HOST_IP (default 192.168.50.1), BOARD_IP (192.168.50.2),
#              BOARD_PORT (lan0), IMAGE_NAME (core-image-minimal)
#
# The factory U-Boot only talks to TFTP port 69, so a TFTP server already
# running on the host (e.g. the tftpd-hpa service) must be stopped first.
set -euo pipefail

TOP=$(cd "$(dirname "$0")/.." && pwd)
BUILDDIR=${BUILDDIR:-$TOP/build/espressobin-wrynose/build}
DEPLOY=$BUILDDIR/tmp/deploy/images/espressobin
COMPONENTS=$BUILDDIR/tmp/sysroots-components/x86_64
IMAGE_NAME=${IMAGE_NAME:-core-image-minimal}
DTB=armada-3720-espressobin-v7-emmc.dtb

HOST_IP=${HOST_IP:-192.168.50.1}
BOARD_IP=${BOARD_IP:-192.168.50.2}
BOARD_PORT=${BOARD_PORT:-lan0}
NFSD_PORT=3049
MOUNTD_PORT=3048

NETBOOT=$TOP/build/netboot
TFTP_DIR=$NETBOOT/tftp
NFS_ROOT=$NETBOOT/nfsroot
PSEUDO_STATE=$NETBOOT/nfsroot.pseudo_state

PSEUDO_PREFIX=$COMPONENTS/pseudo-native/usr
UNFSD=$COMPONENTS/unfs3-native/usr/bin/unfsd

in_container() { [ -f /.dockerenv ]; }

# Run this script's subcommand $1 in the container, keeping the settings.
in_build_container() {
    "$TOP/scripts/container-run.sh" env BUILDDIR="$BUILDDIR" IMAGE_NAME="$IMAGE_NAME" \
        HOST_IP="$HOST_IP" BOARD_IP="$BOARD_IP" BOARD_PORT="$BOARD_PORT" "$0" "$1"
}

# The pseudo server keeps its database in memory and writes it out when it
# exits. Stop it explicitly: the container's exit would kill it unflushed
# and lose the root ownership recorded during extraction.
pseudo_shutdown() {
    PSEUDO_LOCALSTATEDIR=$PSEUDO_STATE "$PSEUDO_PREFIX/bin/pseudo" -P "$PSEUDO_PREFIX" -S
}

# Build unfsd, copy kernel + DTB to the TFTP directory and extract the rootfs
# tarball into the NFS export, recording ownership in a pseudo database.
prepare() {
    (
        cd "$TOP"
        set +u
        if [ -f "$BUILDDIR/init-build-env" ]; then
            source "$BUILDDIR/init-build-env" >/dev/null
        else
            # Vanilla flow, see README.md
            export BITBAKEDIR=$TOP/layers/bitbake
            source "$TOP/layers/openembedded-core/oe-init-build-env" "$BUILDDIR" >/dev/null
        fi
        bitbake unfs3-native pseudo-native
    )

    local tarball
    tarball=$(readlink -f "$DEPLOY/$IMAGE_NAME-espressobin.rootfs.tar.bz2")

    mkdir -p "$TFTP_DIR"
    cp -L "$DEPLOY/Image" "$DEPLOY/$DTB" "$TFTP_DIR/"

    echo "Extracting $tarball to $NFS_ROOT"
    rm -rf "$NFS_ROOT" "$PSEUDO_STATE"
    mkdir -p "$NFS_ROOT" "$PSEUDO_STATE"
    PSEUDO_LOCALSTATEDIR=$PSEUDO_STATE \
        "$PSEUDO_PREFIX/bin/pseudo" -P "$PSEUDO_PREFIX" \
        tar --numeric-owner -xjf "$tarball" -C "$NFS_ROOT"
    pseudo_shutdown
    echo "$(basename "$tarball")" > "$NETBOOT/nfsroot.image"
}

# Runs as container root with the host network.
serve() {
    [ -d "$PSEUDO_STATE" ] || { echo "Run '$0 prepare' first" >&2; exit 1; }
    local user
    user=$(stat -c %U "$TOP")

    if ss -Hlnu "sport = :69" | grep -q .; then
        echo "UDP port 69 is already in use, probably by a TFTP server on the host." >&2
        echo "Stop it while netbooting, e.g.: sudo systemctl stop tftpd-hpa" >&2
        exit 1
    fi

    echo "$NFS_ROOT (rw,no_root_squash,no_all_squash,insecure)" > "$NETBOOT/exports"

    in.tftpd --foreground --secure --verbose --user "$user" \
        --address "$HOST_IP:69" "$TFTP_DIR" &
    runuser -u "$user" -- env PSEUDO_LOCALSTATEDIR="$PSEUDO_STATE" \
        "$PSEUDO_PREFIX/bin/pseudo" -P "$PSEUDO_PREFIX" \
        "$UNFSD" -d -p -l "$HOST_IP" -e "$NETBOOT/exports" \
        -n $NFSD_PORT -m $MOUNTD_PORT &

    # Stop the servers, then flush what the board changed to the pseudo DB
    trap 'kill $(jobs -p) 2>/dev/null; wait; runuser -u "$user" -- "$0" pseudo-shutdown' EXIT
    trap 'exit 130' INT TERM
    sleep 1
    echo
    echo "TFTP: $TFTP_DIR ($(ls "$TFTP_DIR" | tr '\n' ' '))"
    echo "NFS:  $NFS_ROOT ($(cat "$NETBOOT/nfsroot.image"))"
    echo
    echo "One-off U-Boot commands (nothing is saved):"
    echo "  setenv serverip $HOST_IP; setenv ipaddr $BOARD_IP"
    echo "  tftpboot 0x7000000 Image; tftpboot 0x6f00000 $DTB"
    echo "  setenv bootargs console=ttyMV0,115200 earlycon=ar3700_uart,0xd0012000" \
         "root=/dev/nfs rw nfsroot=$HOST_IP:$NFS_ROOT,nfsvers=3,tcp,port=$NFSD_PORT,mountport=$MOUNTD_PORT,mountproto=tcp" \
         "ip=$BOARD_IP::$HOST_IP:255.255.255.0:espressobin:$BOARD_PORT:off"
    echo "  booti 0x7000000 - 0x6f00000"
    echo
    echo "Serving, Ctrl-C to stop."
    wait -n
}

case "${1:-all}" in
    pseudo-shutdown)
        pseudo_shutdown ;;
    prepare)
        if in_container; then prepare; else in_build_container prepare; fi ;;
    serve)
        if in_container; then serve; else NET_HOST=1 AS_ROOT=1 in_build_container serve; fi ;;
    all)
        "$0" prepare
        "$0" serve ;;
    *)
        echo "Usage: $0 [prepare|serve]" >&2; exit 1 ;;
esac

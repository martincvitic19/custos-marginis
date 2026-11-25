SUMMARY = "Custos XDP edge filter (A12): EDSA-aware bogon/TIF/SYN filter on eth0"
DESCRIPTION = "XDP program for the ESPRESSObin DSA conduit (EDSA tagged \
frames): drops bogon sources, HaGeZi threat-intelligence sources and SYN \
floods arriving on the wan port, before the kernel allocates an skb. \
Loaded with bpftool by custos-xdp.service; native on eth0 with a generic \
fallback on wan."
# The BPF program is GPL-2.0 (kernel helpers); scripts MIT; the TIF address
# set is derived from the HaGeZi lists (GPL-3.0).
LICENSE = "GPL-2.0-only & MIT & GPL-3.0-only"
LICENSE:${PN} = "GPL-2.0-only & MIT"
LICENSE:${PN}-tif = "GPL-3.0-only"
LIC_FILES_CHKSUM = " \
    file://${COMMON_LICENSE_DIR}/GPL-2.0-only;md5=801f80980d171dd6425610833a22dbe6 \
    file://${COMMON_LICENSE_DIR}/MIT;md5=0835ade698e0bcf8506ecda2f7b4f302 \
    file://${COMMON_LICENSE_DIR}/GPL-3.0-only;md5=c79ff39f19dfec6d293b95dea7b07891 \
"

# Same pin as custos-firewall's DoH list (bump together).
HAGEZI_REV = "4e76a04fe84e33cba7ac0b71e21f7c1442350f9d"
SRC_URI = " \
    file://xdp_edge.bpf.c \
    file://custos-xdp \
    file://custos-xdp.service \
    file://custos-xdp-stats.service \
    file://custos-xdp-stats.timer \
    https://raw.githubusercontent.com/hagezi/dns-blocklists/${HAGEZI_REV}/ips/tif.txt;name=tif;downloadfilename=hagezi-tif-${HAGEZI_REV}.txt \
"
SRC_URI[tif.sha256sum] = "0297461e304242936d504ac9ac97b3b7e934752bc2093baec34e85481a686331"
S = "${UNPACKDIR}"

DEPENDS = "clang-native libbpf linux-libc-headers"

inherit systemd

# -D__aarch64__: the BPF target does not define it, and without it the OE
# multilib header wrappers (bits/wordsize.h, asm/types.h) pick the 32-bit
# headers, which do not exist in an aarch64-only sysroot.
# The path mapping keeps build directories out of the debug info and BTF
# (which must stay: the loader needs BTF), see QA check "buildpaths".
BPF_DEBUG_PATH = "/usr/src/debug/${PN}"
do_compile() {
    for v in dsa:1 plain:0; do
        name=${v%:*}; dsa=${v#*:}
        ${STAGING_BINDIR_NATIVE}/clang -O2 -g -target bpf -D__TARGET_ARCH_arm64 -D__aarch64__ \
            -ffile-prefix-map=${WORKDIR}=${BPF_DEBUG_PATH} \
            -fdebug-compilation-dir=${BPF_DEBUG_PATH} \
            -DCUSTOS_DSA=$dsa -I${STAGING_INCDIR} -Wall -Werror \
            -c ${S}/xdp_edge.bpf.c -o ${B}/xdp_edge_$name.o
    done
    # bpftool batch for the tif_v4 LPM trie: key = prefixlen (u32 little
    # endian) + IPv4 address (network order).
    sed -e 's/#.*//' -e 's/[[:space:]]//g' ${UNPACKDIR}/hagezi-tif-${HAGEZI_REV}.txt |
        grep -E '^[0-9]{1,3}(\.[0-9]{1,3}){3}(/[0-9]{1,2})?$' | sort -u |
        awk -F'[./]' '{ len = (NF == 5) ? $5 : 32;
            printf "map update pinned @MAP@ key hex %02x 00 00 00 %02x %02x %02x %02x value hex 01\n", len, $1, $2, $3, $4 }' \
        > ${B}/tif-v4.batch
    [ $(wc -l < ${B}/tif-v4.batch) -gt 1000 ] || bbfatal "TIF list too small"
}

do_install() {
    install -d ${D}${nonarch_libdir}/custos/bpf ${D}${sbindir} ${D}${systemd_system_unitdir}
    install -m 0644 ${B}/xdp_edge_dsa.o ${B}/xdp_edge_plain.o ${D}${nonarch_libdir}/custos/bpf/
    install -m 0644 ${B}/tif-v4.batch ${D}${nonarch_libdir}/custos/bpf/
    install -m 0755 ${UNPACKDIR}/custos-xdp ${D}${sbindir}/
    install -m 0644 ${UNPACKDIR}/custos-xdp.service ${UNPACKDIR}/custos-xdp-stats.service \
        ${UNPACKDIR}/custos-xdp-stats.timer ${D}${systemd_system_unitdir}/
}

PACKAGES =+ "${PN}-tif"
FILES:${PN}-tif = "${nonarch_libdir}/custos/bpf/tif-v4.batch"
FILES:${PN} += "${nonarch_libdir}/custos/bpf ${systemd_system_unitdir}"
SYSTEMD_SERVICE:${PN} = "custos-xdp.service custos-xdp-stats.timer"
RDEPENDS:${PN} = "bpftool"
RRECOMMENDS:${PN} = "${PN}-tif"

# BPF objects: not target-arch ELF, and stripping would remove the BTF the
# loader needs.
INHIBIT_PACKAGE_STRIP = "1"
INHIBIT_PACKAGE_DEBUG_SPLIT = "1"
INSANE_SKIP:${PN} += "arch"

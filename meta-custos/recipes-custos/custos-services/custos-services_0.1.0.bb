SUMMARY = "Custos Marginis services: custos-apply, custosd, custos-watch"
# Recipe name is not "custos": that is the DISTRO name and so an OVERRIDE.
DESCRIPTION = "custos-apply (root) turns /data/custos/custos.json into \
AdGuard Home settings, nftables policy, DHCP reservations, WireGuard and \
hostapd configuration. custosd (unprivileged) is the HTTPS API and web UI. \
custos-watch (unprivileged, CAP_NET_RAW/CAP_NET_ADMIN) raises bypass, \
upload, ISP-side, ISP Wi-Fi and ARP spoofing alerts."
HOMEPAGE = "https://github.com/martincvitic19/custos-marginis"
SECTION = "net"

# Own code MIT; vendored golang.org/x/crypto, golang.org/x/sys and
# rsc.io/qr are BSD-3-Clause.
LICENSE = "MIT & BSD-3-Clause"
LIC_FILES_CHKSUM = " \
    file://src/${GO_IMPORT}/LICENSE;md5=1ad8a172487c8ec3062c07d1c9295538 \
    file://src/${GO_IMPORT}/vendor/golang.org/x/crypto/LICENSE;md5=7998cb338f82d15c0eff93b7004d272a \
    file://src/${GO_IMPORT}/vendor/golang.org/x/sys/LICENSE;md5=7998cb338f82d15c0eff93b7004d272a \
    file://src/${GO_IMPORT}/vendor/rsc.io/qr/LICENSE;md5=5d4950ecb7b26d2c5e4e7b4e0dd74707 \
"

# The Go module lives at the top of the repository (custos/), next to the
# layers. Its dependencies are vendored (custos/vendor), so do_compile needs
# no network.
FILESEXTRAPATHS:prepend := "${THISDIR}/../../..:"
SRC_URI = " \
    file://custos;subdir=${BP}/src/github.com/martincvitic19/custos-marginis \
    file://custos-apply-boot.service \
    file://custos-apply.service \
    file://custos-apply-retry.service \
    file://custos-apply.timer \
    file://custos-apply.socket \
    file://custos-apply@.service \
    file://custosd.service \
    file://custos-watch.service \
    file://custos-hostapd.service \
    file://custos-tmpfiles.conf \
    file://60-custos-mdns.conf \
"

GO_IMPORT = "github.com/martincvitic19/custos-marginis/custos"
GO_INSTALL = "${GO_IMPORT}/cmd/..."
GO_LINKSHARED = ""

inherit go-mod systemd useradd

export GOTOOLCHAIN = "local"
export GOFLAGS = "-mod=vendor"
GOPROXY:task-compile = "off"
export CGO_ENABLED = "0"

# Extra build tags. custos-image-demo sets "vulnerable_demo" (A5/A11
# scenario S3); the production image must never have it.
CUSTOS_GO_TAGS ?= ""
GOBUILDFLAGS:prepend = "${@'-tags=' + d.getVar('CUSTOS_GO_TAGS') + ' ' if d.getVar('CUSTOS_GO_TAGS') else ''}"

PACKAGES =+ "custos-apply custosd custos-watch"
# go.bbclass skips the "ldflags" QA check for ${PN} only (Go links
# internally, so the binaries have no GNU_HASH); the binaries live in these
# packages.
INSANE_SKIP:custos-apply += "ldflags"
INSANE_SKIP:custosd += "ldflags"
INSANE_SKIP:custos-watch += "ldflags"

# Users: custosd and custoswatch, both in group custos (shared /data/custos).
USERADD_PACKAGES = "custos-apply custosd custos-watch"
GROUPADD_PARAM:custos-apply = "--system custos"
GROUPADD_PARAM:custosd = "--system custos"
GROUPADD_PARAM:custos-watch = "--system custos"
USERADD_PARAM:custosd = "--system --no-create-home --home-dir /data/custosd --shell /sbin/nologin --user-group --groups custos custosd"
USERADD_PARAM:custos-watch = "--system --no-create-home --home-dir /nonexistent --shell /sbin/nologin --user-group --groups custos custoswatch"

SYSTEMD_PACKAGES = "custos-apply custosd custos-watch"
SYSTEMD_SERVICE:custos-apply = "custos-apply-boot.service custos-apply.service custos-apply.timer custos-apply.socket custos-hostapd.service"
SYSTEMD_SERVICE:custosd = "custosd.service"
SYSTEMD_SERVICE:custos-watch = "custos-watch.service"
SYSTEMD_AUTO_ENABLE = "enable"

do_install() {
    install -d ${D}${bindir}
    for b in custos-apply custosd custos-watch; do
        install -m 0755 ${B}/${GO_BUILD_BINDIR}/$b ${D}${bindir}/$b
    done
    install -d ${D}${systemd_system_unitdir}
    for u in custos-apply-boot.service custos-apply.service custos-apply-retry.service \
             custos-apply.timer custos-apply.socket custos-apply@.service \
             custosd.service custos-watch.service custos-hostapd.service; do
        install -m 0644 ${UNPACKDIR}/$u ${D}${systemd_system_unitdir}/
    done
    install -d ${D}${nonarch_libdir}/tmpfiles.d
    install -m 0644 ${UNPACKDIR}/custos-tmpfiles.conf ${D}${nonarch_libdir}/tmpfiles.d/custos.conf
    install -d ${D}${nonarch_libdir}/systemd/resolved.conf.d
    install -m 0644 ${UNPACKDIR}/60-custos-mdns.conf ${D}${nonarch_libdir}/systemd/resolved.conf.d/
}

ALLOW_EMPTY:${PN} = "1"
FILES:custos-apply = " \
    ${bindir}/custos-apply \
    ${systemd_system_unitdir}/custos-apply* \
    ${systemd_system_unitdir}/custos-hostapd.service \
    ${nonarch_libdir}/tmpfiles.d/custos.conf \
"
FILES:custosd = " \
    ${bindir}/custosd \
    ${systemd_system_unitdir}/custosd.service \
    ${nonarch_libdir}/systemd/resolved.conf.d/60-custos-mdns.conf \
"
FILES:custos-watch = "${bindir}/custos-watch ${systemd_system_unitdir}/custos-watch.service"

RDEPENDS:custos-apply = "nftables conntrack-tools custos-firewall custos-network adguardhome"
RRECOMMENDS:custos-apply = "wireguard-tools hostapd kernel-module-wireguard kernel-module-nf-conntrack-netlink"
RDEPENDS:custosd = "custos-apply"
RDEPENDS:custos-watch = "nftables iw"

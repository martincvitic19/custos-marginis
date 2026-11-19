SUMMARY = "AdGuard Home: network-wide DNS filtering server"
DESCRIPTION = "DNS server with block lists, SafeSearch and a query log. On the \
Custos edge router every DNS query from the LAN VLANs and wg0 is redirected \
to it (custos-firewall). Built from source; the web UI listens on \
127.0.0.1:3000 only and its state lives on /data."
HOMEPAGE = "https://github.com/AdguardTeam/AdGuardHome"
SECTION = "net"

# PV comes from the file name. A version bump also needs SRCREV, the
# frontend sha256sum, AGH_COMMIT_TIME and a new update_modules run.

LICENSE = "GPL-3.0-only"
LIC_FILES_CHKSUM = "file://src/${GO_IMPORT}/LICENSE.txt;md5=1ebbd3e34237af26da5dc08a4e440464"
# Licenses of the Go module dependencies (generated, see below).
require ${BPN}-licenses.inc

# Source: pinned release tag; bitbake checks that the tag resolves to SRCREV.
#
# Web UI: main.go embeds build/ (go:embed). That directory is produced by an
# npm build of client/, which needs network access and a node toolchain.
# AdGuard publishes exactly that output for each release as
# AdGuardHome_frontend.tar.gz (HTML/JS/CSS, no executable code for the
# target); it is pinned by checksum and copied into build/ before compiling.
# The Go binary itself is always compiled here, never taken from a release.
SRC_URI = " \
    git://github.com/AdguardTeam/AdGuardHome.git;protocol=https;nobranch=1;tag=v${PV};destsuffix=${GO_SRCURI_DESTSUFFIX} \
    https://github.com/AdguardTeam/AdGuardHome/releases/download/v${PV}/AdGuardHome_frontend.tar.gz;name=frontend;subdir=adguardhome-frontend;downloadfilename=AdGuardHome_frontend-${PV}.tar.gz \
    file://adguardhome.service \
    file://AdGuardHome.yaml \
    file://adguardhome-tmpfiles.conf \
    file://50-custos-adguardhome.conf \
"
SRCREV = "05ba17b282da1c4393d6a4ba4db0cf519194a362"
SRC_URI[frontend.sha256sum] = "30184c157d4aefe5371b575a43c0ab5a7314aef039a820cc421ea67bab994ce8"

# Go modules, vendored through the gomod:// fetcher: every module is
# downloaded in do_fetch with a sha256sum, and do_compile runs offline.
# Both .inc files are generated, never edited by hand:
#   bitbake -c update_modules adguardhome
# Re-run it after changing PV/SRCREV; check the "Unknown license" warnings.
require ${BPN}-go-mods.inc

GO_IMPORT = "github.com/AdguardTeam/AdGuardHome"
# Only the main package (main.go); not the tools listed in go.mod.
GO_INSTALL = "${GO_IMPORT}"
GO_LINKSHARED = ""

inherit go-mod go-mod-update-modules systemd useradd

# v0.107.79 needs go >= 1.26.6 (go.mod). Never download a newer toolchain:
# fail instead if the layer's Go is too old.
export GOTOOLCHAIN = "local"
# No network in do_compile: everything must already be in GOMODCACHE.
GOPROXY:task-compile = "off"
# Same as the upstream release build (scripts/make/go-build.sh).
export CGO_ENABLED = "0"

# Upstream sets these through -X; without them the binary reports an empty
# version and the "development" channel. committime is the tag's commit time
# (git show -s --format=%ct v${PV}).
AGH_VERSION_PKG = "${GO_IMPORT}/internal/version"
AGH_COMMIT_TIME = "1786960338"
GO_EXTRA_LDFLAGS = " \
    -X ${AGH_VERSION_PKG}.version=v${PV} \
    -X ${AGH_VERSION_PKG}.channel=release \
    -X ${AGH_VERSION_PKG}.committime=${AGH_COMMIT_TIME} \
"

do_configure:append() {
    # go:embed build: put the release web UI where main.go expects it.
    cp -R ${UNPACKDIR}/adguardhome-frontend/build/. ${S}/src/${GO_IMPORT}/build/
    test -f ${S}/src/${GO_IMPORT}/build/static/index.html
}

# Dedicated system user; its home is the state directory on /data.
USERADD_PACKAGES = "${PN}"
USERADD_PARAM:${PN} = "--system --home-dir /data/adguardhome --no-create-home \
    --shell /sbin/nologin --user-group adguardhome"

SYSTEMD_SERVICE:${PN} = "adguardhome.service"
SYSTEMD_AUTO_ENABLE:${PN} = "enable"

# Replaces go_do_install, which would also install the Go sources.
do_install() {
    install -d ${D}${bindir}
    install -m 0755 ${B}/${GO_BUILD_BINDIR}/AdGuardHome ${D}${bindir}/AdGuardHome

    # Initial configuration; the service copies it to /data on first start.
    install -d ${D}${datadir}/adguardhome
    install -m 0644 ${UNPACKDIR}/AdGuardHome.yaml ${D}${datadir}/adguardhome/

    install -d ${D}${systemd_system_unitdir}
    install -m 0644 ${UNPACKDIR}/adguardhome.service ${D}${systemd_system_unitdir}/

    install -d ${D}${nonarch_libdir}/tmpfiles.d
    install -m 0644 ${UNPACKDIR}/adguardhome-tmpfiles.conf \
        ${D}${nonarch_libdir}/tmpfiles.d/adguardhome.conf

    # Frees port 53: systemd-resolved's stub listener would hold 127.0.0.53:53.
    install -d ${D}${nonarch_libdir}/systemd/resolved.conf.d
    install -m 0644 ${UNPACKDIR}/50-custos-adguardhome.conf \
        ${D}${nonarch_libdir}/systemd/resolved.conf.d/
}

FILES:${PN} = " \
    ${bindir}/AdGuardHome \
    ${datadir}/adguardhome \
    ${systemd_system_unitdir}/adguardhome.service \
    ${nonarch_libdir}/tmpfiles.d/adguardhome.conf \
    ${nonarch_libdir}/systemd/resolved.conf.d \
"

# DoH upstreams and block list downloads need the CA bundle.
RDEPENDS:${PN} = "ca-certificates"

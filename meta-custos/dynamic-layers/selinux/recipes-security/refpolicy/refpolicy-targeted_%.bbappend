# A11: the Custos policy module (domains custos_apply_t, custosd_t,
# custos_watch_t, adguardhome_t; types for /data/custos, the WireGuard keys,
# /data/custosd, /data/adguardhome, /run/custos), compiled into the policy
# store at build time like every refpolicy module.
FILESEXTRAPATHS:prepend := "${THISDIR}/files:"
SRC_URI += "file://custos.te file://custos.if file://custos.fc"

do_configure:append() {
    install -m 0644 ${UNPACKDIR}/custos.te ${UNPACKDIR}/custos.if ${UNPACKDIR}/custos.fc \
        ${S}/policy/modules/services/
}

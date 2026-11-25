# Custos (A10): the root file system is read-only, so do_rootfs refuses
# packages that must be configured on first boot. aide's on-target postinst
# only initialises or checks a database in the read-only rootfs; Custos keeps
# its databases on /data instead (custos-aide-check, custos-ids).
python () {
    d.delVar("pkg_postinst_ontarget:%s" % d.getVar("PN"))
}

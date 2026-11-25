# Custos (A12): no Python on the device. suricatasc and suricatactl are Rust
# programs since Suricata 8; the "python" option would only pull python3-core
# into the image.
PACKAGECONFIG:remove = "python"

# Custos (A10): the root file system is read-only, so do_rootfs refuses
# packages that must be configured on first boot. This postinst only runs
# sysvinit's populate-volatile.sh, which does not exist with systemd; Custos
# creates /data/suricata with systemd-tmpfiles (custos-ids).
python () {
    d.delVar("pkg_postinst_ontarget:%s" % d.getVar("PN"))
}

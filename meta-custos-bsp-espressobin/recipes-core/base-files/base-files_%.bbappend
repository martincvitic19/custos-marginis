# /data (partition 4, label "data") in the root file system's own fstab.
# Not left to wic: wic rewrites /etc/fstab with debugfs, which drops the
# SELinux label (unlabeled_t, AVCs from systemd-fstab-generator and mount),
# and a RAUC slot image never passes through wic at all, so slot B would
# boot without /data. The wks root partitions use --no-fstab-update.
# nofail: a missing /data must not drop the boot into emergency mode (no
# SSH); services that need it have RequiresMountsFor= and stay down (DNS
# fails closed). x-systemd.before=local-fs.target: nofail alone drops that
# ordering and systemd-tmpfiles would create /data/* on the root fs first.
# The mount point is part of the image: with the read-only root file system
# (A10) systemd cannot create /data at boot, so data.mount failed and SSH,
# DNS and the event log went down with it (slot A, 2026-11-21).
do_install:append:espressobin() {
    install -d -m 0755 ${D}/data
    printf 'LABEL=data\t/data\text4\tdefaults,nofail,x-systemd.before=local-fs.target,x-systemd.device-timeout=10s\t0\t0\n' \
        >> ${D}${sysconfdir}/fstab
}
FILES:${PN}:append:espressobin = " /data"

# Custos (A10, read-only root file system): copy+bind instead of overlayfs.
# Under SELinux the overlay's upper-layer operations run with the mounter's
# credentials (mount_t); creating the work directory was denied and the
# kernel mounted /var/lib read-only ("failed to create directory
# /var/volatile/.lib-work/work (errno: 13); mounting read-only"), which broke
# logind, timesyncd, rfkill and networkd's state directories (2026-11-21).
# /var/lib in the image is ~16 MB (mostly the SELinux policy store).
AVOID_OVERLAYFS = "1"

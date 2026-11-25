# Custos runs Suricata through custos-suricata.service (br-lan, own
# configuration); the stock unit (-i eth0) must not start as well.
SYSTEMD_AUTO_ENABLE:${PN} = "disable"

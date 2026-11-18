# Key-only SSH: replace the stock /etc/default/dropbear ("-w", no root
# login) with one that allows root but disables password authentication.
FILESEXTRAPATHS:prepend := "${THISDIR}/files:"

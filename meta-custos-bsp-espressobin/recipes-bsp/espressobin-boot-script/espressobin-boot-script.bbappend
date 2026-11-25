# Same boot script as meta-espressobin plus "pcie_aspm=off": with ASPM
# (L0s/L1) enabled the Aardvark PCIe link to the Compex WLE900VX (QCA9880)
# drops right after enumeration; the first ath10k register access then
# raises an SError and the kernel panics. Tested on 2026-11-20: with
# pcie_aspm=off the card works (firmware 10.2.4, two AP interfaces).
FILESEXTRAPATHS:prepend := "${THISDIR}/files:"
#
# "lockdown=integrity" (A10): kernel lockdown from the first instruction of
# userspace, together with CONFIG_MODULE_SIG_FORCE: no unsigned modules, no
# /dev/mem, no kexec of unsigned kernels, even for root.
#
# A13: boot.cmd.in is the A/B script (slot from custos-boot.env on the boot
# partition, kernel and DTB from /boot of the slot). The single-slot
# version is kept for reference as boot-single.cmd.in.txt.

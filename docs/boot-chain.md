# Boot chain and integrity (A10)

## Today

```
BootROM (Armada 3720, no eFuse secure boot)
  -> SPI NOR: TF-A + factory U-Boot          NOT verified, NOT locked
     -> mmc 0:1 ext4: boot.scr, custos-boot.env   NOT verified (plain files)
        -> kernel + DTB from /boot of slot A|B (custos-boot.env, docs/rauc.md)
           root=PARTUUID=<disk>-02|03 ro   rootfs ext4 read-only, no journal,
           dm-verity hash tree appended in the same partition
           -> systemd
              custos-firewall (sysinit, fail closed)
              custos-apply-boot (policy before networking)
              ... services in sandboxes (ProtectSystem=strict, no root
                  except custos-apply)
```

Kernel command line (boot.scr from meta-custos-bsp-espressobin):
`... pcie_aspm=off lockdown=integrity` (ASPM off: the Wi-Fi card's PCIe
link drops with L0s/L1 and the driver then panics the kernel).

What is enforced now (custos-image, not custos-image-baseline):

| Measure | Effect | Where |
|---------|--------|-------|
| Kernel lockdown `integrity` **from boot** (`lockdown=integrity`) | root cannot read/write kernel memory (`/dev/mem`, `/proc/kcore`), kexec unsigned kernels, load unsigned modules | boot.cmd.in (BSP), `CONFIG_SECURITY_LOCKDOWN_LSM=y`; custos-lockdown.service stays as a fallback |
| **Module signatures enforced** | every module is signed at build time (key in the kernel build directory, not in a layer); unsigned modules are rejected (`CONFIG_MODULE_SIG_FORCE=y`). Checked before enabling: all 400 packaged modules carried their signature, kernel not tainted | `custos-router.cfg` |
| SELinux (A11), **enforcing** | every Custos service in its own domain | docs/selinux.md |
| **Read-only root file system** | `/` mounted `ro` from the kernel command line on; /var/lib, /var/cache, /var/spool, /srv are tmpfs copies (volatile-binds, copy+bind: an overlayfs under SELinux came up read-only); everything persistent is on /data. ext4 without a journal: the kernel rewrites the journal superblock on the first mount, even read-only | custos-image.bbappend (`read-only-rootfs`, `-O ^has_journal`), volatile-binds.bbappend |
| **dm-verity hash tree per root image (check, not yet boot)** | built with `veritysetup format --hash-offset` after the data; the root hash goes into the RAUC bundle and, at install, into custos-boot.env (`CUSTOS_<slot>_VERITY`). `custos-verity-check` compares the whole slot at boot (idle I/O) and before a new slot is marked good; a mismatch is an alert and fails the health check | custos-image.bbappend, custos-rauc |
| Yama `ptrace_scope=2` | only root can ptrace | `CONFIG_SECURITY_YAMA=y`, `91-custos-hardening.conf` |
| `kptr_restrict=2`, `dmesg_restrict=1`, `unprivileged_bpf_disabled=1`, `bpf_jit_harden=2`, `HARDENED_USERCOPY` | less kernel info for an attacker, no unprivileged BPF | same |
| Sandboxed services | adguardhome, custosd, custos-watch: dedicated users, only the capabilities they need, read-only system | unit files |
| No default password | UI forces a password on first use; Wi-Fi passphrases random per device | custosd, custos-apply |

## Not done (and why)

| Plan item | Status | Reason / next step |
|-----------|--------|--------------------|
| Boot **from** the dm-verity device (phase 2) | **prepared, not enabled** | `CUSTOS_<slot>_DM` in custos-boot.env already holds the `dm-mod.create=` table. Remaining: `CONFIG_DM_INIT=y`, boot.scr passing `dm-mod.create="${CUSTOS_x_DM}" root=/dev/dm-0`. Needs you on the serial console for the first boot (a wrong table stops the boot before userspace, and the Linux-side boot counting cannot see that; `panic=10` reboots into the same slot). |
| Root hash in a signed FIT | **not done** | Without it the root hash in custos-boot.env is only as trustworthy as the boot partition. Needs a FIT image and a U-Boot that verifies it; replacing the factory U-Boot means writing SPI NOR (only you, with a UART recovery plan: `mox-imager`/`wtpdownloader`). |
| U-Boot `bootdelay=0`, read-only env | **not done** | SPI NOR. Back up the environment first, then `setenv bootdelay 0; saveenv` (only on the serial console; not run). |
| AIDE | **done** (A12 stage) | custos-ids: /usr/bin, /usr/sbin, units, sysctl, modprobe, /etc/custos, CA bundle, passwd/shadow, WireGuard keys, admin password hash; checked every 6 h, changes → alert in the UI |
| LKRG | **not enabled** | meta-security has lkrg-module 0.9.7; not verified against kernel 6.18, and out-of-tree modules must also be signed now (MODULE_SIG_FORCE). Next step after A13. |

**Known limit (documented):** the Armada 3720 has no eFuse-backed secure
boot, so the root of trust is the unverified U-Boot in SPI NOR. Anyone with
physical access and a UART can change it. The i.MX 93 (AHAB, EdgeLock
secure enclave) solves this; W-TPM can add measured boot (PCRs) and sealed
keys on this board, but not verified boot from ROM.

## Baseline vs hardened

`custos-image-baseline` is the same image without `custos-hardening` and
without `custos-watch`, for the before/after demos (tests/demo).

# A/B updates with RAUC (A13)

```
microSD: p1 boot (boot.scr, custos-boot.env) | p2 root A | p3 root B | p4 /data
boot.scr: reads custos-boot.env -> CUSTOS_SLOT=A|B -> kernel + DTB from
          /boot of that slot, root=PARTUUID=<disk>-0<2|3>
```

* **Bundle:** `bitbake custos-bundle` → `custos-bundle-espressobin.raucb`,
  format `verity` (dm-verity protected squashfs), contains the root file
  system (with the kernel in /boot). Signed with `keys/rauc/signing.*`
  (development CA `keys/rauc/ca.cert.pem`), a key used for nothing else;
  the image only contains the CA certificate (`/usr/lib/rauc/ca.cert.pem`).
* **Install:** `rauc install /data/custos-bundle.raucb` writes the inactive
  slot, then `custos-bootchooser set-primary B`: B becomes `trying` with 3
  boots. Reboot.
* **Boot counting:** `custos-boot-guard.service` (very early) counts the
  boots of a `trying` slot down; at 0 it marks the slot `bad`, switches
  back and reboots. `custos-health.timer` (3 min after boot) checks
  firewall, nft policy, AdGuard Home, DNS, custosd and the admin VLAN; on
  success `rauc status mark-good` → slot `good`; on failure the board
  reboots (one try fewer).
* **Post-install handler** (`custos-rauc-post-install`, system.conf
  `[handlers] post-install`): after the image is written it copies the
  root hash from the bundle (`<image>.verity.env`) into custos-boot.env as
  `CUSTOS_<slot>_VERITY` / `CUSTOS_<slot>_DM`, and installs the bundle's
  boot.scr on p1 (magic number checked, atomic rename). The slot itself is
  never mounted: a read-write mount would change it against its hash.
  The handler that runs is the one of the *running* slot, so a change to
  it takes effect one update later (an older slot installs the new image
  without verity data; custos-verity-check then reports "no root hash
  recorded").
* **Integrity of a new slot:** custos-health-check runs
  `custos-verity-check` (veritysetup verify through a read-only loop
  device, the mounted root cannot be opened directly) before marking a
  `trying` slot good. A mismatch is an event and counts as a failed boot.
* **Keyring:** `check-purpose=codesign`: the signing certificate carries
  extendedKeyUsage=codeSigning; RAUC's default (S/MIME) purpose rejects it
  ("unsuitable certificate purpose").
* **Kernel:** the verity bundle is a squashfs with SELinux labels, so
  `CONFIG_SQUASHFS_XATTR=y` is required (without it mounting the bundle
  fails with "Operation not supported", also in permissive mode).
* **State that survives an update** lives on /data: SSH host key
  (/data/dropbear), timesyncd clock (/data/timesync, the board has no RTC),
  AIDE databases (one per root image version), RAUC status file.
* **Tested 2026-11-21:** B→A and A→B installs of the same bundle, both
  slots marked good by the health check, root hash match on both; fallback
  after 3 failed boots (slot with /data missing) back to the good slot.
* **Wrong signature:** `rauc install evil.raucb` (signed with
  keys/rauc/evil.*) → `signature verification failed`, nothing written.

## Known limit (needs your decision)

The factory U-Boot counts boots only by writing its environment in SPI
NOR (`saveenv`), which this project does not do (rule 2). Counting is
therefore done by Linux: a kernel that panics before userspace is not
rolled back automatically (`panic=10` reboots into the same slot). Closing
that gap needs either U-Boot boot counting in SPI NOR (your call, with the
UART recovery plan) or an own U-Boot (also needed for verified boot, A10).
